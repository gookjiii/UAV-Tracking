package service

import (
	"testing"
	"time"

	"github.com/uav_tracking/internal/domain"
	"github.com/uav_tracking/internal/memory"
)

type recordingRepository struct{ batches int }

func (r *recordingRepository) SaveBatch([]*domain.DronePositionUpdate) int {
	r.batches++
	return 0
}

func TestTelemetryPipelineSamplesHistory(t *testing.T) {
	cache := memory.NewMemoryCache(10)
	repo := &recordingRepository{}
	pipeline := NewTelemetryPipeline(cache, repo, nil, 5*time.Minute)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for _, offset := range []time.Duration{0, time.Minute, 5 * time.Minute} {
		pipeline.ConsumeBatch([]*domain.DronePositionUpdate{{DroneID: "UAV-0001", Timestamp: base.Add(offset)}})
	}

	if repo.batches != 2 {
		t.Fatalf("expected 2 sampled batches, got %d", repo.batches)
	}
	if got := len(cache.GetHistory("UAV-0001", 10)); got != 3 {
		t.Fatalf("expected all 3 points in memory, got %d", got)
	}
}
