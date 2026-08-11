package network

import "time"

// Core is a player's personal network root (Perimeter "Frame"): the source of
// energy and the anchor every beacon link ultimately traces back to.
type Core struct {
	ID          string     `json:"id" db:"id"`
	PlayerID    string     `json:"player_id" db:"player_id"`
	CellID      string     `json:"cell_id" db:"cell_id"`
	Lat         float64    `json:"lat" db:"lat"`
	Lng         float64    `json:"lng" db:"lng"`
	Level       int        `json:"level" db:"level"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	RelocatedAt *time.Time `json:"relocated_at,omitempty" db:"relocated_at"`
}

// Link is an auto-formed edge between two nodes (core or towers).
type Link struct {
	ID       string  `json:"id" db:"id"`
	PlayerID string  `json:"player_id" db:"player_id"`
	FromID   string  `json:"from_id" db:"from_id"`
	ToID     string  `json:"to_id" db:"to_id"`
	LengthM  float64 `json:"length_m" db:"length_m"`
	Cost     int     `json:"cost" db:"cost"`
}

// Node is a graph vertex (the core or a player's tower) with its position.
type Node struct {
	ID     string
	Lat    float64
	Lng    float64
	IsCore bool
	Level  int // drives ConnectionRadius (transmission reach)
	// LegacySince marks a beacon as legacy infrastructure (owner went Symbiont);
	// nil for active/human-owned nodes and all cores (R4-4a).
	LegacySince *time.Time
}

// NodeStatus reports a node's computed power state and position for the client
// (so it can draw the network without a second positions lookup).
type NodeStatus struct {
	ID       string  `json:"id"`
	Lat      float64 `json:"lat"`
	Lng      float64 `json:"lng"`
	IsCore   bool    `json:"is_core"`
	Powered  bool    `json:"powered"`
	Brownout bool    `json:"brownout"`
	RadiusM  float64 `json:"radius_m"` // energy-disk radius (client draws the field as overlapping disks)
	// LegacyState is "" for active nodes, or legacy_stable/legacy_fading for a
	// degrading legacy beacon (offline ones are dropped from the network). R4-4a.
	LegacyState string `json:"legacy_state,omitempty"`
}

// LatLng is a geographic vertex (used for the dome polygon ring).
type LatLng struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// FieldDisk is one powered node's energy disk with ownership stripped, used for
// the merged regional field overlay: neighbouring/allied domes render as one
// continuous shield (the visual half of Meta-Dome — suppression already merges
// server-side, ownership-agnostically). Returned by GET /map/fields.
type FieldDisk struct {
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
	RadiusM float64 `json:"radius_m"`
}

// State is the computed network snapshot returned by GET /network.
type State struct {
	Core           *Core        `json:"core"`
	Links          []Link       `json:"links"`
	Nodes          []NodeStatus `json:"nodes"`
	EcapUsed       int          `json:"ecap_used"`
	EcapMax        int          `json:"ecap_max"`
	DomeSealed     bool         `json:"dome_sealed"`
	DomedCellCount int          `json:"domed_cell_count"`
	DomePolygon    []LatLng     `json:"dome_polygon,omitempty"`
	// Crystals the NEXT Core relocation will cost (0 = the first one is still
	// free). Lets the client show the right confirm before charging.
	NextRelocationCost int `json:"next_relocation_cost"`
}
