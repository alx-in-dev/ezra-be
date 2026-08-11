package geo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// --- Haversine ---

func Test_Haversine_SamePoint(t *testing.T) {
	d := Haversine(55.7558, 37.6173, 55.7558, 37.6173)
	assert.Equal(t, 0.0, d)
}

func Test_Haversine_KnownDistance_MoscowToSamara(t *testing.T) {
	d := Haversine(55.7558, 37.6173, 53.1959, 50.1002)
	expected := 856_000.0 // ~856 km
	tolerance := expected * 0.01
	assert.InDelta(t, expected, d, tolerance, "Moscow-Samara distance should be ~856km ±1%%")
}

func Test_Haversine_ShortDistance_About50m(t *testing.T) {
	// ~50m offset in latitude: 0.00045 degrees ≈ 50m
	lat := 55.7558
	lng := 37.6173
	d := Haversine(lat, lng, lat+0.00045, lng)
	assert.InDelta(t, 50.0, d, 5.0, "short distance should be ~50m")
}

// --- ValidateSpeed ---

func Test_ValidateSpeed_NormalWalk(t *testing.T) {
	// 100m in 120s ≈ 3 km/h, max 50 km/h → ok
	lat1, lng1 := 55.7558, 37.6173
	lat2 := lat1 + 0.0009 // ~100m north
	lng2 := lng1
	err := ValidateSpeed(lat1, lng1, lat2, lng2, 120, 50)
	assert.NoError(t, err)
}

func Test_ValidateSpeed_TooFast(t *testing.T) {
	// 10km in 5s ≈ 7200 km/h → error
	lat1, lng1 := 55.7558, 37.6173
	lat2 := lat1 + 0.09 // ~10km north
	lng2 := lng1
	err := ValidateSpeed(lat1, lng1, lat2, lng2, 5, 50)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "impossible_speed")
}

func Test_ValidateSpeed_ZeroElapsed(t *testing.T) {
	err := ValidateSpeed(55.0, 37.0, 56.0, 38.0, 0, 50)
	assert.NoError(t, err)
}

// --- CellID ---

func Test_CellID_FormatGridSnap(t *testing.T) {
	id := CellID(55.75580, 37.61730)
	// Should match "%.5f_%.5f" format
	assert.Regexp(t, `^\d+\.\d{5}_\d+\.\d{5}$`, id)
}

func Test_CellID_SameCellForClosePoints(t *testing.T) {
	// Two points within the same ~50m grid cell
	id1 := CellID(55.75580, 37.61730)
	id2 := CellID(55.75582, 37.61732)
	assert.Equal(t, id1, id2, "close points should map to the same cell")
}

func Test_CellID_DifferentCellsFor100mApart(t *testing.T) {
	lat := 55.75580
	lng := 37.61730
	// ~100m north ≈ 0.0009 degrees latitude, well beyond one 0.00045 grid step
	id1 := CellID(lat, lng)
	id2 := CellID(lat+0.0009, lng)
	assert.NotEqual(t, id1, id2, "points 100m apart should be in different cells")
}

func Test_CellID_NegativeCoordinates(t *testing.T) {
	id := CellID(-33.8688, 151.2093)
	_ = id // should not panic
	assert.NotEmpty(t, id)
}
