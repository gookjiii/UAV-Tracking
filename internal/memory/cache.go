package memory

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/uav_tracking/internal/domain"
)

const (
	ShardCount        = 32
	DefaultMaxHistory = 500
)

type MemoryCache struct {
	shards     []*cacheShard
	maxHistory int
	lastUpdate atomic.Int64
}

type cacheShard struct {
	mu      sync.RWMutex
	items   map[string]*domain.DronePositionUpdate
	history map[string]*historyRing
}

type historyPoint struct {
	Type       domain.DroneType
	OrbitType  domain.OrbitType
	Position   domain.Vector3D
	SpeedMS    float64
	HeadingDeg float64
	Timestamp  time.Time
}

type historyRing struct {
	points []historyPoint
	start  int
	length int
}

func NewMemoryCache(historyPoints ...int) *MemoryCache {
	maxHistory := DefaultMaxHistory
	if len(historyPoints) > 0 && historyPoints[0] > 0 {
		maxHistory = historyPoints[0]
	}
	mc := &MemoryCache{
		shards:     make([]*cacheShard, ShardCount),
		maxHistory: maxHistory,
	}
	for i := 0; i < ShardCount; i++ {
		mc.shards[i] = &cacheShard{
			items:   make(map[string]*domain.DronePositionUpdate),
			history: make(map[string]*historyRing),
		}
	}
	return mc
}

func (mc *MemoryCache) getShard(key string) *cacheShard {
	idx := shardIndex(key)
	return mc.shards[idx]
}

func shardIndex(key string) uint32 {
	const offset32 = uint32(2166136261)
	const prime32 = uint32(16777619)
	hash := offset32
	for i := 0; i < len(key); i++ {
		hash ^= uint32(key[i])
		hash *= prime32
	}
	return hash % ShardCount
}

func (mc *MemoryCache) Set(update *domain.DronePositionUpdate) {
	if update == nil || update.DroneID == "" {
		return
	}
	shard := mc.getShard(update.DroneID)
	shard.mu.Lock()
	mc.setLocked(shard, update)
	shard.mu.Unlock()
	mc.lastUpdate.Store(update.Timestamp.UnixNano())
}

func (mc *MemoryCache) SetBatch(updates []*domain.DronePositionUpdate) {
	if len(updates) == 0 {
		return
	}
	groups := make([][]*domain.DronePositionUpdate, ShardCount)
	var newest time.Time
	for _, update := range updates {
		if update == nil || update.DroneID == "" {
			continue
		}
		idx := shardIndex(update.DroneID)
		groups[idx] = append(groups[idx], update)
		if update.Timestamp.After(newest) {
			newest = update.Timestamp
		}
	}
	for i, group := range groups {
		if len(group) == 0 {
			continue
		}
		shard := mc.shards[i]
		shard.mu.Lock()
		for _, update := range group {
			mc.setLocked(shard, update)
		}
		shard.mu.Unlock()
	}
	if !newest.IsZero() {
		mc.lastUpdate.Store(newest.UnixNano())
	}
}

func (mc *MemoryCache) setLocked(shard *cacheShard, update *domain.DronePositionUpdate) {
	shard.items[update.DroneID] = update
	ring := shard.history[update.DroneID]
	if ring == nil {
		ring = &historyRing{points: make([]historyPoint, mc.maxHistory)}
		shard.history[update.DroneID] = ring
	}
	point := historyPoint{
		Type:       update.Type,
		OrbitType:  update.OrbitType,
		Position:   update.Position,
		SpeedMS:    update.SpeedMS,
		HeadingDeg: update.HeadingDeg,
		Timestamp:  update.Timestamp,
	}
	if ring.length < len(ring.points) {
		idx := (ring.start + ring.length) % len(ring.points)
		ring.points[idx] = point
		ring.length++
		return
	}
	ring.points[ring.start] = point
	ring.start = (ring.start + 1) % len(ring.points)
}

func (mc *MemoryCache) Get(droneID string) (*domain.DronePositionUpdate, bool) {
	shard := mc.getShard(droneID)
	shard.mu.RLock()
	val, ok := shard.items[droneID]
	shard.mu.RUnlock()
	return val, ok
}

func (mc *MemoryCache) GetHistory(droneID string, maxPoints int) []*domain.DronePositionUpdate {
	shard := mc.getShard(droneID)
	shard.mu.RLock()
	defer shard.mu.RUnlock()

	ring, ok := shard.history[droneID]
	if !ok || ring.length == 0 {
		return []*domain.DronePositionUpdate{}
	}

	count := ring.length
	startOffset := 0
	if maxPoints > 0 && count > maxPoints {
		startOffset = count - maxPoints
		count = maxPoints
	}
	result := make([]*domain.DronePositionUpdate, 0, count)
	for i := 0; i < count; i++ {
		point := ring.points[(ring.start+startOffset+i)%len(ring.points)]
		result = append(result, &domain.DronePositionUpdate{
			DroneID: droneID, Type: point.Type, OrbitType: point.OrbitType,
			Position: point.Position, SpeedMS: point.SpeedMS,
			HeadingDeg: point.HeadingDeg, Timestamp: point.Timestamp,
		})
	}
	return result
}

func (mc *MemoryCache) GetFiltered(typeFilter domain.DroneType, searchQuery string) []*domain.DronePositionUpdate {
	searchQuery = strings.ToLower(strings.TrimSpace(searchQuery))
	results := make([]*domain.DronePositionUpdate, 0, 1000)

	for i := 0; i < ShardCount; i++ {
		shard := mc.shards[i]
		shard.mu.RLock()
		for _, item := range shard.items {
			if typeFilter != domain.DroneTypeUnspecified && item.Type != typeFilter {
				continue
			}
			if searchQuery != "" && !strings.Contains(strings.ToLower(item.DroneID), searchQuery) {
				continue
			}
			results = append(results, item)
		}
		shard.mu.RUnlock()
	}

	return results
}

func (mc *MemoryCache) Count() int {
	total := 0
	for i := 0; i < ShardCount; i++ {
		shard := mc.shards[i]
		shard.mu.RLock()
		total += len(shard.items)
		shard.mu.RUnlock()
	}
	return total
}

func (mc *MemoryCache) LastUpdated() time.Time {
	nanos := mc.lastUpdate.Load()
	if nanos == 0 {
		return time.Time{}
	}
	return time.Unix(0, nanos).UTC()
}

func (mc *MemoryCache) Clear() {
	for i := 0; i < ShardCount; i++ {
		shard := mc.shards[i]
		shard.mu.Lock()
		shard.items = make(map[string]*domain.DronePositionUpdate)
		shard.history = make(map[string]*historyRing)
		shard.mu.Unlock()
	}
	mc.lastUpdate.Store(0)
}
