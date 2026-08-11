package hive

import "time"

// Hive (Очаг) is the Symbiont mirror of the human Core — the root of an
// infection region (E1). It pulses infection into its radius and spawns rifts
// in its zone; the D2 tide then channels that infection toward player networks.
type Hive struct {
	ID        string     `json:"id" db:"id"`
	CellID    string     `json:"cell_id" db:"cell_id"`
	Lat       float64    `json:"lat" db:"lat"`
	Lng       float64    `json:"lng" db:"lng"`
	Level     int        `json:"level" db:"level"`
	Intensity int        `json:"intensity" db:"intensity"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
	ClosedAt  *time.Time `json:"closed_at" db:"closed_at"`

	// RadiusM is the hive's zone radius (computed from level for the client).
	RadiusM float64 `json:"radius_m" db:"-"`
}

// LevelConfig holds per-level hive behaviour. Higher-level hives reach farther,
// pulse harder, and seed nastier rifts — escalating the regional threat.
type LevelConfig struct {
	RadiusM      float64
	PulsePerTick float64 // infection points added to in-zone cells per pulse
	RiftType     string  // rift type the hive seeds
}

var levelConfigs = map[int]LevelConfig{
	1: {RadiusM: 200, PulsePerTick: 4.0, RiftType: "minor"},
	2: {RadiusM: 350, PulsePerTick: 6.0, RiftType: "medium"},
	3: {RadiusM: 500, PulsePerTick: 9.0, RiftType: "critical"},
}

// Config returns the level config (clamped to L1).
func Config(level int) LevelConfig {
	if cfg, ok := levelConfigs[level]; ok {
		return cfg
	}
	return levelConfigs[1]
}

// decorate fills computed fields for the client.
func (h *Hive) decorate() {
	h.RadiusM = Config(h.Level).RadiusM
}
