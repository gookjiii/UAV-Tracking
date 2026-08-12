package service

import (
	"testing"
	"time"

	"github.com/uav_tracking/internal/domain"
)

func TestMergeAndDownsampleHistory(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	point := func(second int) *domain.DronePositionUpdate {
		return &domain.DronePositionUpdate{DroneID: "UAV-0001", Timestamp: base.Add(time.Duration(second) * time.Second)}
	}

	merged := mergeAndDownsample(
		[]*domain.DronePositionUpdate{point(0), point(2), point(4)},
		[]*domain.DronePositionUpdate{point(4), point(6), point(8)},
		3,
	)
	if len(merged) != 3 {
		t.Fatalf("expected 3 points, got %d", len(merged))
	}
	if !merged[0].Timestamp.Equal(base) || !merged[2].Timestamp.Equal(base.Add(8*time.Second)) {
		t.Fatalf("downsampling must retain endpoints: %+v", merged)
	}
}

func TestFilterUpdates(t *testing.T) {
	updates := []*domain.DronePositionUpdate{
		{DroneID: "UAV-ALLY", Type: domain.DroneTypeAlly},
		{DroneID: "UAV-ENEMY", Type: domain.DroneTypeEnemy},
	}
	filtered := filterUpdates(updates, domain.DroneTypeEnemy, "enemy")
	if len(filtered) != 1 || filtered[0].DroneID != "UAV-ENEMY" {
		t.Fatalf("unexpected filtered updates: %+v", filtered)
	}
}
