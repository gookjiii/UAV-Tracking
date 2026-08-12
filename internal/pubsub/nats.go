package pubsub

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/uav_tracking/internal/domain"
)

const (
	StreamName    = "DRONE_UPDATES"
	SubjectPrefix = "drones.position"
)

type NATSService struct {
	nc *nats.Conn
}

func NewNATSService(url string, maxBytes ...int64) (*NATSService, error) {
	// Attempt connection with retry
	var nc *nats.Conn
	var err error
	for i := 0; i < 5; i++ {
		nc, err = nats.Connect(url,
			nats.Name("UAV-Tracking-Service"),
			nats.ReconnectWait(1*time.Second),
			nats.MaxReconnects(10),
		)
		if err == nil {
			break
		}
		log.Printf("Waiting for NATS server at %s... (attempt %d/5)", url, i+1)
		time.Sleep(1 * time.Second)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS at %s: %w", url, err)
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("failed to get JetStream context: %w", err)
	}

	streamMaxBytes := int64(128 * 1024 * 1024)
	if len(maxBytes) > 0 && maxBytes[0] > 0 {
		streamMaxBytes = maxBytes[0]
	}
	streamConfig := &nats.StreamConfig{
		Name:              StreamName,
		Subjects:          []string{SubjectPrefix + ".*", SubjectPrefix},
		Storage:           nats.MemoryStorage,
		Retention:         nats.LimitsPolicy,
		Discard:           nats.DiscardOld,
		MaxAge:            time.Hour,
		MaxBytes:          streamMaxBytes,
		MaxMsgsPerSubject: 1,
		Duplicates:        time.Minute,
	}
	stream, infoErr := js.StreamInfo(StreamName)
	if infoErr != nil || stream == nil {
		_, err = js.AddStream(streamConfig)
	} else {
		_, err = js.UpdateStream(streamConfig)
	}
	if err != nil {
		log.Printf("Warning: failed to configure JetStream stream; core NATS publishing remains available: %v", err)
	} else {
		log.Printf("JetStream stream %s configured with one latest message per UAV", StreamName)
	}

	return &NATSService{nc: nc}, nil
}

func (s *NATSService) PublishPosition(update *domain.DronePositionUpdate) error {
	data, err := json.Marshal(update)
	if err != nil {
		return err
	}
	subject := fmt.Sprintf("%s.%s", SubjectPrefix, update.DroneID)
	return s.nc.Publish(subject, data)
}

func (s *NATSService) PublishBatch(updates []*domain.DronePositionUpdate) error {
	for _, up := range updates {
		if err := s.PublishPosition(up); err != nil {
			return err
		}
	}
	return nil
}

func (s *NATSService) Healthy() bool {
	return s != nil && s.nc != nil && s.nc.IsConnected()
}

func (s *NATSService) Close() {
	if s.nc != nil {
		s.nc.Close()
	}
}
