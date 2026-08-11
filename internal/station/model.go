// Package station implements player-built power plants (energy pillar, T-777):
// generators that raise the local beacon capacity so players can build where the
// OSM grid is sparse. Anti-geography-nullification: a per-radius station cap +
// spacing + the global HARD_CAP keep a barren area improving without overtaking
// a dense centre.
package station

import "time"

// Tuning (provisional; balance T-779).
const (
	CapacityBonus = 2   // capacity each active station adds to its area
	CostMaterials = 100 // build cost — deliberately expensive (not Ecap, anti-P2W)
	CostEnergy    = 200
	MinSpacingM   = 50.0  // no two plants closer than this
	MaxInRadius   = 2     // at most this many plants within the beacon-overload radius
	BuildRadiusM  = 100.0 // = canon.TowerOverloadRadiusM (the capacity zone)

	// Lifecycle (T-778): a plant runs active for ActiveDays after its last
	// upkeep, then degrades, then collapses to ruins (no capacity). Servicing
	// it (UpkeepCostMaterials) resets the clock.
	ActiveDays          = 7
	DegradeDays         = 3 // extra days in 'degrading' before 'ruins'
	UpkeepCostMaterials = 40
)

// PowerPlant is one built generator.
type PowerPlant struct {
	ID            string    `json:"id"`
	OwnerID       string    `json:"owner_id"`
	Lat           float64   `json:"lat"`
	Lng           float64   `json:"lng"`
	CapacityBonus int       `json:"capacity_bonus"`
	State         string    `json:"state"`
	CreatedAt     time.Time `json:"created_at"`
}
