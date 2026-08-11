package spire

import (
	"fmt"
	"math"
	"time"
)

const (
	// FragmentsToOpen blueprint fragments must be pooled before the build opens.
	FragmentsToOpen = 3
	// ResourceChunk is the energy spent per contribution call. The server
	// computes the delta — the client never sends a contribution amount.
	ResourceChunk = 250
	// DefaultRadiusM is the Spire's effect/zone radius (10 km).
	DefaultRadiusM = 10000
	// SuppressionPerHour is the extra infection decay an active Spire grants to
	// every cell in its radius (the "+10% suppression" civic bonus).
	SuppressionPerHour = 1.5
	// regionGridDeg buckets the world into ~11 km cells so one Spire serves a
	// region (dedup key); the anchor is the bucket centre.
	regionGridDeg = 0.1

	// --- Escrow build window (megastructure_spire.md §3, owner decision А) ---
	// BuildWindow is how long a building has to reach its target before the
	// escrowed contributions are refunded and the Spire resets to assembling.
	BuildWindow = 14 * 24 * time.Hour

	// --- Requirement from population (P_zone, owner decision В) ---
	// PerCapitaTarget is the build resource required per active player in the
	// zone; the target scales with population so build time is ~constant region
	// to region. MinBuildTarget is the floor so a tiny region still has a real
	// (but achievable) goal.
	PerCapitaTarget = 300
	MinBuildTarget  = 1500
	// ActiveWindow scopes P_zone to recently-active residents.
	ActiveWindowDays = 28

	// Lifecycle upkeep windows: an active Spire with no upkeep for DegradeAfter
	// halves its bonus (degrading); after RuinsAfter it collapses to ruins.
	DegradeAfter = 7 * 24 * time.Hour
	RuinsAfter   = 14 * 24 * time.Hour
	// DegradingSuppressionFactor halves the bonus while degrading.
	DegradingSuppressionFactor = 0.5

	// Contribution tiers (recognition, granted on activation).
	TierGold   = "gold"
	TierSilver = "silver"
	TierBronze = "bronze"
)

// TargetForPopulation returns the build resource requirement for a region of the
// given active population (owner decision: requirement scales with P_zone).
func TargetForPopulation(pZone int) int {
	t := pZone * PerCapitaTarget
	if t < MinBuildTarget {
		return MinBuildTarget
	}
	return t
}

// TierFor classifies a player's resource contribution relative to the fair share
// (= target / population). Returns "" if the player contributed nothing.
func TierFor(resources, target, pZone int) string {
	if resources <= 0 {
		return ""
	}
	denom := pZone
	if denom < 1 {
		denom = 1
	}
	fair := target / denom
	if fair < 1 {
		fair = 1
	}
	switch {
	case resources >= 2*fair:
		return TierGold
	case resources >= fair:
		return TierSilver
	default:
		return TierBronze
	}
}

type Spire struct {
	ID            string     `json:"id" db:"id"`
	RegionKey     string     `json:"region_key" db:"region_key"`
	Lat           float64    `json:"lat" db:"lat"`
	Lng           float64    `json:"lng" db:"lng"`
	State          string     `json:"state" db:"state"`
	Fragments      int        `json:"fragments" db:"fragments"`
	BuildProgress  int        `json:"build_progress" db:"build_progress"`
	BuildTarget    int        `json:"build_target" db:"build_target"`
	PZone          int        `json:"p_zone" db:"p_zone"`
	RadiusM        int        `json:"radius_m" db:"radius_m"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	ActivatedAt    *time.Time `json:"activated_at" db:"activated_at"`
	BuildDeadline  *time.Time `json:"build_deadline" db:"build_deadline"`
	LastActivityAt time.Time  `json:"last_activity_at" db:"last_activity_at"`
}

// Status is the client-facing Spire view for a region.
type Status struct {
	Spire           *Spire `json:"spire"`
	FragmentsToOpen int    `json:"fragments_to_open"`
	YourFragments   int    `json:"your_fragments"`   // blueprint fragments the player holds (inventory)
	YourContributed int    `json:"your_contributed"` // resources this player put in
	YourTier        string `json:"your_tier"`        // contribution tier once activated (bronze/silver/gold)
	ResourceChunk   int    `json:"resource_chunk"`
}

// RegionKeyFor returns the dedup key for the region containing a point.
func RegionKeyFor(lat, lng float64) string {
	return fmt.Sprintf("%.1f_%.1f", math.Floor(lat/regionGridDeg)*regionGridDeg, math.Floor(lng/regionGridDeg)*regionGridDeg)
}

// RegionCenter returns the anchor point (bucket centre) for a region.
func RegionCenter(lat, lng float64) (float64, float64) {
	return math.Floor(lat/regionGridDeg)*regionGridDeg + regionGridDeg/2,
		math.Floor(lng/regionGridDeg)*regionGridDeg + regionGridDeg/2
}
