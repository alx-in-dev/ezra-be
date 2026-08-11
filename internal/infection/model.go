package infection

// RecalcResult holds the outcome of a single cell infection recalculation.
type RecalcResult struct {
	CellID       string  `json:"cell_id"`
	OldInfection float64 `json:"old_infection"`
	NewInfection float64 `json:"new_infection"`
	Growth       float64 `json:"growth"`
	RiftEffect   float64 `json:"rift_effect"`
	TowerEffect  float64 `json:"tower_effect"`
}
