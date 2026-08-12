package simulator

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uav_tracking/internal/domain"
)

// MockPublisher tracks published updates
type MockPublisher struct {
	mu      sync.Mutex
	updates []*domain.DronePositionUpdate
	count   int64
}

func (m *MockPublisher) PublishPosition(update *domain.DronePositionUpdate) error {
	atomic.AddInt64(&m.count, 1)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updates = append(m.updates, update)
	return nil
}

func (m *MockPublisher) GetCount() int64 {
	return atomic.LoadInt64(&m.count)
}

func (m *MockPublisher) GetUpdates() []*domain.DronePositionUpdate {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]*domain.DronePositionUpdate, len(m.updates))
	copy(cp, m.updates)
	return cp
}

// MockCache tracks cached updates
type MockCache struct {
	mu    sync.Mutex
	data  map[string]*domain.DronePositionUpdate
	count int64
}

func NewMockCache() *MockCache {
	return &MockCache{
		data: make(map[string]*domain.DronePositionUpdate),
	}
}

func (m *MockCache) Set(update *domain.DronePositionUpdate) {
	atomic.AddInt64(&m.count, 1)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[update.DroneID] = update
}

func (m *MockCache) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.data)
}

// MockRepo tracks saved repo updates
type MockRepo struct {
	count int64
}

func (m *MockRepo) SavePosition(update *domain.DronePositionUpdate) {
	atomic.AddInt64(&m.count, 1)
}

type MockBatchSink struct {
	publisher *MockPublisher
	cache     *MockCache
	repo      *MockRepo
}

func (m *MockBatchSink) ConsumeBatch(updates []*domain.DronePositionUpdate) {
	for _, update := range updates {
		if m.publisher != nil {
			_ = m.publisher.PublishPosition(update)
		}
		if m.cache != nil {
			m.cache.Set(update)
		}
		if m.repo != nil {
			m.repo.SavePosition(update)
		}
	}
}

// TestSimulationEngine_InitDronesDataGen tests data generation during initialization
func TestSimulationEngine_InitDronesDataGen(t *testing.T) {
	count := 1000
	engine := NewSimulationEngine(count, 300, nil)

	if len(engine.drones) != count {
		t.Fatalf("Expected %d drones initialized, got %d", count, len(engine.drones))
	}

	typeCounts := make(map[int32]int)
	orbitCounts := make(map[int32]int)

	for i, drone := range engine.drones {
		// Verify DroneID format
		if !strings.HasPrefix(drone.DroneID, "UAV-") {
			t.Errorf("Drone %d: expected prefix UAV-, got %s", i, drone.DroneID)
		}

		// Verify Drone Type range (1: Enemy, 2: Ally, 3: Undefined)
		if drone.Type < 1 || drone.Type > 3 {
			t.Errorf("Drone %d: invalid type %d", i, drone.Type)
		}
		typeCounts[drone.Type]++

		// Verify Orbit Type range (1: Circle, 2: Straight, 3: Zigzag)
		if drone.OrbitType < 1 || drone.OrbitType > 3 {
			t.Errorf("Drone %d: invalid orbit type %d", i, drone.OrbitType)
		}
		orbitCounts[drone.OrbitType]++

		// Verify global geographic bounds
		if drone.CenterLat < -70.0 || drone.CenterLat > 70.0 {
			t.Errorf("Drone %d: CenterLat %.4f out of expected range [-70, 70]", i, drone.CenterLat)
		}
		if drone.CenterLon < -180.0 || drone.CenterLon > 180.0 {
			t.Errorf("Drone %d: CenterLon %.4f out of expected range [-180, 180]", i, drone.CenterLon)
		}

		// Verify Altitude, Radius, Speed bounds
		if drone.BaseAlt < 200.0 || drone.BaseAlt > 3000.0 {
			t.Errorf("Drone %d: BaseAlt %.2f out of bounds [200, 3000]", i, drone.BaseAlt)
		}
		if drone.Radius < 500.0 || drone.Radius > 5000.0 {
			t.Errorf("Drone %d: Radius %.2f out of bounds [500, 5000]", i, drone.Radius)
		}
		if drone.SpeedMS < 15.0 || drone.SpeedMS > 75.0 {
			t.Errorf("Drone %d: SpeedMS %.2f out of bounds [15, 75]", i, drone.SpeedMS)
		}
	}

	// Verify all drone types & orbit types were generated
	if len(typeCounts) != 3 {
		t.Errorf("Expected all 3 drone types to be present, found %d", len(typeCounts))
	}
	if len(orbitCounts) != 3 {
		t.Errorf("Expected all 3 orbit types to be present, found %d", len(orbitCounts))
	}
}

// TestSimulationEngine_DefaultParameters tests fallback when count/interval are non-positive
func TestSimulationEngine_DefaultParameters(t *testing.T) {
	engine := NewSimulationEngine(0, 0, nil)
	if engine.targetCount != 10 {
		t.Errorf("Expected default targetCount 10, got %d", engine.targetCount)
	}
	if engine.intervalMs != 300 {
		t.Errorf("Expected default intervalMs 300, got %d", engine.intervalMs)
	}
}

// TestSimulationEngine_DataGenerationAndPublishing tests active data generation run
func TestSimulationEngine_DataGenerationAndPublishing(t *testing.T) {
	pub := &MockPublisher{}
	cache := NewMockCache()
	repo := &MockRepo{}

	droneCount := 50
	intervalMs := 20

	engine := NewSimulationEngine(droneCount, intervalMs, &MockBatchSink{publisher: pub, cache: cache, repo: repo})
	engine.Start()

	// Let simulation engine run for a short duration
	time.Sleep(120 * time.Millisecond)
	engine.Stop()

	pubCount := pub.GetCount()
	cacheCount := int64(cache.Count())
	repoCount := atomic.LoadInt64(&repo.count)

	if pubCount == 0 {
		t.Errorf("Expected publisher to receive generated data updates, got 0")
	}
	if cacheCount != int64(droneCount) {
		t.Errorf("Expected cache to contain all %d drones, got %d", droneCount, cacheCount)
	}
	if repoCount == 0 {
		t.Errorf("Expected repo to receive generated data updates, got 0")
	}

	// Inspect generated updates content
	updates := pub.GetUpdates()
	if len(updates) > 0 {
		sample := updates[0]
		if sample.DroneID == "" {
			t.Errorf("Generated update DroneID is empty")
		}
		if sample.Position.Latitude == 0 || sample.Position.Longitude == 0 {
			t.Errorf("Generated update position is zero: %+v", sample.Position)
		}
		if sample.Timestamp.IsZero() {
			t.Errorf("Generated update timestamp is zero")
		}
	}
}

// TestSimulationEngine_UpdateConfig tests dynamically reconfiguring the simulator engine
func TestSimulationEngine_UpdateConfig(t *testing.T) {
	pub := &MockPublisher{}
	cache := NewMockCache()

	engine := NewSimulationEngine(10, 100, &MockBatchSink{publisher: pub, cache: cache})

	// Update drone count and interval without activating
	newCount, ok, reset, msg := engine.UpdateConfig(50, 100, false)
	if !ok || newCount != 50 {
		t.Errorf("UpdateConfig failed: ok=%v, count=%d, msg=%s", ok, newCount, msg)
	}
	if !reset {
		t.Errorf("Expected population update to request a cache reset")
	}

	if len(engine.drones) != 50 {
		t.Errorf("Expected 50 drones after config update, got %d", len(engine.drones))
	}
	if atomic.LoadInt32(&engine.intervalMs) != 100 {
		t.Errorf("Expected intervalMs 100, got %d", engine.intervalMs)
	}

	// Activate via UpdateConfig
	engine.UpdateConfig(50, 100, true)
	time.Sleep(140 * time.Millisecond)

	if atomic.LoadInt32(&engine.active) != 1 {
		t.Errorf("Engine should be active after UpdateConfig(..., true)")
	}

	// Deactivate via UpdateConfig
	engine.UpdateConfig(50, 100, false)
	if atomic.LoadInt32(&engine.active) != 0 {
		t.Errorf("Engine should be inactive after UpdateConfig(..., false)")
	}
}

// TestSimulationEngine_StartStopIdempotency ensures starting/stopping multiple times is safe
func TestSimulationEngine_StartStopIdempotency(t *testing.T) {
	engine := NewSimulationEngine(10, 50, nil)

	engine.Start()
	engine.Start() // Double start should be a no-op

	time.Sleep(30 * time.Millisecond)

	engine.Stop()
	engine.Stop() // Double stop should be a no-op
}

// TestSimulationEngine_DataAccuracy3D verifies generated coordinates change dynamically over time
func TestSimulationEngine_DataAccuracy3D(t *testing.T) {
	pub := &MockPublisher{}
	engine := NewSimulationEngine(5, 10, &MockBatchSink{publisher: pub})

	engine.Start()
	time.Sleep(50 * time.Millisecond)
	engine.Stop()

	updates := pub.GetUpdates()
	if len(updates) < 2 {
		t.Fatalf("Need at least 2 updates to verify position changes, got %d", len(updates))
	}

	// Group updates by DroneID
	droneHistory := make(map[string][]*domain.DronePositionUpdate)
	for _, u := range updates {
		droneHistory[u.DroneID] = append(droneHistory[u.DroneID], u)
	}

	// Ensure at least one drone has multiple positions that move over time
	moved := false
	for _, history := range droneHistory {
		if len(history) >= 2 {
			p1 := history[0].Position
			p2 := history[len(history)-1].Position
			if p1.Latitude != p2.Latitude || p1.Longitude != p2.Longitude || p1.Altitude != p2.Altitude {
				moved = true
				break
			}
		}
	}

	if !moved {
		t.Errorf("Drone positions did not update over time during simulation run")
	}
}

type discardBatchSink struct{}

func (discardBatchSink) ConsumeBatch([]*domain.DronePositionUpdate) {}

func BenchmarkSimulationTick10K(b *testing.B) {
	engine := NewSimulationEngine(10_000, 300, discardBatchSink{})
	now := time.Now().UTC()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.generateSnapshot(now.Add(time.Duration(i) * 300 * time.Millisecond))
	}
}
