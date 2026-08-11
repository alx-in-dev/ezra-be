package canon

import "time"

// Legacy human network degradation (R4-4a, legacy_human_network.md). When a
// player sides with the Symbionts their human beacons don't vanish — they become
// legacy infrastructure that degrades over time unless an active human nearby
// sustains it.

// Legacy states (also the value of NodeStatus.LegacyState; "" = active/human-owned).
const (
	LegacyStable  = "legacy_stable"  // full dome, just no income to the ex-owner
	LegacyFading  = "legacy_fading"  // weakening, visible decay — last chance for support
	LegacyOffline = "legacy_offline" // dead structure: no dome, no suppression
)

// LegacyFadingHours is how long a beacon lingers in the fading window before it
// goes fully offline (canon "additional 0–24 hrs").
const LegacyFadingHours = 24.0

// LegacySupportRadiusM: an active human node within this distance of a legacy
// beacon sustains it (resets the degradation timer → back to stable).
const LegacySupportRadiusM = 200.0

// legacyStableHours returns how long a beacon stays in the stable window before
// fading, scaled by tower level (canon "24–48 hrs (L1–L5)"): L1 24h … L5 48h.
func legacyStableHours(level int) float64 {
	if level < 1 {
		level = 1
	}
	if level > 5 {
		level = 5
	}
	return 24.0 + 6.0*float64(level-1) // 24,30,36,42,48
}

// LegacyState returns the degradation state for a legacy beacon of the given
// level after the elapsed time since it became legacy.
func LegacyState(level int, elapsed time.Duration) string {
	h := elapsed.Hours()
	stable := legacyStableHours(level)
	switch {
	case h < stable:
		return LegacyStable
	case h < stable+LegacyFadingHours:
		return LegacyFading
	default:
		return LegacyOffline
	}
}
