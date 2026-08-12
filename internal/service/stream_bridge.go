package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/uav_tracking/internal/domain"
	"github.com/uav_tracking/internal/memory"
)

type StreamBridge struct {
	cache       *memory.MemoryCache
	subscribers map[chan streamSnapshot]bool
	mu          sync.RWMutex
	interval    time.Duration
	sequence    uint64
	stop        chan struct{}
	closeOnce   sync.Once
	wg          sync.WaitGroup
}

type streamSnapshot struct {
	sequence uint64
	updates  []*domain.DronePositionUpdate
}

func NewStreamBridge(cache *memory.MemoryCache, intervals ...time.Duration) *StreamBridge {
	interval := 300 * time.Millisecond
	if len(intervals) > 0 && intervals[0] > 0 {
		interval = intervals[0]
	}
	sb := &StreamBridge{
		cache:       cache,
		subscribers: make(map[chan streamSnapshot]bool),
		interval:    interval,
		stop:        make(chan struct{}),
	}
	sb.wg.Add(1)
	go sb.broadcastLoop()
	return sb
}

func (sb *StreamBridge) broadcastLoop() {
	defer sb.wg.Done()
	ticker := time.NewTicker(sb.interval)
	defer ticker.Stop()

	for {
		select {
		case <-sb.stop:
			return
		case <-ticker.C:
		}
		sb.mu.RLock()
		if len(sb.subscribers) == 0 {
			sb.mu.RUnlock()
			continue
		}
		sb.mu.RUnlock()

		drones := sb.cache.GetFiltered(domain.DroneTypeUnspecified, "")
		sb.mu.Lock()
		sb.sequence++
		snapshot := streamSnapshot{sequence: sb.sequence, updates: drones}
		for ch := range sb.subscribers {
			select {
			case ch <- snapshot:
			default:
				// Replace a stale snapshot so slow clients always receive the latest state.
				select {
				case <-ch:
				default:
				}
				select {
				case ch <- snapshot:
				default:
				}
			}
		}
		sb.mu.Unlock()
	}
}

func (sb *StreamBridge) ServeSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	// Read optional query filter
	typeStr := r.URL.Query().Get("type")
	var typeFilter domain.DroneType
	if t, err := strconv.Atoi(typeStr); err == nil {
		typeFilter = domain.DroneType(t)
	}
	searchQuery := r.URL.Query().Get("search")
	chunked := r.URL.Query().Get("chunked") == "1"
	streamLimit := 0
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil && parsed > 0 {
			streamLimit = parsed
		}
	}

	clientChan := make(chan streamSnapshot, 1)

	sb.mu.Lock()
	sb.subscribers[clientChan] = true
	sb.mu.Unlock()

	defer func() {
		sb.mu.Lock()
		delete(sb.subscribers, clientChan)
		sb.mu.Unlock()
		close(clientChan)
	}()

	notify := r.Context().Done()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-notify:
			return
		case <-heartbeat.C:
			fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
		case snapshot := <-clientChan:
			filtered := filterUpdates(snapshot.updates, typeFilter, searchQuery)
			if streamLimit > 0 && len(filtered) > streamLimit {
				// Sharded map iteration is intentionally unordered; sort before
				// truncating so the remote demo keeps a stable visible sample.
				sort.Slice(filtered, func(i, j int) bool {
					return filtered[i].DroneID < filtered[j].DroneID
				})
				filtered = filtered[:streamLimit]
			}

			var err error
			if chunked {
				err = writeSnapshotParts(w, flusher, snapshot.sequence, filtered)
			} else {
				err = writeSnapshotEvent(w, flusher, snapshot.sequence, filtered)
			}
			if err != nil {
				return
			}
		}
	}
}

// writeSnapshotParts keeps each SSE event below the size limits imposed by
// public reverse proxies. The data of every part is still a JSON array; the
// event name carries enough metadata for a client that opted into chunking to
// reassemble the original snapshot.
func writeSnapshotParts(
	w http.ResponseWriter,
	flusher http.Flusher,
	sequence uint64,
	updates []*domain.DronePositionUpdate,
) error {
	const partSize = 500
	total := (len(updates) + partSize - 1) / partSize
	if total == 0 {
		total = 1
	}
	for part := 0; part < total; part++ {
		start := part * partSize
		end := start + partSize
		if end > len(updates) {
			end = len(updates)
		}
		name := fmt.Sprintf("snapshot-part-%d-%d-%d", sequence, part, total)
		if _, err := fmt.Fprintf(w, "event: %s\ndata: ", name); err != nil {
			return err
		}
		encoded, err := json.Marshal(updates[start:end])
		if err != nil {
			return err
		}
		if _, err := w.Write(encoded); err != nil {
			return err
		}
		if _, err := fmt.Fprint(w, "\n\n"); err != nil {
			return err
		}
		flusher.Flush()
	}
	return nil
}

// writeSnapshotEvent deliberately emits the JSON array as many complete SSE
// data lines instead of one multi-megabyte write. Some reverse proxies (and
// ngrok's free edge) reset a streaming response when a single chunk is too
// large. Newlines between data lines are legal JSON whitespace, so existing
// SSE clients that join data lines continue to receive the same JSON array.
func writeSnapshotEvent(
	w http.ResponseWriter,
	flusher http.Flusher,
	sequence uint64,
	updates []*domain.DronePositionUpdate,
) error {
	if _, err := fmt.Fprintf(w, "id: %d\nevent: snapshot\ndata: [\n", sequence); err != nil {
		return err
	}
	flusher.Flush()

	const maxChunk = 64 * 1024
	var chunk bytes.Buffer
	for i, update := range updates {
		encoded, err := json.Marshal(update)
		if err != nil {
			return err
		}
		if i > 0 {
			chunk.WriteString("data: ,\n")
		}
		chunk.WriteString("data: ")
		chunk.Write(encoded)
		chunk.WriteByte('\n')
		if chunk.Len() >= maxChunk {
			if _, err := chunk.WriteTo(w); err != nil {
				return err
			}
			flusher.Flush()
		}
	}
	if chunk.Len() > 0 {
		if _, err := chunk.WriteTo(w); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprint(w, "data: ]\n\n"); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func (sb *StreamBridge) Close() {
	sb.closeOnce.Do(func() {
		close(sb.stop)
		sb.wg.Wait()
	})
}

func filterUpdates(updates []*domain.DronePositionUpdate, typeFilter domain.DroneType, searchQuery string) []*domain.DronePositionUpdate {
	searchQuery = strings.ToLower(strings.TrimSpace(searchQuery))
	if typeFilter == domain.DroneTypeUnspecified && searchQuery == "" {
		return updates
	}
	filtered := make([]*domain.DronePositionUpdate, 0, len(updates))
	for _, update := range updates {
		if typeFilter != domain.DroneTypeUnspecified && update.Type != typeFilter {
			continue
		}
		if searchQuery != "" && !strings.Contains(strings.ToLower(update.DroneID), searchQuery) {
			continue
		}
		filtered = append(filtered, update)
	}
	return filtered
}
