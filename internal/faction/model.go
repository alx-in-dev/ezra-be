package faction

import "time"

// Sides.
const (
	Human    = "human"
	Symbiont = "symbiont"
)

// SwitchCooldown is the minimum time between allegiance flips. Free instant
// re-switching let players reset the legacy-network decay timer forever (a
// flip clears legacy_since, the next flip re-stamps it) and dodge every
// faction commitment. The first choice is never blocked.
const SwitchCooldown = 24 * time.Hour

// State is a player's faction-onboarding state.
type State struct {
	Faction   string `json:"faction"`
	Contacted bool   `json:"contacted"`  // Contact experienced → choice unlocked
	Chosen    bool   `json:"chosen"`     // an explicit allegiance was declared
	CanChoose bool   `json:"can_choose"` // contacted (choice available)
	// SymbiontPlayable is false until the v1.1 symbiont path ships — the client
	// shows the symbiont option as a teaser ("в разработке").
	SymbiontPlayable bool   `json:"symbiont_playable"`
	FactionRu        string `json:"faction_ru"`
	// Portable Symbiont Resonance (#6) — surfaced here so the faction screen
	// shows the live value without a separate profile fetch.
	SymbiontResonance int `json:"symbiont_resonance"`
	// Resonance roster progression (R4-2): the one-time human→Symbiont Pool, the
	// Resonance Level, its control-slot cap, and XP toward the next level.
	ResonancePool    int `json:"resonance_pool"`
	ResonanceLevel   int `json:"resonance_level"`
	ResonanceControl int `json:"resonance_control"`     // entity control-slot cap at this RL
	ResonanceXP      int `json:"resonance_xp"`
	ResonanceXPNext  int `json:"resonance_xp_next"`     // cumulative XP for next RL, -1 at max
}

func FactionRu(side string) string {
	switch side {
	case Symbiont:
		return "Симбионты"
	default:
		return "Люди"
	}
}

func ValidSide(side string) bool {
	return side == Human || side == Symbiont
}
