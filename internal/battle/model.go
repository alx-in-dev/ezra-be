package battle

import (
	"time"

	"github.com/ezra-game/server/internal/unit"
)

// Battle represents a tactical combat encounter.
type Battle struct {
	ID         string    `json:"id" db:"id"`
	AttackerID string    `json:"attacker_id" db:"attacker_id"`
	SquadID    string    `json:"squad_id" db:"squad_id"`
	TargetType string    `json:"target_type" db:"target_type"`
	TargetID   string    `json:"target_id" db:"target_id"`
	Rounds     []Round   `json:"rounds"`
	Status     string    `json:"status" db:"status"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time `json:"updated_at" db:"updated_at"`

	// UnitLevelUps carries the units that leveled up on this resolution (D3).
	// Transient — populated in onBattleResolved, surfaced via Snapshot, never
	// persisted.
	UnitLevelUps []unit.UnitLevelUp `json:"-" db:"-"`

	// EpicLoot carries the epic drop (D5 resonance signature) for this
	// resolution. Transient — Snapshot rebuilds the loot map, so the grant is
	// stashed here and merged back in. Never persisted.
	EpicLoot map[string]any `json:"-" db:"-"`
}

// Round captures one round of combat results.
type Round struct {
	Number    int    `json:"number"`
	Tactic    string `json:"tactic"`
	DmgDealt  int    `json:"dmg_dealt"`
	DmgTaken  int    `json:"dmg_taken"`
	UnitsLost int    `json:"units_lost"`
	EnemyHP   int    `json:"enemy_hp"`
	SquadHP   int    `json:"squad_hp"`
}

// Spirit represents an enemy combatant spawned by a rift.
type Spirit struct {
	Type string `json:"type"`
	HP   int    `json:"hp"`
	ATK  int    `json:"atk"`
}

// SpiritConfigs defines base stats per spirit type.
// "shard" is the critical-rift (III) boss — the «осколок» from
// docs/03_world_system.md: a single high-HP elite anchoring a swarm.
var SpiritConfigs = map[string]Spirit{
	"small":    {Type: "small", HP: 30, ATK: 5},
	"humanoid": {Type: "humanoid", HP: 80, ATK: 15},
	"shard":    {Type: "shard", HP: 260, ATK: 42},
}
