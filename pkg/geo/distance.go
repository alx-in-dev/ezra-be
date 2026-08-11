package geo

import (
	"fmt"
	"math"
)

const earthRadiusM = 6371000.0

// Haversine returns distance in meters between two lat/lng points.
func Haversine(lat1, lng1, lat2, lng2 float64) float64 {
	dLat := toRad(lat2 - lat1)
	dLng := toRad(lng2 - lng1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*
			math.Sin(dLng/2)*math.Sin(dLng/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusM * c
}

// ValidateSpeed checks if movement between two points is physically possible.
// Returns error if speed exceeds maxKmH.
func ValidateSpeed(lat1, lng1, lat2, lng2 float64, elapsedSec float64, maxKmH float64) error {
	if elapsedSec <= 0 {
		return nil
	}
	dist := Haversine(lat1, lng1, lat2, lng2)
	speedKmH := (dist / 1000.0) / (elapsedSec / 3600.0)
	if speedKmH > maxKmH {
		return fmt.Errorf("impossible_speed: %.1f km/h exceeds max %.1f km/h", speedKmH, maxKmH)
	}
	return nil
}

// CellID generates a cell identifier from lat/lng rounded to 50m grid.
func CellID(lat, lng float64) string {
	// Round to ~50m grid (0.00045 degrees ~ 50m)
	gridLat := math.Round(lat/0.00045) * 0.00045
	gridLng := math.Round(lng/0.00045) * 0.00045
	return fmt.Sprintf("%.5f_%.5f", gridLat, gridLng)
}

func toRad(deg float64) float64 {
	return deg * math.Pi / 180
}
