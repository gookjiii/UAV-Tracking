package simulator

import (
	"fmt"
	"log"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/uav_tracking/internal/domain"
)

type BatchSink interface {
	ConsumeBatch(updates []*domain.DronePositionUpdate)
}

type SimulationEngine struct {
	targetCount   int32
	intervalMs    int32
	active        int32
	sink          BatchSink
	drones        []*OrbitState
	mu            sync.RWMutex
	controlMu     sync.Mutex
	stopChan      chan struct{}
	configChanged chan struct{}
	wg            sync.WaitGroup
}

func NewSimulationEngine(
	targetCount int,
	intervalMs int,
	sink BatchSink,
) *SimulationEngine {
	if targetCount <= 0 {
		targetCount = 10
	}
	if intervalMs <= 0 {
		intervalMs = 300
	}

	engine := &SimulationEngine{
		targetCount:   int32(targetCount),
		intervalMs:    int32(intervalMs),
		sink:          sink,
		stopChan:      make(chan struct{}),
		configChanged: make(chan struct{}, 1),
	}

	engine.initDrones(int(targetCount))
	return engine
}

func (se *SimulationEngine) initDrones(count int) {
	se.mu.Lock()
	defer se.mu.Unlock()

	se.drones = make([]*OrbitState, count)
	now := time.Now().UTC()
	r := rand.New(rand.NewSource(42))

	for i := 0; i < count; i++ {
		// Type distribution: 40% Enemy (1), 40% Ally (2), 20% Undefined (3)
		var droneType int32 = 1
		p := r.Float64()
		if p > 0.4 && p <= 0.8 {
			droneType = 2
		} else if p > 0.8 {
			droneType = 3
		}

		// Orbit distribution: 1: Circle, 2: Straight, 3: Zigzag
		orbitType := int32(r.Intn(3) + 1)

		// Unbounded global coordinate generation (unrestricted boundary)
		centerLat := (r.Float64() - 0.5) * 140.0 // -70° to +70° Latitude
		centerLon := (r.Float64() - 0.5) * 360.0 // -180° to +180° Longitude

		se.drones[i] = &OrbitState{
			DroneID:     fmt.Sprintf("UAV-%04d", i),
			Type:        droneType,
			OrbitType:   orbitType,
			CenterLat:   centerLat,
			CenterLon:   centerLon,
			BaseAlt:     200.0 + r.Float64()*2800.0, // 200m - 3000m altitude
			Radius:      500.0 + r.Float64()*4500.0, // 500m - 5000m orbit radius
			SpeedMS:     15.0 + r.Float64()*60.0,    // 15 - 75 m/s (~50-270 km/h)
			HeadingDeg:  r.Float64() * 360.0,
			Amplitude:   50.0 + r.Float64()*250.0,
			Frequency:   0.05 + r.Float64()*0.1,
			PhaseOffset: r.Float64() * 2 * 3.14159,
			StartTime:   now,
		}
	}
}

func (se *SimulationEngine) Start() {
	if !atomic.CompareAndSwapInt32(&se.active, 0, 1) {
		return
	}

	se.wg.Add(1)
	go se.runLoop()
	log.Printf("Simulation Engine started: %d drones at %d ms interval (~%.1f updates/sec)",
		se.targetCount, se.intervalMs, float64(se.targetCount)/(float64(se.intervalMs)/1000.0))
}

func (se *SimulationEngine) Stop() {
	if atomic.CompareAndSwapInt32(&se.active, 1, 0) {
		close(se.stopChan)
		se.wg.Wait()
		log.Println("Simulation Engine stopped")
	}
}

func (se *SimulationEngine) UpdateConfig(count int, intervalMs int, active bool) (int, bool, bool, string) {
	se.controlMu.Lock()
	defer se.controlMu.Unlock()

	if count < 1 || count > 10000 {
		return int(atomic.LoadInt32(&se.targetCount)), false, false, "target_drone_count must be between 1 and 10000"
	}
	if intervalMs < 100 || intervalMs > 2000 {
		return int(atomic.LoadInt32(&se.targetCount)), false, false, "update_interval_ms must be between 100 and 2000"
	}

	reset := false
	if count != int(atomic.LoadInt32(&se.targetCount)) {
		se.initDrones(count)
		atomic.StoreInt32(&se.targetCount, int32(count))
		reset = true
	}
	if intervalMs != int(atomic.LoadInt32(&se.intervalMs)) {
		atomic.StoreInt32(&se.intervalMs, int32(intervalMs))
		select {
		case se.configChanged <- struct{}{}:
		default:
		}
	}

	if active && atomic.LoadInt32(&se.active) == 0 {
		se.stopChan = make(chan struct{})
		se.Start()
	} else if !active && atomic.LoadInt32(&se.active) == 1 {
		se.Stop()
	}

	return int(atomic.LoadInt32(&se.targetCount)), true, reset, "Configuration updated successfully"
}

func (se *SimulationEngine) runLoop() {
	defer se.wg.Done()
	timer := time.NewTimer(time.Duration(atomic.LoadInt32(&se.intervalMs)) * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case <-se.stopChan:
			return
		case <-se.configChanged:
			resetTimer(timer, time.Duration(atomic.LoadInt32(&se.intervalMs))*time.Millisecond)
		case <-timer.C:
			se.generateSnapshot(time.Now().UTC())
			resetTimer(timer, time.Duration(atomic.LoadInt32(&se.intervalMs))*time.Millisecond)
		}
	}
}

func (se *SimulationEngine) generateSnapshot(now time.Time) {
	se.mu.RLock()
	// Keep each tick immutable for asynchronous consumers while allocating the
	// 10,000 update records as one contiguous block instead of 10,000 objects.
	values := make([]domain.DronePositionUpdate, len(se.drones))
	updates := make([]*domain.DronePositionUpdate, len(values))
	for i, state := range se.drones {
		pos, speed, heading := state.ComputePosition(now)
		values[i] = domain.DronePositionUpdate{
			DroneID: state.DroneID, Type: domain.DroneType(state.Type),
			OrbitType: domain.OrbitType(state.OrbitType), Position: pos,
			SpeedMS: speed, HeadingDeg: heading, Timestamp: now,
		}
		updates[i] = &values[i]
	}
	se.mu.RUnlock()
	if se.sink != nil {
		se.sink.ConsumeBatch(updates)
	}
}

func resetTimer(timer *time.Timer, duration time.Duration) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(duration)
}

func (se *SimulationEngine) IsActive() bool {
	return atomic.LoadInt32(&se.active) == 1
}

func (se *SimulationEngine) TargetCount() int {
	return int(atomic.LoadInt32(&se.targetCount))
}
