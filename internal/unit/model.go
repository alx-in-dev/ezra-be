package unit

import "math"

// Unit represents a single army unit belonging to a player.
type Unit struct {
	ID       string  `json:"id" db:"id"`
	PlayerID string  `json:"player_id" db:"player_id"`
	Type     string  `json:"type" db:"type"`
	Level    int     `json:"level" db:"level"`
	XP       int     `json:"xp" db:"xp"`
	HP       int     `json:"hp" db:"hp"`
	ATK      int     `json:"atk" db:"atk"`
	SquadID  *string `json:"squad_id" db:"squad_id"`
	Status   string  `json:"status" db:"status"`

	// XPMax is the XP needed to reach the next level (0 when maxed). Computed,
	// not persisted — populated for client display.
	XPMax int `json:"xp_max" db:"-"`
}

// BaseStats defines starting HP and ATK by unit type.
var BaseStats = map[string]struct{ HP, ATK int }{
	"fighter":  {HP: 50, ATK: 10},
	"engineer": {HP: 40, ATK: 5},
	"scout":    {HP: 35, ATK: 7},
	"medic":    {HP: 30, ATK: 3},
}

// MaxLevel caps unit progression (D3 / 08-scope). Pets cap at 5; line units
// go further so a veteran squad stays meaningfully stronger than fresh recruits.
const MaxLevel = 10

// LevelStatGrowth is the per-level fraction of base HP/ATK a unit gains (linear).
// Referenced both here (StatsForLevel) and in repository.HealAll so a healed
// unit is restored to its level-appropriate max, not its level-1 base.
const LevelStatGrowth = 0.15

// XPForLevel returns the XP required to advance to the given level from the
// previous one. Formula: round(20 * n^1.4) (§4.2 / 08-scope). Mirrors the pet
// curve convention in internal/pet (cost-to-reach, subtracted on level-up).
func XPForLevel(level int) int {
	if level <= 1 {
		return 0
	}
	return int(math.Round(20 * math.Pow(float64(level), 1.4)))
}

// StatsForLevel returns the HP and ATK a unit of the given type has at a level.
// Each level adds LevelStatGrowth of the type's base stats (linear growth).
func StatsForLevel(unitType string, level int) (hp, atk int) {
	base, ok := BaseStats[unitType]
	if !ok {
		base = BaseStats["fighter"]
	}
	growth := 1.0 + LevelStatGrowth*float64(level-1)
	hp = int(math.Round(float64(base.HP) * growth))
	atk = int(math.Round(float64(base.ATK) * growth))
	return
}

// GainXP applies battle XP to the unit, leveling up (and rescaling HP/ATK to
// the new max) while thresholds are met. Returns the number of levels gained.
func (u *Unit) GainXP(amount int) int {
	if amount <= 0 || u.Level >= MaxLevel {
		return 0
	}
	u.XP += amount
	gained := 0
	for u.Level < MaxLevel {
		need := XPForLevel(u.Level + 1)
		if u.XP < need {
			break
		}
		u.XP -= need
		u.Level++
		gained++
	}
	if gained > 0 {
		u.HP, u.ATK = StatsForLevel(u.Type, u.Level)
	}
	if u.Level >= MaxLevel {
		u.XP = 0
	}
	return gained
}
