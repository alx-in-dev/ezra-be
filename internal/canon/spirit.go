package canon

import "time"

// Wild-spirit constants (N2, spirit_world_and_symbiont_nest.md §4/§6). All
// numbers are placeholders to be tuned via balance_tables.md + the FIRST
// playtest — N2 is explicitly playtest-gated (consolidated_review №10).

// SpiritMaxClass is the class cap (I..V → 1..5).
const SpiritMaxClass = 5

// SpiritClassConfig is the per-class behaviour: rarer/higher classes move
// differently, threaten more, and are harder to tame.
type SpiritClassConfig struct {
	SpeedMps          float64 // metres/second along its route (slow, telegraphed)
	DangerRadiusM     float64 // touch-debuff radius in the field
	VisibilityRadiusM float64 // must be > DangerRadiusM (§4.3: seen before dangerous)
	WaveDrainPct      float64 // % of a beacon's undelivered income drained on arrival
	TameRLGate        int     // minimum Resonance Level to tame this class
	TameBaseChance    float64 // base tame chance at/above the gate (before pity)
	Reactive          bool    // IV–V evade the hunter (seam); I–III are deterministic
}

var spiritClassConfigs = map[int]SpiritClassConfig{
	1: {SpeedMps: 1.2, DangerRadiusM: 25, VisibilityRadiusM: 120, WaveDrainPct: 0.06, TameRLGate: 1, TameBaseChance: 0.75, Reactive: false},
	2: {SpeedMps: 1.4, DangerRadiusM: 30, VisibilityRadiusM: 140, WaveDrainPct: 0.08, TameRLGate: 1, TameBaseChance: 0.60, Reactive: false},
	3: {SpeedMps: 1.6, DangerRadiusM: 35, VisibilityRadiusM: 160, WaveDrainPct: 0.10, TameRLGate: 2, TameBaseChance: 0.45, Reactive: false},
	4: {SpeedMps: 1.9, DangerRadiusM: 40, VisibilityRadiusM: 180, WaveDrainPct: 0.12, TameRLGate: 4, TameBaseChance: 0.30, Reactive: true},
	5: {SpeedMps: 2.2, DangerRadiusM: 45, VisibilityRadiusM: 200, WaveDrainPct: 0.15, TameRLGate: 5, TameBaseChance: 0.18, Reactive: true},
}

// SpiritConfig returns the class config (clamped to class I for unknown values).
func SpiritConfig(class int) SpiritClassConfig {
	if c, ok := spiritClassConfigs[class]; ok {
		return c
	}
	return spiritClassConfigs[1]
}

const (
	// SpiritRegionBudget: max live wild spirits per region (mirror of the source
	// budget — bounds both server cost and the Scarcity value of taming a rare
	// class). SpiritRegionFloor keeps a region with active players non-empty.
	SpiritRegionBudget = 40
	SpiritRegionFloor  = 3

	// SpiritRouteMaxM: a spirit's route length is capped so it always arrives in a
	// bounded, telegraph-able window (slow speed × this ≈ minutes, not hours).
	SpiritRouteMaxM = 600.0

	// SpiritTouchDebuffMinutes: how long the "Истощение" debuff lasts after a
	// spirit touch in the field (venue-safe: a soft reduction, never a loss).
	SpiritTouchDebuffMinutes = 10

	// SpiritNoviceLevelFloor: spirits are fully inert to players at or below this
	// level (§4.3, decision (д)) — onboarding stays a clean White-Hat phase.
	SpiritNoviceLevelFloor = 3

	// SpiritWaveDrainMaxPct: hard ceiling on how much of a beacon's undelivered
	// income one wave can drain — Executable Loss ≤15% (a spirit never zeroes a
	// beacon; Ultimate Loss from spirits is impossible, §4.2).
	SpiritWaveDrainMaxPct = 0.15

	// SpiritWaveCooldownMinutes: a beacon can't be wave-drained again for this
	// long (impulse, not continuous — venue-safety for an offline owner).
	SpiritWaveCooldownMinutes = 60

	// SpiritBrownoutClass: a wave of at least this class forces the target
	// perimeter beacon into a time-boxed spirit-brownout (the cascade, T-864).
	SpiritBrownoutClass         = 3
	SpiritBrownoutWindowMinutes = 45

	// SpiritExpireAfter: a spirit that has been in flight this long past arrival
	// is discharged/expired by the tick (housekeeping).
	SpiritExpireAfter = 30 * time.Minute

	// RepellentRadiusM: the Ionized Charge repellent clears live spirits within
	// this radius of the player (T-881, "серебряные нити"). Clearing the local area
	// is the player's protection window — no separate immunity flag needed.
	RepellentRadiusM = 150.0

	// RepellentHackKeyCost: hack_key spent per repellent use — the sink that gives
	// the otherwise-dead hack_key a purpose (consolidated_review №9, T-881).
	RepellentHackKeyCost = 1

	// SurgeBudgetMultiplier: while a surge window is active the per-region spirit
	// budget is scaled by this — denser, more intense waves (T-880, replaces the
	// old "Энергетический шторм"). A telegraphed spike bracketed by a start
	// broadcast and a recovery beat (White-Hat bookend), NOT the baseline. Surges
	// are OFF by default — nothing schedules one until playtest tuning.
	SurgeBudgetMultiplier = 2.0

	// GeyserSeedChance: per-tick probability the spirit tick plants one new geyser
	// (the rare class-V source, T-872). Low so geysers stay a special POI, not
	// scenery — with a ~5-min tick this is roughly one new geyser per few hours of
	// active play near hives.
	GeyserSeedChance = 0.05

	// GeyserMinGapM: minimum spacing between geysers — keeps class-V sources sparse
	// so class V never floods a region (Scarcity + venue-safety).
	GeyserMinGapM = 1500.0

	// SpiritWeakenPerAction: how much a single weaken action adds to weakened_pct
	// (100 = fully softened).
	SpiritWeakenPerAction = 34.0

	// SpiritTameMinWeakened: a spirit must be softened to at least this before a
	// tame attempt is allowed (the "выследить → ослабить → приручить" loop, §6.1).
	SpiritTameMinWeakened = 34.0

	// SpiritWeakenTameBonus: weakening doubles as pity — the softer the spirit,
	// the better the tame chance (folds "ослабление" and "pity" into one, so no
	// separate per-class pity store is needed). At 100% weakened this adds the
	// full bonus on top of the class base chance.
	SpiritWeakenTameBonus = 0.5
)

// SpiritTameChance returns the tame chance for a class given the player's RL and
// how softened the spirit is (weakening doubles as pity). Returns 0 if RL is
// below the class gate.
func SpiritTameChance(class, resonanceLevel int, weakenedPct float64) float64 {
	cfg := SpiritConfig(class)
	if resonanceLevel < cfg.TameRLGate {
		return 0
	}
	if weakenedPct < 0 {
		weakenedPct = 0
	}
	if weakenedPct > 100 {
		weakenedPct = 100
	}
	chance := cfg.TameBaseChance + SpiritWeakenTameBonus*(weakenedPct/100.0)
	if chance > 0.95 {
		chance = 0.95
	}
	return chance
}

// SpiritWaveDrainEnergy is the ⚡ a class-N wave steals from the target beacon's
// owner on arrival (proxy for "недособранный доход" until a per-beacon income
// buffer exists — see the N2 flag). Scales with class; a placeholder кап keeps
// it a small, recoverable Executable Loss, never a wipe.
func SpiritWaveDrainEnergy(class int) int {
	if class < 1 {
		class = 1
	}
	if class > SpiritMaxClass {
		class = SpiritMaxClass
	}
	return class * 8 // I=8 … V=40 ⚡ per wave (placeholder)
}

// SpiritArchetype maps a spirit class to the roster archetype it becomes when
// tamed (spirit_world_and_symbiont_nest.md §6.2 / symbiont_roster.md).
func SpiritArchetype(class int) string {
	switch class {
	case 1:
		return EntityAssault
	case 2:
		return EntityScout
	case 3:
		return EntityDistortion
	case 4:
		return EntityAbsorber
	default:
		return EntityAssault // class V → a strong assault entity
	}
}
