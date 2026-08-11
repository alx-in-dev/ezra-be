package tower

import (
	"time"

	"github.com/ezra-game/server/internal/canon"
)

// Tower represents a player-placed structure. It stands at its own lat/lng
// (free placement); cell_id anchors it to the infection grid for
// pressure/suppression bookkeeping.
type Tower struct {
	ID                string    `json:"id" db:"id"`
	CellID            string    `json:"cell_id" db:"cell_id"`
	Lat               float64   `json:"lat" db:"lat"`
	Lng               float64   `json:"lng" db:"lng"`
	OwnerID           string    `json:"owner_id" db:"owner_id"`
	Level             int       `json:"level" db:"level"`
	Type              string    `json:"type" db:"type"`
	HP                int       `json:"hp" db:"hp"`
	HPMax             int       `json:"hp_max" db:"hp_max"`
	RadiusM           float64   `json:"radius_m" db:"radius_m"`
	EffectPerHour     float64   `json:"effect_per_hour" db:"effect_per_hour"`
	InstalledAt       time.Time `json:"installed_at" db:"installed_at"`
	UpgradedAt        time.Time `json:"upgraded_at" db:"upgraded_at"`
	// Brownout mirrors towers.brownout (network over-budget verdict); filled
	// only by queries that select it explicitly (db:"-" keeps the shared
	// column list untouched).
	Brownout          bool      `json:"brownout,omitempty" db:"-"`
	State             string    `json:"state,omitempty" db:"-"`
	OverloadCount     int       `json:"overload_count,omitempty" db:"-"`
	OverloadLimit     int       `json:"overload_limit,omitempty" db:"-"`
	SafeZoneState     string    `json:"safe_zone_state,omitempty" db:"-"`
	SafeZoneInfection float64   `json:"safe_zone_infection" db:"-"`
}

// LevelConfig defines stats and costs for a tower level.
type LevelConfig struct {
	RadiusM          float64
	EffectPerHour    float64
	HPMax            int
	EnergyCost       int
	MaterialCost     int
	EnergyPerHour    int
	MaterialsPerHour int
	BuildTimeSec     int
}

// LevelConfigs maps tower level to its configuration.
var LevelConfigs = map[int]LevelConfig{
	1: levelConfigFromCanon(canon.TowerLevelSpecs[1]),
	2: levelConfigFromCanon(canon.TowerLevelSpecs[2]),
	3: levelConfigFromCanon(canon.TowerLevelSpecs[3]),
	4: levelConfigFromCanon(canon.TowerLevelSpecs[4]),
	5: levelConfigFromCanon(canon.TowerLevelSpecs[5]),
}

func levelConfigFromCanon(spec canon.TowerLevelSpec) LevelConfig {
	return LevelConfig{
		RadiusM:          spec.RadiusM,
		EffectPerHour:    spec.EffectPerHour,
		HPMax:            spec.HPMax,
		EnergyCost:       spec.EnergyCost,
		MaterialCost:     spec.MaterialCost,
		EnergyPerHour:    spec.EnergyPerHour,
		MaterialsPerHour: spec.MaterialsPerHour,
		BuildTimeSec:     int(spec.BuildTime / time.Second),
	}
}
