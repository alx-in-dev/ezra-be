package battle

import "github.com/ezra-game/server/internal/unit"

// BattleSide is the client-facing HP snapshot for one side of combat.
type BattleSide struct {
	HP           int    `json:"hp"`
	HPMax        int    `json:"hp_max"`
	UnitCount    int    `json:"unit_count,omitempty"`
	RiftType     string `json:"rift_type,omitempty"`
	SpiritsCount int    `json:"spirits_count,omitempty"`

	// Enemy composition + counter hint (M1 combat depth): surface the C3
	// weakness math so the player's tactic choice each round is an informed
	// decision instead of a blind one. SwarmShare/HumanoidShare are fractions
	// of enemy HP; Weakness names the super-effective tactic ("energy" vs a
	// swarm, "defend" vs humanoids) or "" when no tactic clearly dominates.
	SwarmShare    float64 `json:"swarm_share,omitempty"`
	HumanoidShare float64 `json:"humanoid_share,omitempty"`
	Weakness      string  `json:"weakness,omitempty"`
}

// Snapshot is the normalized battle payload used by the Unity client.
type Snapshot struct {
	BattleID    string         `json:"battle_id"`
	Status      string         `json:"status"`
	BattleState string         `json:"battle_state"`
	Round       int            `json:"round"`
	TargetType  string         `json:"target_type"`
	TargetID    string         `json:"target_id"`
	EnemyHP     int            `json:"enemy_hp"`
	SquadHP     int            `json:"squad_hp"`
	Enemy       BattleSide     `json:"enemy"`
	Squad       BattleSide     `json:"squad"`
	LastRound   *Round         `json:"last_round,omitempty"`
	Loot         map[string]any     `json:"loot,omitempty"`
	Profile      map[string]any     `json:"profile,omitempty"`
	Onboarding   map[string]any     `json:"onboarding,omitempty"`
	UnitLevelUps []unit.UnitLevelUp `json:"unit_levelups,omitempty"`
}
