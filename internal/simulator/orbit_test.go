package simulator

import (
	"testing"
	"time"
)

func TestCircleOrbitPosition(t *testing.T) {
	now := time.Now()
	state := &OrbitState{
		DroneID:   "UAV-TEST-CIRCLE",
		OrbitType: 1,
		CenterLat: 21.0285,
		CenterLon: 105.8542,
		BaseAlt:   500.0,
		Radius:    1000.0,
		SpeedMS:   30.0,
		StartTime: now,
	}

	pos0, speed0, heading0 := state.ComputePosition(now)
	if speed0 != 30.0 {
		t.Errorf("expected speed 30.0, got %f", speed0)
	}

	pos1, _, heading1 := state.ComputePosition(now.Add(5 * time.Second))

	if pos0.Latitude == pos1.Latitude && pos0.Longitude == pos1.Longitude {
		t.Errorf("circle orbit position failed to advance over time")
	}

	if heading0 == heading1 {
		t.Errorf("circle orbit heading should change over time")
	}
}

func TestStraightOrbitPosition(t *testing.T) {
	now := time.Now()
	state := &OrbitState{
		DroneID:    "UAV-TEST-STRAIGHT",
		OrbitType:  2,
		CenterLat:  21.0285,
		CenterLon:  105.8542,
		BaseAlt:    500.0,
		SpeedMS:    40.0,
		HeadingDeg: 90.0, // Heading East
		StartTime:  now,
	}

	pos0, _, _ := state.ComputePosition(now)
	pos1, _, _ := state.ComputePosition(now.Add(10 * time.Second))

	if pos1.Longitude <= pos0.Longitude {
		t.Errorf("heading east should increase longitude over time")
	}
}

func TestZigzagOrbitPosition(t *testing.T) {
	now := time.Now()
	state := &OrbitState{
		DroneID:    "UAV-TEST-ZIGZAG",
		OrbitType:  3,
		CenterLat:  21.0285,
		CenterLon:  105.8542,
		BaseAlt:    500.0,
		SpeedMS:    25.0,
		HeadingDeg: 0.0, // North
		Amplitude:  100.0,
		Frequency:  0.1,
		StartTime:  now,
	}

	pos0, _, _ := state.ComputePosition(now)
	pos1, _, _ := state.ComputePosition(now.Add(time.Duration(2.5 * float64(time.Second))))

	if pos0.Latitude == pos1.Latitude && pos0.Longitude == pos1.Longitude {
		t.Errorf("zigzag orbit position failed to update")
	}
}
