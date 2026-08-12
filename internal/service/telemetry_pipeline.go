package service

import (
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/uav_tracking/internal/domain"
	"github.com/uav_tracking/internal/memory"
)

type batchPublisher interface {
	PublishBatch([]*domain.DronePositionUpdate) error
}

type batchRepository interface {
	SaveBatch([]*domain.DronePositionUpdate) int
}

// TelemetryPipeline fans one immutable simulator snapshot out to the cache,
// sampled history, and NATS without allowing a slow optional dependency to
// block the next simulation tick.
type TelemetryPipeline struct {
	cache          *memory.MemoryCache
	repo           batchRepository
	publisher      batchPublisher
	sampleInterval time.Duration
	lastSample     time.Time
	publishQueue   chan []*domain.DronePositionUpdate
	stop           chan struct{}
	wg             sync.WaitGroup
	droppedNATS    atomic.Uint64
	droppedDB      atomic.Uint64
}

func NewTelemetryPipeline(
	cache *memory.MemoryCache,
	repo batchRepository,
	publisher batchPublisher,
	sampleInterval time.Duration,
) *TelemetryPipeline {
	pipeline := &TelemetryPipeline{
		cache:          cache,
		repo:           repo,
		publisher:      publisher,
		sampleInterval: sampleInterval,
		publishQueue:   make(chan []*domain.DronePositionUpdate, 1),
		stop:           make(chan struct{}),
	}
	if publisher != nil {
		pipeline.wg.Add(1)
		go pipeline.publishLoop()
	}
	return pipeline
}

func (p *TelemetryPipeline) ConsumeBatch(updates []*domain.DronePositionUpdate) {
	if len(updates) == 0 {
		return
	}
	p.cache.SetBatch(updates)

	if p.publisher != nil {
		select {
		case p.publishQueue <- updates:
		default:
			select {
			case <-p.publishQueue:
				p.droppedNATS.Add(1)
			default:
			}
			select {
			case p.publishQueue <- updates:
			default:
				p.droppedNATS.Add(1)
			}
		}
	}

	if p.repo != nil {
		now := updates[0].Timestamp
		if p.lastSample.IsZero() || now.Sub(p.lastSample) >= p.sampleInterval {
			p.lastSample = now
			if dropped := p.repo.SaveBatch(updates); dropped > 0 {
				p.droppedDB.Add(uint64(dropped))
			}
		}
	}
}

func (p *TelemetryPipeline) publishLoop() {
	defer p.wg.Done()
	for {
		select {
		case <-p.stop:
			return
		case updates := <-p.publishQueue:
			if err := p.publisher.PublishBatch(updates); err != nil {
				log.Printf("NATS snapshot publish failed: %v", err)
			}
		}
	}
}

func (p *TelemetryPipeline) DroppedNATSSnapshots() uint64 { return p.droppedNATS.Load() }
func (p *TelemetryPipeline) DroppedDBPoints() uint64      { return p.droppedDB.Load() }

func (p *TelemetryPipeline) Close() {
	if p.publisher == nil {
		return
	}
	close(p.stop)
	p.wg.Wait()
}
