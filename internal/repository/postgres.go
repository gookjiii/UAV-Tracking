package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/uav_tracking/internal/domain"
)

type PostgresRepository struct {
	db            *sql.DB
	batchChan     chan *domain.DronePositionUpdate
	batchSize     int
	flushInterval time.Duration
	retentionDays int
	stopChan      chan struct{}
	wg            sync.WaitGroup
	closeOnce     sync.Once
}

func NewPostgresRepository(dsn string, batchSize, retentionDays int) (*PostgresRepository, error) {
	var db *sql.DB
	var err error

	for i := 0; i < 5; i++ {
		db, err = sql.Open("pgx", dsn)
		if err == nil {
			err = db.Ping()
			if err == nil {
				break
			}
		}
		log.Printf("Waiting for PostgreSQL... (attempt %d/5)", i+1)
		time.Sleep(1 * time.Second)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	db.SetMaxOpenConns(50)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	repo := &PostgresRepository{
		db:            db,
		batchChan:     make(chan *domain.DronePositionUpdate, batchSize*4),
		batchSize:     batchSize,
		flushInterval: 250 * time.Millisecond,
		retentionDays: retentionDays,
		stopChan:      make(chan struct{}),
	}

	if err := repo.initSchema(); err != nil {
		db.Close()
		return nil, err
	}

	// Start batch worker & partition retention cleaner
	repo.wg.Add(2)
	go repo.batchWorker()
	go repo.retentionCleaner()

	return repo, nil
}

func (r *PostgresRepository) initSchema() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Master table partitioned by RANGE (timestamp)
	query := `
	CREATE TABLE IF NOT EXISTS drone_history (
		drone_id VARCHAR(64) NOT NULL,
		drone_type SMALLINT NOT NULL,
		orbit_type SMALLINT NOT NULL,
		latitude DOUBLE PRECISION NOT NULL,
		longitude DOUBLE PRECISION NOT NULL,
		altitude DOUBLE PRECISION NOT NULL,
		speed_m_s DOUBLE PRECISION NOT NULL,
		heading_deg DOUBLE PRECISION NOT NULL,
		timestamp TIMESTAMPTZ NOT NULL
	) PARTITION BY RANGE (timestamp);
	`
	if _, err := r.db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("failed to create drone_history master table: %w", err)
	}

	// Ensure daily partitions for today and next 3 days.
	return r.ensurePartitions()
}

func (r *PostgresRepository) ensurePartitions() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	now := time.Now().UTC()
	for i := 0; i <= 3; i++ {
		t := now.AddDate(0, 0, i)
		partitionName := fmt.Sprintf("drone_history_y%04dm%02dd%02d", t.Year(), int(t.Month()), t.Day())
		startOfDay := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
		endOfDay := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1).Format(time.RFC3339)

		q := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s PARTITION OF drone_history
		FOR VALUES FROM ('%s') TO ('%s');
		
		CREATE INDEX IF NOT EXISTS idx_%s_id_ts ON %s (drone_id, timestamp DESC);
		`, partitionName, startOfDay, endOfDay, partitionName, partitionName)

		if _, err := r.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("create partition %s: %w", partitionName, err)
		}
	}
	return nil
}

func (r *PostgresRepository) retentionCleaner() {
	defer r.wg.Done()
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopChan:
			return
		case <-ticker.C:
			if err := r.ensurePartitions(); err != nil {
				log.Printf("Failed to maintain PostgreSQL partitions: %v", err)
			}
			r.purgeOldPartitions()
		}
	}
}

func (r *PostgresRepository) purgeOldPartitions() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cutoff := time.Now().UTC().AddDate(0, 0, -r.retentionDays)

	// Query existing partition tables
	rows, err := r.db.QueryContext(ctx, `
		SELECT tablename FROM pg_tables 
		WHERE tablename LIKE 'drone_history_y%'
	`)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			continue
		}
		// Parse date from partition table name drone_history_yYYYYmMMdDD
		var y, m, d int
		if _, err := fmt.Sscanf(tableName, "drone_history_y%04dm%02dd%02d", &y, &m, &d); err == nil {
			partDate := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
			if partDate.Before(time.Date(cutoff.Year(), cutoff.Month(), cutoff.Day(), 0, 0, 0, 0, time.UTC)) {
				dropQuery := fmt.Sprintf("DROP TABLE IF EXISTS %s;", tableName)
				if _, err := r.db.ExecContext(ctx, dropQuery); err == nil {
					log.Printf("Purged partition older than %d days: %s", r.retentionDays, tableName)
				}
			}
		}
	}
}

func (r *PostgresRepository) SaveBatch(updates []*domain.DronePositionUpdate) int {
	if r == nil || r.batchChan == nil {
		return len(updates)
	}
	dropped := 0
	for _, update := range updates {
		if update == nil {
			continue
		}
		select {
		case r.batchChan <- update:
		default:
			dropped++
		}
	}
	return dropped
}

func (r *PostgresRepository) batchWorker() {
	defer r.wg.Done()

	buffer := make([]*domain.DronePositionUpdate, 0, r.batchSize)
	ticker := time.NewTicker(r.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopChan:
			for {
				select {
				case item := <-r.batchChan:
					buffer = append(buffer, item)
					if len(buffer) >= r.batchSize {
						r.flushBuffer(buffer)
						buffer = make([]*domain.DronePositionUpdate, 0, r.batchSize)
					}
				default:
					r.flushBuffer(buffer)
					return
				}
			}
		case item, ok := <-r.batchChan:
			if !ok {
				r.flushBuffer(buffer)
				return
			}
			buffer = append(buffer, item)
			if len(buffer) >= r.batchSize {
				r.flushBuffer(buffer)
				buffer = make([]*domain.DronePositionUpdate, 0, r.batchSize)
			}
		case <-ticker.C:
			if len(buffer) > 0 {
				r.flushBuffer(buffer)
				buffer = make([]*domain.DronePositionUpdate, 0, r.batchSize)
			}
		}
	}
}

func (r *PostgresRepository) flushBuffer(buffer []*domain.DronePositionUpdate) {
	if len(buffer) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// High-speed multi-row INSERT INTO statement
	var sb strings.Builder
	sb.WriteString("INSERT INTO drone_history (drone_id, drone_type, orbit_type, latitude, longitude, altitude, speed_m_s, heading_deg, timestamp) VALUES ")

	vals := make([]interface{}, 0, len(buffer)*9)
	for i, item := range buffer {
		if i > 0 {
			sb.WriteString(",")
		}
		base := i * 9
		sb.WriteString(fmt.Sprintf("($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8, base+9))

		vals = append(vals,
			item.DroneID,
			int16(item.Type),
			int16(item.OrbitType),
			item.Position.Latitude,
			item.Position.Longitude,
			item.Position.Altitude,
			item.SpeedMS,
			item.HeadingDeg,
			item.Timestamp,
		)
	}

	_, err := r.db.ExecContext(ctx, sb.String(), vals...)
	if err != nil {
		log.Printf("Error flushing batch of %d records to DB: %v", len(buffer), err)
	}
}

func (r *PostgresRepository) GetHistory(ctx context.Context, droneID string, startTime, endTime time.Time, maxPoints int) ([]*domain.DronePositionUpdate, error) {
	if r == nil || r.db == nil {
		return []*domain.DronePositionUpdate{}, nil
	}
	if maxPoints <= 0 {
		maxPoints = 500
	}
	if maxPoints > 5000 {
		maxPoints = 5000
	}
	query := `
	WITH bucketed AS (
		SELECT drone_id, drone_type, orbit_type, latitude, longitude, altitude,
		       speed_m_s, heading_deg, timestamp,
		       ntile($4) OVER (ORDER BY timestamp) AS bucket
		FROM drone_history
		WHERE drone_id = $1 AND timestamp >= $2 AND timestamp <= $3
	)
	SELECT DISTINCT ON (bucket)
	       drone_id, drone_type, orbit_type, latitude, longitude, altitude,
	       speed_m_s, heading_deg, timestamp
	FROM bucketed
	ORDER BY bucket, timestamp ASC
	`

	rows, err := r.db.QueryContext(ctx, query, droneID, startTime, endTime, maxPoints)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []*domain.DronePositionUpdate
	for rows.Next() {
		var item domain.DronePositionUpdate
		var dType, oType int16

		err := rows.Scan(
			&item.DroneID,
			&dType,
			&oType,
			&item.Position.Latitude,
			&item.Position.Longitude,
			&item.Position.Altitude,
			&item.SpeedMS,
			&item.HeadingDeg,
			&item.Timestamp,
		)
		if err != nil {
			return nil, fmt.Errorf("scan drone history: %w", err)
		}
		item.Type = domain.DroneType(dType)
		item.OrbitType = domain.OrbitType(oType)
		records = append(records, &item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate drone history: %w", err)
	}
	return records, nil
}

func (r *PostgresRepository) Close() {
	if r == nil {
		return
	}
	r.closeOnce.Do(func() {
		close(r.stopChan)
		r.wg.Wait()
		if r.db != nil {
			r.db.Close()
		}
	})
}

func (r *PostgresRepository) Healthy(ctx context.Context) bool {
	if r == nil || r.db == nil {
		return false
	}
	return r.db.PingContext(ctx) == nil
}
