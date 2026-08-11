package squad

import "time"

// Squad represents a group of units assigned to a mission.
type Squad struct {
	ID           string    `json:"id" db:"id"`
	PlayerID     string    `json:"player_id" db:"player_id"`
	Name         string    `json:"name" db:"name"`
	Status       string    `json:"status" db:"status"`
	Mission      *Mission  `json:"mission" db:"mission"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UnitCount    int       `json:"unit_count,omitempty" db:"-"`
	FighterCount int       `json:"fighter_count,omitempty" db:"-"`
}

// Mission describes what a squad is doing.
type Mission struct {
	Type      string    `json:"type"` // "attack_rift", "patrol", "capture"
	TargetID  string    `json:"target_id"`
	StartedAt time.Time `json:"started_at"`
	ReturnsAt time.Time `json:"returns_at"`
}
