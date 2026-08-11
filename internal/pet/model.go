package pet

import (
	"math"
	"time"

	"github.com/ezra-game/server/internal/survivor"
)

// Pet represents a player's companion creature.
type Pet struct {
	ID       string   `json:"id" db:"id"`
	PlayerID string   `json:"player_id" db:"player_id"`
	Name     string   `json:"name" db:"name"`
	Type     string   `json:"type" db:"type"`
	Level    int      `json:"level" db:"level"`
	XP       int      `json:"xp" db:"xp"`
	Status   string   `json:"status" db:"status"`
	Task     *PetTask `json:"task"` // JSONB

	// Computed (db:"-"), populated via Decorate for client display.
	Stage     int    `json:"stage" db:"-"`
	StageName string `json:"stage_name" db:"-"`
	Evolved   bool   `json:"evolved" db:"-"`
	XPMax     int    `json:"xp_max" db:"-"`
}

// MaxLevel caps pet progression (search/scout tables top out at 5).
const MaxLevel = 5

// EvolutionStage maps a pet's level to its evolution stage and display name
// (D3). The pet visibly grows up as it levels: птенец → молодой → зрелый.
func EvolutionStage(level int) (stage int, name string) {
	switch {
	case level >= 5:
		return 3, "Зрелый дух"
	case level >= 3:
		return 2, "Молодой дух"
	default:
		return 1, "Дух-птенец"
	}
}

// Decorate fills the computed evolution/XP fields for client display.
func (p *Pet) Decorate() {
	p.Stage, p.StageName = EvolutionStage(p.Level)
	p.Evolved = p.Stage >= 3
	if p.Level < MaxLevel {
		p.XPMax = XPForLevel(p.Level + 1)
	} else {
		p.XPMax = 0
	}
}

// PetTask describes an active task a pet is performing.
type PetTask struct {
	Type          string    `json:"type"`
	StartedAt     time.Time `json:"started_at"`
	ReturnsAt     time.Time `json:"returns_at"`
	LootRoll      float64   `json:"loot_roll"`
	TargetTowerID string    `json:"target_tower_id,omitempty"`
}

// SearchDuration returns search duration in minutes by pet level.
var SearchDuration = map[int]int{1: 45, 2: 35, 3: 25, 4: 18, 5: 12}

// RareChance returns rare loot probability by pet level.
var RareChance = map[int]float64{1: 0.05, 2: 0.10, 3: 0.18, 4: 0.28, 5: 0.40}

const (
	TaskTypeSearch = "search"
	TaskTypeScout  = "scout"
)

const ScoutCooldownWindow = 2 * time.Hour

type ScoutResult struct {
	TowerID       string              `json:"tower_id,omitempty"`
	Status        string              `json:"status,omitempty"`
	CooldownUntil time.Time           `json:"cooldown_until,omitempty"`
	Survivors     []survivor.Survivor `json:"survivors,omitempty"`
}

// XPForLevel returns the XP required to reach the given level.
// Formula: 30 * n^1.5
func XPForLevel(level int) int {
	return int(30 * math.Pow(float64(level), 1.5))
}
