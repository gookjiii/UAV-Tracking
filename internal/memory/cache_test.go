package memory

import (
	"testing"
	"time"

	"github.com/uav_tracking/internal/domain"
)

func TestMemoryCacheRingKeepsNewestPoints(t *testing.T) {
	cache := NewMemoryCache(3)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		cache.Set(&domain.DronePositionUpdate{
			DroneID:   "UAV-0001",
			Position:  domain.Vector3D{Latitude: float64(i)},
			Timestamp: base.Add(time.Duration(i) * time.Second),
		})
	}

	history := cache.GetHistory("UAV-0001", 10)
	if len(history) != 3 {
		t.Fatalf("expected 3 ring points, got %d", len(history))
	}
	for i, want := range []float64{2, 3, 4} {
		if history[i].Position.Latitude != want {
			t.Fatalf("history[%d] latitude = %v, want %v", i, history[i].Position.Latitude, want)
		}
	}
}

func TestMemoryCacheSetBatchWritesOnce(t *testing.T) {
	cache := NewMemoryCache(10)
	now := time.Now().UTC()
	cache.SetBatch([]*domain.DronePositionUpdate{{DroneID: "UAV-0001", Timestamp: now}})

	if got := len(cache.GetHistory("UAV-0001", 10)); got != 1 {
		t.Fatalf("expected one history point, got %d", got)
	}
	if cache.Count() != 1 {
		t.Fatalf("expected one current drone, got %d", cache.Count())
	}
	if !cache.LastUpdated().Equal(now) {
		t.Fatalf("last update = %v, want %v", cache.LastUpdated(), now)
	}
}

func BenchmarkMemoryCacheSetBatch10K(b *testing.B) {
	cache := NewMemoryCache(1)
	updates := make([]*domain.DronePositionUpdate, 10000)
	for i := range updates {
		updates[i] = &domain.DronePositionUpdate{DroneID: benchmarkDroneID(i), Timestamp: time.Now().UTC()}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.SetBatch(updates)
	}
}

func benchmarkDroneID(i int) string {
	const digits = "0123456789"
	b := []byte("UAV-0000")
	for p := len(b) - 1; p >= 4 && i > 0; p-- {
		b[p] = digits[i%10]
		i /= 10
	}
	return string(b)
}
