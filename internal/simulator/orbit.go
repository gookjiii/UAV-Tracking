package simulator

import (
	"math"
	"time"

	"github.com/uav_tracking/internal/domain"
)

const (
	MetersPerDegreeLat = 111320.0
	EarthRadiusMeters  = 6371000.0
)

type OrbitState struct {
	DroneID     string    `json:"drone_id"`
	Type        int32     `json:"type"`
	OrbitType   int32     `json:"orbit_type"`
	CenterLat   float64   `json:"center_lat"`
	CenterLon   float64   `json:"center_lon"`
	BaseAlt     float64   `json:"base_alt"`
	Radius      float64   `json:"radius"`
	SpeedMS     float64   `json:"speed_m_s"`
	HeadingDeg  float64   `json:"heading_deg"`
	Amplitude   float64   `json:"amplitude"`
	Frequency   float64   `json:"frequency"`
	PhaseOffset float64   `json:"phase_offset"`
	StartTime   time.Time `json:"start_time"`
}

// ComputePosition returns updated (Lat, Lon, Alt, Heading, Speed) at time t
func (os *OrbitState) ComputePosition(now time.Time) (domain.Vector3D, float64, float64) {
	dt := now.Sub(os.StartTime).Seconds()

	switch os.OrbitType {
	case 1: // CIRCLE
		// Angular velocity omega = speed / radius (rad/s)
		if os.Radius <= 0 {
			os.Radius = 500.0
		}
		omega := os.SpeedMS / os.Radius
		angle := os.PhaseOffset + omega*dt

		dLat := (os.Radius * math.Cos(angle)) / MetersPerDegreeLat
		cosLat := math.Cos(os.CenterLat * math.Pi / 180.0)
		if cosLat == 0 {
			cosLat = 1.0
		}
		dLon := (os.Radius * math.Sin(angle)) / (MetersPerDegreeLat * cosLat)

		alt := os.BaseAlt + 20.0*math.Sin(0.1*dt)
		heading := math.Mod(((-angle*180.0/math.Pi)+90.0)+360.0, 360.0)

		return domain.Vector3D{
			Latitude:  os.CenterLat + dLat,
			Longitude: os.CenterLon + dLon,
			Altitude:  alt,
		}, os.SpeedMS, heading

	case 3: // ZIGZAG
		// Straight forward displacement + perpendicular wave offset
		dist := os.SpeedMS * dt
		headingRad := os.HeadingDeg * math.Pi / 180.0

		// Forward displacement vector
		fwdLat := (dist * math.Cos(headingRad)) / MetersPerDegreeLat
		cosLat := math.Cos(os.CenterLat * math.Pi / 180.0)
		if cosLat == 0 {
			cosLat = 1.0
		}
		fwdLon := (dist * math.Sin(headingRad)) / (MetersPerDegreeLat * cosLat)

		// Perpendicular wave displacement
		perpRad := headingRad + math.Pi/2.0
		waveOffset := os.Amplitude * math.Sin(2*math.Pi*os.Frequency*dt+os.PhaseOffset)

		perpLat := (waveOffset * math.Cos(perpRad)) / MetersPerDegreeLat
		perpLon := (waveOffset * math.Sin(perpRad)) / (MetersPerDegreeLat * cosLat)

		alt := os.BaseAlt + 15.0*math.Sin(0.2*dt)
		curHeading := math.Mod(os.HeadingDeg+15.0*math.Cos(2*math.Pi*os.Frequency*dt)+360.0, 360.0)

		return domain.Vector3D{
			Latitude:  os.CenterLat + fwdLat + perpLat,
			Longitude: os.CenterLon + fwdLon + perpLon,
			Altitude:  alt,
		}, os.SpeedMS, curHeading

	default: // 2: STRAIGHT (or default)
		dist := os.SpeedMS * dt
		headingRad := os.HeadingDeg * math.Pi / 180.0

		dLat := (dist * math.Cos(headingRad)) / MetersPerDegreeLat
		cosLat := math.Cos(os.CenterLat * math.Pi / 180.0)
		if cosLat == 0 {
			cosLat = 1.0
		}
		dLon := (dist * math.Sin(headingRad)) / (MetersPerDegreeLat * cosLat)

		alt := os.BaseAlt + 5.0*math.Sin(0.05*dt)

		return domain.Vector3D{
			Latitude:  os.CenterLat + dLat,
			Longitude: os.CenterLon + dLon,
			Altitude:  alt,
		}, os.SpeedMS, os.HeadingDeg
	}
}
