package cell

import "time"

type Cell struct {
	ID                string    `json:"id" db:"id"`
	Lat               float64   `json:"lat" db:"lat"`
	Lng               float64   `json:"lng" db:"lng"`
	Infection         float64   `json:"infection" db:"infection"`
	Terrain           string    `json:"terrain" db:"terrain"`
	TowerID           *string   `json:"tower_id" db:"tower_id"`
	RiftID            *string   `json:"rift_id" db:"rift_id"`
	LastCalculated    time.Time `json:"last_calculated" db:"last_calculated"`
	HasNearbyBuilding bool      `json:"has_nearby_building" db:"has_nearby_building"`
	HasNearbyRoad     bool      `json:"has_nearby_road" db:"has_nearby_road"`
	NearestRoadClass  *string   `json:"nearest_road_class" db:"nearest_road_class"`
}
