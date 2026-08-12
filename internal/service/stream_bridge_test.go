package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/uav_tracking/internal/domain"
	"github.com/uav_tracking/internal/memory"
)

func testUpdate(latitude float64) *domain.DronePositionUpdate {
	return &domain.DronePositionUpdate{
		DroneID:   "UAV-0001",
		Type:      domain.DroneTypeEnemy,
		Position:  domain.Vector3D{Latitude: latitude, Longitude: 106},
		Timestamp: time.Now().UTC(),
	}
}

func TestStreamBridgeReplacesSnapshotForSlowSubscriber(t *testing.T) {
	cache := memory.NewMemoryCache(4)
	cache.Set(testUpdate(1))
	bridge := NewStreamBridge(cache, 5*time.Millisecond)
	defer bridge.Close()

	subscriber := make(chan streamSnapshot, 1)
	bridge.mu.Lock()
	bridge.subscribers[subscriber] = true
	bridge.mu.Unlock()
	defer func() {
		bridge.mu.Lock()
		delete(bridge.subscribers, subscriber)
		bridge.mu.Unlock()
	}()

	select {
	case <-subscriber:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("initial snapshot was not broadcast")
	}
	cache.Set(testUpdate(2))
	time.Sleep(12 * time.Millisecond) // leave latitude=2 queued
	cache.Set(testUpdate(3))
	time.Sleep(12 * time.Millisecond) // latest snapshot replaces latitude=2

	select {
	case snapshot := <-subscriber:
		if got := snapshot.updates[0].Position.Latitude; got != 3 {
			t.Fatalf("slow subscriber received stale latitude %v", got)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("latest snapshot was not broadcast")
	}
}

func TestServeSSEWritesNamedSnapshotEvent(t *testing.T) {
	cache := memory.NewMemoryCache(4)
	cache.Set(testUpdate(21))
	bridge := NewStreamBridge(cache, 5*time.Millisecond)
	defer bridge.Close()

	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest("GET", "/v1/drones/stream", nil).WithContext(ctx)
	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		bridge.ServeSSE(recorder, request)
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("SSE handler did not stop after cancellation")
	}

	body := recorder.Body.String()
	if !strings.Contains(body, "event: snapshot\n") ||
		!strings.Contains(body, "id: ") ||
		!strings.Contains(body, `"drone_id":"UAV-0001"`) {
		t.Fatalf("unexpected SSE response: %q", body)
	}
}

func TestServeSSEChunkedWritesReassemblableParts(t *testing.T) {
	cache := memory.NewMemoryCache(4)
	for i := 0; i < 501; i++ {
		update := testUpdate(float64(i))
		update.DroneID = fmt.Sprintf("UAV-%04d", i)
		cache.Set(update)
	}
	bridge := NewStreamBridge(cache, 5*time.Millisecond)
	defer bridge.Close()

	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest("GET", "/v1/drones/stream?chunked=1", nil).
		WithContext(ctx)
	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		bridge.ServeSSE(recorder, request)
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("chunked SSE handler did not stop after cancellation")
	}

	body := recorder.Body.String()
	if !strings.Contains(body, "event: snapshot-part-") ||
		!strings.Contains(body, "-0-2\n") ||
		!strings.Contains(body, "-1-2\n") {
		t.Fatalf("unexpected chunked SSE response: %q", body[:min(len(body), 500)])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func BenchmarkSSESnapshot10K(b *testing.B) {
	updates := make([]*domain.DronePositionUpdate, 10_000)
	now := time.Now().UTC()
	for i := range updates {
		updates[i] = &domain.DronePositionUpdate{
			DroneID:   fmt.Sprintf("UAV-%04d", i),
			Type:      domain.DroneType((i % 3) + 1),
			OrbitType: domain.OrbitType((i % 3) + 1),
			Position: domain.Vector3D{
				Latitude:  float64(i%140) - 70,
				Longitude: float64(i%360) - 180,
				Altitude:  1000,
			},
			SpeedMS: 45, HeadingDeg: 180, Timestamp: now,
		}
	}

	b.ReportAllocs()
	encoder := json.NewEncoder(io.Discard)
	encoder.SetEscapeHTML(false)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filtered := filterUpdates(updates, domain.DroneTypeUnspecified, "")
		if err := encoder.Encode(filtered); err != nil {
			b.Fatal(err)
		}
	}
}
