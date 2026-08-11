package rift

import "time"

type Rift struct {
	ID           string     `json:"id" db:"id"`
	CellID       string     `json:"cell_id" db:"cell_id"`
	Type         string     `json:"type" db:"type"`
	Intensity    int        `json:"intensity" db:"intensity"`
	RadiusCells  int        `json:"radius_cells" db:"radius_cells"`
	SpiritsCount int        `json:"spirits_count" db:"spirits_count"`
	OpenedAt     time.Time  `json:"opened_at" db:"opened_at"`
	ClosedAt     *time.Time `json:"closed_at" db:"closed_at"`
}

// TypeConfig holds rift configuration per type.
type TypeConfig struct {
	MinInfection int
	RadiusCells  int
	MinSpirits   int
	MaxSpirits   int
}

var TypeConfigs = map[string]TypeConfig{
	"minor": {MinInfection: 75, RadiusCells: 1, MinSpirits: 3, MaxSpirits: 5},
	// Medium caps at 8 (was 10) so the worst case (1 humanoid + 7 small) stays
	// proportionate to its 5-fighter entry gate once weaknesses/roles are played
	// (C2; balance_tables.md — starting point, tune via playtest). Critical
	// stays the heavy encounter.
	"medium":   {MinInfection: 82, RadiusCells: 3, MinSpirits: 5, MaxSpirits: 8},
	"critical": {MinInfection: 90, RadiusCells: 7, MinSpirits: 10, MaxSpirits: 20},
}
