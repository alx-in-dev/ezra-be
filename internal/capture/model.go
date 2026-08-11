package capture

// LockpickRequest is the request body for lockpick capture.
type LockpickRequest struct {
	TowerID string `json:"tower_id"`
}

// ForceCaptureRequest is the request body for force capture (starts a battle).
type ForceCaptureRequest struct {
	TowerID string `json:"tower_id"`
	SquadID string `json:"squad_id"`
}
