package survivor

// Survivor is a recruitable NPC spawned near the player.
type Survivor struct {
	ID   string  `json:"id"`
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
	Type string  `json:"type"`
}

// TypeWeights defines spawn probability weights for each survivor type.
//
// MVP only spawns the two recruitable types (fighter/engineer); survivor.Recruit
// rejects anything else. scout/medic are post-MVP unit types — re-add them here
// once Recruit accepts them, otherwise they spawn on the map as un-recruitable
// dead-ends.
var TypeWeights = []struct {
	Type   string
	Weight int
}{
	{"fighter", 60},
	{"engineer", 40},
}
