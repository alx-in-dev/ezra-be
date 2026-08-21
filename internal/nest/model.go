// Package nest implements the Symbiont home Nest (N3): the mirror of the human
// Core that finally gives the Symbiont faction Ownership (CD4) and Loss (CD8).
// A Nest is player-owned (cap 1 live/player), opened for free at onboarding,
// relocatable, emits an infection/territory aura, trickles Resonance into a
// buffer, and defends via a soft-timer siege that a raid can never close in one
// blow. See architecture.md ADR-N3-1..11. It borrows hive's SQL idioms but is a
// separate table/domain (ADR-N3-1) because its ownership lifecycle differs.
package nest

import (
	"time"

	"github.com/ezra-game/server/internal/canon"
)

// Siege states (ADR-N3-4). The state-machine lives on the nest row; only
// nest:tick applies the terminal collapse (single-writer invariant).
const (
	SiegeHealthy    = "healthy"
	SiegeUnderSiege = "under_siege"
	SiegeCollapsing = "collapsing"
	SiegeCollapsed  = "collapsed"
)

// Nest is a player's stationary Symbiont home.
type Nest struct {
	ID      string  `json:"id" db:"id"`
	OwnerID string  `json:"owner_id" db:"owner_id"`
	CellID  string  `json:"cell_id" db:"cell_id"`
	Lat     float64 `json:"lat" db:"lat"`
	Lng     float64 `json:"lng" db:"lng"`
	Level   int     `json:"level" db:"level"`

	// AccruedResonance is the trickle buffer (ADR-N3-7): the ONLY thing lost on
	// collapse. Collected explicitly (buffer→profile) so a snapshot survives.
	AccruedResonance float64 `json:"accrued_resonance" db:"accrued_resonance"`
	// SupportLevel is 0..100 vitality (T-834): feeding restores it, neglect
	// decays it, degrading aura/trickle OUTPUT — never existence.
	SupportLevel float64 `json:"support_level" db:"support_level"`

	// Siege fields (ADR-N3-4).
	SiegeHP         float64    `json:"siege_hp" db:"siege_hp"`
	SiegeState      string     `json:"siege_state" db:"siege_state"`
	CollapseAt      *time.Time `json:"collapse_at,omitempty" db:"collapse_at"`
	SiegeAttackerID *string    `json:"siege_attacker_id,omitempty" db:"siege_attacker_id"`

	// RelocatedAt is nil until the first (free) move; doubles as the "has
	// relocated before" flag for pricing (mirror of cores.relocated_at).
	RelocatedAt *time.Time `json:"relocated_at,omitempty" db:"relocated_at"`

	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	CollapsedAt *time.Time `json:"collapsed_at,omitempty" db:"collapsed_at"`

	// Computed for the client (not persisted).
	AuraRadiusM float64 `json:"aura_radius_m" db:"-"`
}

// Config returns the level config (clamped to L1).
func (n *Nest) Config() canon.NestLevelConfig { return canon.NestConfig(n.Level) }

// IsUnderThreat reports whether the nest is actively being sieged (relocation
// is blocked and the client shows the defense timer).
func (n *Nest) IsUnderThreat() bool {
	return n.SiegeState == SiegeUnderSiege || n.SiegeState == SiegeCollapsing
}

// decorate fills computed fields for the client.
func (n *Nest) decorate() {
	n.AuraRadiusM = canon.NestAuraRadiusM(n.Level)
}
