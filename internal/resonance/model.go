package resonance

// CounterWaveRadiusM is how far the counter-wave permanently stabilizes cells
// around the player's position (a neighborhood-scale cleanse).
const CounterWaveRadiusM = 300

// SpectrumStatus is one spectrum's collection state for the client.
type SpectrumStatus struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Owned int    `json:"owned"`
	Have  bool   `json:"have"`
}

// Status is the player's resonance-endgame progress.
type Status struct {
	Spectra      []SpectrumStatus `json:"spectra"`
	Complete     bool             `json:"complete"`     // has ≥1 of every spectrum → can launch
	Collected    int              `json:"collected"`    // distinct spectra owned
	Total        int              `json:"total"`        // total spectra in the set
	WavesLaunched int             `json:"waves_launched"`
}

// ActivateResult is returned after launching a counter-wave.
type ActivateResult struct {
	CellsStabilized int     `json:"cells_stabilized"`
	WaveNumber      int     `json:"wave_number"`
	Lat             float64 `json:"lat"`
	Lng             float64 `json:"lng"`
	RadiusM         int     `json:"radius_m"`
}
