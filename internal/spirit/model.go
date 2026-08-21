// Package spirit implements N2 wild spirits: the wandering энергетические
// существа that threaten human beacons (wave-drain), embody the lore on the
// map, and become the Symbiont's army when tamed. Modelled as "рейсы" (N0
// verdict): a spirit's live position is interpolated from its route + timing, so
// movement costs zero writes; the tick only handles arrivals/expiry.
package spirit

import (
	"time"

	"github.com/ezra-game/server/internal/canon"
)

// States (mirror the CHECK on wild_spirits.state).
const (
	StateWandering = "wandering"
	StateWeakened  = "weakened"
	StateTamed     = "tamed"
	StateExpired   = "expired"
)

// Spirit is one wild spirit in flight from a source toward its destination.
type Spirit struct {
	ID    string `json:"id" db:"id"`
	Class int    `json:"class" db:"class"`

	OriginLat float64 `json:"origin_lat" db:"origin_lat"`
	OriginLng float64 `json:"origin_lng" db:"origin_lng"`
	DestLat   float64 `json:"dest_lat" db:"dest_lat"`
	DestLng   float64 `json:"dest_lng" db:"dest_lng"`

	SpawnTs  time.Time `json:"spawn_ts" db:"spawn_ts"`
	SpeedMps float64   `json:"speed_mps" db:"speed_mps"`
	DistM    float64   `json:"dist_m" db:"dist_m"`
	ArriveAt time.Time `json:"arrive_at" db:"arrive_at"`

	State         string  `json:"state" db:"state"`
	WeakenedPct   float64 `json:"weakened_pct" db:"weakened_pct"`
	TamedBy       *string `json:"tamed_by,omitempty" db:"tamed_by"`
	TargetTowerID *string `json:"target_tower_id,omitempty" db:"target_tower_id"`
	RegionKey     *string `json:"region_key,omitempty" db:"region_key"`

	CreatedAt time.Time `json:"created_at" db:"created_at"`

	// Computed for the client (not persisted).
	Lat            float64 `json:"lat" db:"-"`
	Lng            float64 `json:"lng" db:"-"`
	DangerRadiusM  float64 `json:"danger_radius_m" db:"-"`
	VisibleRadiusM float64 `json:"visible_radius_m" db:"-"`
}

// Config returns the class config.
func (s *Spirit) Config() canon.SpiritClassConfig { return canon.SpiritConfig(s.Class) }

// progress returns the [0,1] fraction of the route travelled at time `at`.
func (s *Spirit) progress(at time.Time) float64 {
	if s.DistM <= 0 || s.SpeedMps <= 0 {
		return 1
	}
	elapsed := at.Sub(s.SpawnTs).Seconds()
	f := (elapsed * s.SpeedMps) / s.DistM
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}

// decorate fills the interpolated live position + client radii at time `at`.
func (s *Spirit) decorate(at time.Time) {
	f := s.progress(at)
	s.Lat = s.OriginLat + (s.DestLat-s.OriginLat)*f
	s.Lng = s.OriginLng + (s.DestLng-s.OriginLng)*f
	cfg := s.Config()
	s.DangerRadiusM = cfg.DangerRadiusM
	s.VisibleRadiusM = cfg.VisibilityRadiusM
}
