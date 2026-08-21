package factionwar

import (
	"fmt"
	"time"
)

// Faction-war point awards, from docs/feature/faction_war.md.
const (
	PointsCloseRiftMinor    = 25
	PointsCloseRiftMedium   = 60
	PointsCloseRiftCritical = 140
	PointsCollapseHive      = 140 // collapsing an infection root is a III-class feat
	// Symbiont-side feats (canon asymmetric scoring).
	PointsOverloadBeacon = 20
	PointsDomeBreach     = 45
)

// PointsForRiftClose maps a rift tier to its Human score.
func PointsForRiftClose(tier string) int {
	switch tier {
	case "minor":
		return PointsCloseRiftMinor
	case "medium":
		return PointsCloseRiftMedium
	case "critical":
		return PointsCloseRiftCritical
	default:
		return 0
	}
}

// SeasonFor returns the weekly season key for a time (ISO year-week), e.g.
// "2026-W24". A 24h/weekly regional event is keyed off this.
func SeasonFor(t time.Time) string {
	y, w := t.ISOWeek()
	return fmt.Sprintf("%d-W%02d", y, w)
}

// RegionBalance is the live Human-vs-Symbiont control of a region.
//
// N1 scoring (T-822): control is measured by PROJECTED TERRITORY, not ambient
// infection. Human side = domed cells (its "aura") minus Symbiont-pierced holes;
// Symbiont side = source-aura cells (hives ∪ open rifts) plus pierced cells,
// deduped. Neutral cells (neither) count for nobody, so localizing the world
// (zeroing ambient infection) doesn't instantly recolor a Symbiont region human.
// The ambient counts below (clean/infected) are kept only as diagnostics.
type RegionBalance struct {
	HumanRaw      int `json:"human_raw"`
	SymbiontRaw   int `json:"symbiont_raw"`
	HumanPct      int `json:"human_pct"`
	SymbiontPct   int `json:"symbiont_pct"`
	DomedCells    int `json:"domed_cells"`
	CleanCells    int `json:"clean_cells"`    // diagnostic (ambient), not scored in N1
	InfectedCells int `json:"infected_cells"` // diagnostic (ambient), not scored in N1
	OpenHives     int `json:"open_hives"`
	OpenRifts     int `json:"open_rifts"`
	// N1 territory measures.
	AuraCells    int `json:"aura_cells"`    // cells inside a source aura (hive/rift)
	PiercedCells int `json:"pierced_cells"` // cells currently pierced by a Symbiont
}

// Standing is one player's place on the season leaderboard.
type Standing struct {
	PlayerID string `json:"player_id"`
	Username string `json:"username"`
	Points   int    `json:"points"`
}

// SeasonReward is a player's payout for a settled season.
type SeasonReward struct {
	Season   string `json:"season"`
	Rank     int    `json:"rank"`
	Points   int    `json:"points"`
	Crystals int    `json:"crystals"`
}

// Status is the full faction-war view for the client.
type Status struct {
	Season           string        `json:"season"`
	Balance          RegionBalance `json:"balance"`
	YourContribution int           `json:"your_contribution"`
	Leaderboard      []Standing    `json:"leaderboard"`
	LastReward       *SeasonReward `json:"last_reward,omitempty"`
}

// CrystalsForRank is the season payout by leaderboard rank (participation floor
// for anyone who scored).
func CrystalsForRank(rank int) int {
	switch rank {
	case 1:
		return 500
	case 2:
		return 300
	case 3:
		return 150
	default:
		return 50
	}
}
