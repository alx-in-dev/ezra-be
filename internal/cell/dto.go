package cell

// TowerSummary is a lightweight tower snapshot embedded in cell responses.
type TowerSummary struct {
	ID                string  `json:"id"`
	OwnerID           string  `json:"owner_id"`
	Lat               float64 `json:"lat"`
	Lng               float64 `json:"lng"`
	Level             int     `json:"level"`
	RadiusM           float64 `json:"radius_m"`
	EffectPerHour     float64 `json:"effect_per_hour"`
	HP                int     `json:"hp"`
	HPMax             int     `json:"hp_max"`
	State             string  `json:"state"`
	OverloadCount     int     `json:"overload_count"`
	OverloadLimit     int     `json:"overload_limit"`
	SafeZoneState     string  `json:"safe_zone_state"`
	SafeZoneInfection float64 `json:"safe_zone_infection"`
}

// CellDTO is the public representation of a cell returned by the API.
type CellDTO struct {
	ID                string        `json:"id"`
	Lat               float64       `json:"lat"`
	Lng               float64       `json:"lng"`
	Infection         float64       `json:"infection"`
	Terrain           string        `json:"terrain"`
	TowerID           *string       `json:"tower_id"`
	RiftID            *string       `json:"rift_id"`
	Tower             *TowerSummary `json:"tower,omitempty"`
	HasNearbyBuilding bool          `json:"has_nearby_building"`
	HasNearbyRoad     bool          `json:"has_nearby_road"`
	NearestRoadClass  *string       `json:"nearest_road_class,omitempty"`
}
