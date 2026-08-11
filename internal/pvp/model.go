package pvp

// Dome Breach tuning (v1.1 PvP slice).
const (
	BreachEnergyCost   = 200      // ⚡ to puncture an enemy dome
	BreachRiftType     = "medium" // the breach opens a medium rift under the shield
	TargetRadiusM      = 2000     // how far a Symbiont can scout enemy domes
	BreachInfection    = 80       // infection forced on the breached cell so it sticks
	ResonancePerBreach = 10       // portable Symbiont Resonance earned per dome breach (#6)
)

// Target is an enemy domed cell a Symbiont can breach.
type Target struct {
	CellID       string  `json:"cell_id"`
	Lat          float64 `json:"lat"`
	Lng          float64 `json:"lng"`
	DefenderID   string  `json:"defender_id"`
	DefenderName string  `json:"defender_name"`
}

// BreachResult is returned after a successful Dome Breach.
type BreachResult struct {
	CellID       string `json:"cell_id"`
	RiftID       string `json:"rift_id"`
	DefenderName string `json:"defender_name"`
}
