package domain

import "time"

type DroneType int32

const (
	DroneTypeUnspecified DroneType = 0
	DroneTypeEnemy       DroneType = 1
	DroneTypeAlly        DroneType = 2
	DroneTypeUndefined   DroneType = 3
)

type OrbitType int32

const (
	OrbitTypeUnspecified OrbitType = 0
	OrbitTypeCircle      OrbitType = 1
	OrbitTypeStraight    OrbitType = 2
	OrbitTypeZigzag      OrbitType = 3
)

type Vector3D struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Altitude  float64 `json:"altitude"`
}

type DronePositionUpdate struct {
	DroneID    string    `json:"drone_id"`
	Type       DroneType `json:"type"`
	OrbitType  OrbitType `json:"orbit_type"`
	Position   Vector3D  `json:"position"`
	SpeedMS    float64   `json:"speed_m_s"`
	HeadingDeg float64   `json:"heading_deg"`
	Timestamp  time.Time `json:"timestamp"`
}

type FilterParams struct {
	TypeFilter  DroneType `json:"type_filter"`
	SearchQuery string    `json:"search_query"`
}
