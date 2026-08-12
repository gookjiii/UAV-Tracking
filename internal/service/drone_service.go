package service

import (
	"context"
	"sort"
	"time"

	"github.com/uav_tracking/api/proto/drone"
	"github.com/uav_tracking/internal/domain"
	"github.com/uav_tracking/internal/memory"
	"github.com/uav_tracking/internal/repository"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type SimulationControl interface {
	UpdateConfig(count int, intervalMs int, active bool) (activeCount int, success bool, reset bool, message string)
}

type DroneServer struct {
	dronepb.UnimplementedDroneServiceServer
	cache      *memory.MemoryCache
	repo       *repository.PostgresRepository
	simControl SimulationControl
}

func NewDroneServer(
	cache *memory.MemoryCache,
	repo *repository.PostgresRepository,
	simControl SimulationControl,
) *DroneServer {
	return &DroneServer{
		cache:      cache,
		repo:       repo,
		simControl: simControl,
	}
}

func (s *DroneServer) GetCurrentState(ctx context.Context, req *dronepb.GetCurrentStateRequest) (*dronepb.GetCurrentStateResponse, error) {
	typeFilter := domain.DroneType(req.GetTypeFilter())
	searchQuery := req.GetSearchQuery()

	items := s.cache.GetFiltered(typeFilter, searchQuery)
	respItems := make([]*dronepb.DronePositionUpdate, 0, len(items))

	for _, item := range items {
		respItems = append(respItems, toProtoUpdate(item))
	}

	return &dronepb.GetCurrentStateResponse{
		TotalCount: int32(len(respItems)),
		Drones:     respItems,
	}, nil
}

func (s *DroneServer) GetHistory(ctx context.Context, req *dronepb.GetHistoryRequest) (*dronepb.GetHistoryResponse, error) {
	if req.GetDroneId() == "" {
		return nil, status.Error(codes.InvalidArgument, "drone_id is required")
	}

	startTime := time.Now().AddDate(0, 0, -7)
	if req.GetStartTime() != nil {
		startTime = req.GetStartTime().AsTime()
	}

	endTime := time.Now()
	if req.GetEndTime() != nil {
		endTime = req.GetEndTime().AsTime()
	}

	maxPoints := int(req.GetMaxPoints())
	if maxPoints <= 0 {
		maxPoints = 500
	}
	if maxPoints > 5000 {
		maxPoints = 5000
	}
	if endTime.Before(startTime) {
		return nil, status.Error(codes.InvalidArgument, "end_time must be after start_time")
	}

	var persistent []*domain.DronePositionUpdate

	if s.repo != nil {
		var err error
		persistent, err = s.repo.GetHistory(ctx, req.GetDroneId(), startTime, endTime, maxPoints)
		if err != nil && s.cache == nil {
			return nil, status.Errorf(codes.Internal, "load history: %v", err)
		}
	}

	var recent []*domain.DronePositionUpdate
	if s.cache != nil {
		for _, item := range s.cache.GetHistory(req.GetDroneId(), maxPoints) {
			if !item.Timestamp.Before(startTime) && !item.Timestamp.After(endTime) {
				recent = append(recent, item)
			}
		}
	}
	history := mergeAndDownsample(persistent, recent, maxPoints)

	protoHistory := make([]*dronepb.DronePositionUpdate, 0, len(history))
	for _, item := range history {
		protoHistory = append(protoHistory, toProtoUpdate(item))
	}

	return &dronepb.GetHistoryResponse{
		DroneId: req.GetDroneId(),
		History: protoHistory,
	}, nil
}

func (s *DroneServer) ControlSimulation(ctx context.Context, req *dronepb.SimulationConfigRequest) (*dronepb.SimulationConfigResponse, error) {
	if s.simControl == nil {
		return &dronepb.SimulationConfigResponse{
			Success: false,
			Message: "Simulation control engine not attached",
		}, nil
	}

	activeCount, success, reset, msg := s.simControl.UpdateConfig(
		int(req.GetTargetDroneCount()),
		int(req.GetUpdateIntervalMs()),
		req.GetActive(),
	)

	if !success {
		return nil, status.Error(codes.InvalidArgument, msg)
	}

	if reset && s.cache != nil {
		s.cache.Clear()
	}

	return &dronepb.SimulationConfigResponse{
		Success:      success,
		ActiveDrones: int32(activeCount),
		Message:      msg,
	}, nil
}

func mergeAndDownsample(persistent, recent []*domain.DronePositionUpdate, maxPoints int) []*domain.DronePositionUpdate {
	combined := make([]*domain.DronePositionUpdate, 0, len(persistent)+len(recent))
	combined = append(combined, persistent...)
	combined = append(combined, recent...)
	sort.Slice(combined, func(i, j int) bool {
		return combined[i].Timestamp.Before(combined[j].Timestamp)
	})

	deduplicated := combined[:0]
	var previous time.Time
	for _, item := range combined {
		if item == nil || (!previous.IsZero() && item.Timestamp.Equal(previous)) {
			continue
		}
		deduplicated = append(deduplicated, item)
		previous = item.Timestamp
	}
	if maxPoints <= 0 || len(deduplicated) <= maxPoints {
		return deduplicated
	}
	if maxPoints == 1 {
		return deduplicated[len(deduplicated)-1:]
	}

	sampled := make([]*domain.DronePositionUpdate, 0, maxPoints)
	lastIndex := len(deduplicated) - 1
	for i := 0; i < maxPoints; i++ {
		idx := i * lastIndex / (maxPoints - 1)
		sampled = append(sampled, deduplicated[idx])
	}
	return sampled
}

func (s *DroneServer) StreamPositions(req *dronepb.GetCurrentStateRequest, stream dronepb.DroneService_StreamPositionsServer) error {
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()

	typeFilter := domain.DroneType(req.GetTypeFilter())
	searchQuery := req.GetSearchQuery()

	for {
		select {
		case <-stream.Context().Done():
			return nil
		case <-ticker.C:
			items := s.cache.GetFiltered(typeFilter, searchQuery)
			for _, item := range items {
				if err := stream.Send(toProtoUpdate(item)); err != nil {
					return err
				}
			}
		}
	}
}

func toProtoUpdate(item *domain.DronePositionUpdate) *dronepb.DronePositionUpdate {
	return &dronepb.DronePositionUpdate{
		DroneId:   item.DroneID,
		Type:      dronepb.DroneType(item.Type),
		OrbitType: dronepb.OrbitType(item.OrbitType),
		Position: &dronepb.Vector3D{
			Latitude:  item.Position.Latitude,
			Longitude: item.Position.Longitude,
			Altitude:  item.Position.Altitude,
		},
		SpeedMS:    item.SpeedMS,
		HeadingDeg: item.HeadingDeg,
		Timestamp:  timestamppb.New(item.Timestamp),
	}
}
