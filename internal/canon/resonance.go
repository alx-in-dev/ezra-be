package canon

// Symbiont Resonance progression (R4-2 roster, symbiont_roster.md §"Progression
// model"). Two axes: the player's Resonance Level (control cap + unlocks, driven
// by Resonance XP) and per-entity Attunement (later layer).

// ResonanceMaxLevel is the RL cap (RL5).
const ResonanceMaxLevel = 5

// resonanceXPThresholds[L] is the CUMULATIVE Resonance XP needed to reach level
// L. Index 0/1 are 0 (everyone starts at RL1). Per-transition costs from canon:
// RL1→2 100, 2→3 250, 3→4 500, 4→5 900 → cumulative 100 / 350 / 850 / 1750.
var resonanceXPThresholds = [ResonanceMaxLevel + 1]int{0, 0, 100, 350, 850, 1750}

// resonanceControlCap[L] is how many entity control slots RL L grants.
// RL1..RL5 → 2,3,4,5,6.
var resonanceControlCap = [ResonanceMaxLevel + 1]int{0, 2, 3, 4, 5, 6}

// ResonanceLevelForXP returns the Resonance Level (1..ResonanceMaxLevel) for a
// given total Resonance XP.
func ResonanceLevelForXP(xp int) int {
	lvl := 1
	for l := 2; l <= ResonanceMaxLevel; l++ {
		if xp >= resonanceXPThresholds[l] {
			lvl = l
		}
	}
	return lvl
}

// ResonanceControlCap returns the entity control-slot cap for a Resonance Level.
func ResonanceControlCap(level int) int {
	if level < 1 {
		level = 1
	}
	if level > ResonanceMaxLevel {
		level = ResonanceMaxLevel
	}
	return resonanceControlCap[level]
}

// CommanderControlBonusPerPoint: each Commander skill point raises the tamed-
// spirit control cap by this many slots (T-846 — the human skill tree's Commander
// branch, dead until N4, now grows the swarm the Symbiont can field at once).
const CommanderControlBonusPerPoint = 1

// SpiritControlCap is the effective entity control-slot cap: the Resonance-Level
// base plus the Commander-skill bonus (T-846). Commander is what finally makes a
// large tamed roster deployable at once, rather than RL alone.
func SpiritControlCap(level, commanderPoints int) int {
	if commanderPoints < 0 {
		commanderPoints = 0
	}
	return ResonanceControlCap(level) + CommanderControlBonusPerPoint*commanderPoints
}

// ResonanceXPForNextLevel returns the cumulative XP needed for the next level, or
// -1 if already at max (for client "X/Y to RLn" readouts).
func ResonanceXPForNextLevel(level int) int {
	if level >= ResonanceMaxLevel {
		return -1
	}
	if level < 1 {
		level = 1
	}
	return resonanceXPThresholds[level+1]
}

// Resonance Pool conversion weights (symbiont_roster.md §"Resonance Pool"): the
// "резонансный след" a human leaves when siding with the Symbionts.
const (
	PoolPerFighter    = 1
	PoolPerEngineer   = 2
	PoolPerScout      = 1
	PoolPerMedic      = 1
	PoolPerBeaconLow  = 1 // beacon L1-L2
	PoolPerBeaconHigh = 2 // beacon L3+
	PoolPerPetHigh    = 2 // pet/drone L3+

	// PoolStarterMin is the guaranteed floor: even a player who sides with the
	// Symbionts at minimal progress gets enough Pool for 1 starter (assault)
	// entity (symbiont_roster.md "Гарантированное правило").
	PoolStarterMin = 6
)

// PoolPerUnit maps a human unit type to its Resonance Pool contribution.
func PoolPerUnit(unitType string) int {
	switch unitType {
	case "engineer":
		return PoolPerEngineer
	case "scout":
		return PoolPerScout
	case "medic":
		return PoolPerMedic
	case "fighter":
		return PoolPerFighter
	default:
		return PoolPerFighter // unknown line unit counts as a fighter
	}
}

// PoolPerBeacon maps a beacon level to its Resonance Pool contribution.
func PoolPerBeacon(level int) int {
	if level >= 3 {
		return PoolPerBeaconHigh
	}
	return PoolPerBeaconLow
}
