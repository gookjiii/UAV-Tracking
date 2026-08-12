package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	GRPCPort              string
	HTTPPort              string
	NATSURL               string
	PostgresDSN           string
	TargetDroneCount      int
	UpdateIntervalMS      int
	BatchSize             int
	RetentionDays         int
	HistorySampleInterval time.Duration
	MemoryHistoryPoints   int
	SSEInterval           time.Duration
	NATSMaxBytes          int64
}

func Load() (*Config, error) {
	targetDroneCount, err := getEnvAsInt("TARGET_DRONE_COUNT", 10)
	if err != nil {
		return nil, err
	}
	updateIntervalMS, err := getEnvAsInt("UPDATE_INTERVAL_MS", 300)
	if err != nil {
		return nil, err
	}
	batchSize, err := getEnvAsInt("BATCH_SIZE", 5000)
	if err != nil {
		return nil, err
	}
	retentionDays, err := getEnvAsInt("RETENTION_DAYS", 7)
	if err != nil {
		return nil, err
	}
	memoryHistoryPoints, err := getEnvAsInt("MEMORY_HISTORY_POINTS", 500)
	if err != nil {
		return nil, err
	}
	sseIntervalMS, err := getEnvAsInt("SSE_INTERVAL_MS", 300)
	if err != nil {
		return nil, err
	}
	natsMaxBytes, err := getEnvAsInt64("NATS_MAX_BYTES", 128*1024*1024)
	if err != nil {
		return nil, err
	}
	historySampleInterval, err := getEnvAsDuration("HISTORY_SAMPLE_INTERVAL", 5*time.Minute)
	if err != nil {
		return nil, err
	}

	if targetDroneCount < 1 || targetDroneCount > 10000 {
		return nil, fmt.Errorf("TARGET_DRONE_COUNT must be between 1 and 10000")
	}
	if updateIntervalMS < 100 || updateIntervalMS > 2000 {
		return nil, fmt.Errorf("UPDATE_INTERVAL_MS must be between 100 and 2000")
	}
	if batchSize < 1 || batchSize > 7000 {
		return nil, fmt.Errorf("BATCH_SIZE must be between 1 and 7000")
	}
	if retentionDays < 1 || retentionDays > 365 {
		return nil, fmt.Errorf("RETENTION_DAYS must be between 1 and 365")
	}
	if memoryHistoryPoints < 1 || memoryHistoryPoints > 5000 {
		return nil, fmt.Errorf("MEMORY_HISTORY_POINTS must be between 1 and 5000")
	}
	if sseIntervalMS < 100 || sseIntervalMS > 5000 {
		return nil, fmt.Errorf("SSE_INTERVAL_MS must be between 100 and 5000")
	}
	if historySampleInterval < time.Second {
		return nil, fmt.Errorf("HISTORY_SAMPLE_INTERVAL must be at least 1s")
	}
	if natsMaxBytes < 1024*1024 {
		return nil, fmt.Errorf("NATS_MAX_BYTES must be at least 1048576")
	}

	return &Config{
		GRPCPort:              getEnv("GRPC_PORT", ":50051"),
		HTTPPort:              getEnv("HTTP_PORT", ":8080"),
		NATSURL:               getEnv("NATS_URL", "nats://localhost:4222"),
		PostgresDSN:           getEnv("POSTGRES_DSN", "postgres://postgres:postgres@localhost:5432/uav_tracking?sslmode=disable"),
		TargetDroneCount:      targetDroneCount,
		UpdateIntervalMS:      updateIntervalMS,
		BatchSize:             batchSize,
		RetentionDays:         retentionDays,
		HistorySampleInterval: historySampleInterval,
		MemoryHistoryPoints:   memoryHistoryPoints,
		SSEInterval:           time.Duration(sseIntervalMS) * time.Millisecond,
		NATSMaxBytes:          natsMaxBytes,
	}, nil
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvAsInt(key string, defaultVal int) (int, error) {
	if valStr := os.Getenv(key); valStr != "" {
		val, err := strconv.Atoi(valStr)
		if err != nil {
			return 0, fmt.Errorf("%s must be an integer: %w", key, err)
		}
		return val, nil
	}
	return defaultVal, nil
}

func getEnvAsInt64(key string, defaultVal int64) (int64, error) {
	if valStr := os.Getenv(key); valStr != "" {
		val, err := strconv.ParseInt(valStr, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("%s must be an integer: %w", key, err)
		}
		return val, nil
	}
	return defaultVal, nil
}

func getEnvAsDuration(key string, defaultVal time.Duration) (time.Duration, error) {
	if valStr := os.Getenv(key); valStr != "" {
		val, err := time.ParseDuration(valStr)
		if err != nil {
			return 0, fmt.Errorf("%s must be a duration such as 5m or 30s: %w", key, err)
		}
		return val, nil
	}
	return defaultVal, nil
}
