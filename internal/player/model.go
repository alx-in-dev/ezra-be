package player

import "time"

type Player struct {
	ID                     string      `json:"id" db:"id"`
	FirebaseUID            string      `json:"-" db:"firebase_uid"`
	Login                  string      `json:"-" db:"login"`
	PasswordHash           string      `json:"-" db:"password_hash"`
	Username               string      `json:"username" db:"username"`
	UsernameIsCustom       bool        `json:"username_is_custom" db:"username_is_custom"`
	OnboardingStep         string      `json:"onboarding_step" db:"onboarding_step"`
	StarterBeaconAvailable bool        `json:"starter_beacon_available" db:"starter_beacon_available"`
	// QuickStartHuman marks a player who skipped the onboarding narrative
	// (docs/feature/onboarding_quick_start.md): they still place their first
	// beacon manually, but tower.Service grants the rest of the human chain
	// (2nd survivor, tutorial-battle rewards, starter pet) instantly once they do.
	QuickStartHuman bool `json:"-" db:"quick_start_human"`
	Level                  int         `json:"level" db:"level"`
	XP                     int         `json:"xp" db:"xp"`
	Energy                 int         `json:"energy" db:"energy"`
	Materials              int         `json:"materials" db:"materials"`
	ArmyLimit              int         `json:"army_limit" db:"army_limit"`
	Crystals               int         `json:"crystals" db:"crystals"`
	StorageBonusEnergy     int         `json:"storage_bonus_energy" db:"storage_bonus_energy"`
	PetSlots               int         `json:"pet_slots" db:"pet_slots"`
	SymbiontResonance      int         `json:"symbiont_resonance" db:"symbiont_resonance"`
	Skills                 SkillPoints `json:"skills" db:"skills"`
	Position               Position    `json:"position" db:"position"`
	LastActive             time.Time   `json:"last_active" db:"last_active"`
	CreatedAt              time.Time   `json:"created_at" db:"created_at"`
}

type SkillPoints struct {
	Defender  int `json:"defender"`
	Commander int `json:"commander"`
	Energist  int `json:"energist"`
}

type Position struct {
	Lat       float64   `json:"lat"`
	Lng       float64   `json:"lng"`
	UpdatedAt time.Time `json:"updated_at"`
}

// EnergyMax is the player's effective energy storage cap: the canon base, a
// free level-scaled allowance, and shop expansion bonuses. The level allowance
// (anti-P2W: capacity grows with level, not money) guarantees a max-level F2P
// can bank the most expensive upgrade — at level 15 the cap is 3400, above the
// L5 beacon's 3200⚡ cost (C6). See docs/feature/balance_tables.md.
func (p *Player) EnergyMax() int {
	lvl := p.Level
	if lvl < 1 {
		lvl = 1
	}
	return MaxEnergy + (lvl-1)*EnergyCapPerLevel + p.StorageBonusEnergy
}

// MaterialsMax mirrors EnergyMax for 🔩: the flat 500 cap made the whole L5
// tier unreachable (beacon L5 costs 640🔩, core L5 600🔩). At level 15 the cap
// is 710 — above both, same headroom philosophy as the energy cap (C6).
func (p *Player) MaterialsMax() int {
	lvl := p.Level
	if lvl < 1 {
		lvl = 1
	}
	return MaxMaterials + (lvl-1)*MaterialsCapPerLevel
}
