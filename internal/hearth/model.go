// Package hearth implements the Symbiont Ephemeral Hearth (T-805,
// symbiont_geo_playstyle.md §9): a temporary muster point for a coordinated
// raid on one dome. It is deliberately NOT a stationary base or a graph root
// (RED LINE #1) — it expires on its own and buffs any Symbiont standing
// nearby while it lasts, giving the faction its first Social mechanic
// (Octalysis Core Drive 5, previously at zero — ai/octalysis_koster_review.md §1).
package hearth

import "time"

// Tuning (provisional; balance pass later, same as T-765/T-779).
const (
	TTLMin      = 15 // minutes the hearth stays active after summon
	CooldownMin = 60 // minutes before the same player may summon another, from creation
	BuffRadiusM = 150.0

	// DamageBonus is the flat Overload damage bump for a Symbiont standing
	// within BuffRadiusM of any active hearth — the coordinated-raid payoff.
	// Separate from the L5 roster verb-bonus track (roster.VerbBonus).
	DamageBonus = 20
)

// EphemeralHearth is one active or expired muster point.
type EphemeralHearth struct {
	ID        string
	OwnerID   string
	Lat       float64
	Lng       float64
	CreatedAt time.Time
	ExpiresAt time.Time
}
