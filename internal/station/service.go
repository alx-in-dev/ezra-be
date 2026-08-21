package station

import (
	"context"
	"fmt"
	"math"

	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/pkg/geo"
	"github.com/ezra-game/server/pkg/httputil"
)

// ResourceSpender charges the build cost.
type ResourceSpender interface {
	Spend(ctx context.Context, playerID string, energy, materials int) (*player.Player, error)
}

// FactionChecker reports a player's faction for the human-toolkit exclusion
// gate (T-800: Symbionts don't build stationary infrastructure).
type FactionChecker interface {
	IsSymbiont(ctx context.Context, playerID string) (bool, error)
}

// Service implements power-plant build logic.
type Service struct {
	repo      Repository
	resources ResourceSpender
	faction   FactionChecker
}

func NewService(repo Repository, resources ResourceSpender) *Service {
	return &Service{repo: repo, resources: resources}
}

// WithFaction wires the optional faction gate. Kept off the constructor so
// existing callers/tests stay unchanged.
func (s *Service) WithFaction(f FactionChecker) *Service {
	s.faction = f
	return s
}

// Build places a power plant at (lat,lng): spacing + per-radius cap checks, then
// charges the cost. The capacity boost is bounded by MaxInRadius and the global
// HARD_CAP, so a sparse area improves without overtaking a dense centre.
func (s *Service) Build(ctx context.Context, playerID string, lat, lng float64) (*PowerPlant, error) {
	if s.faction != nil {
		if isSymbiont, err := s.faction.IsSymbiont(ctx, playerID); err == nil && isSymbiont {
			return nil, httputil.NewForbidden("symbiont_no_human_toolkit", "симбионт не может строить станции")
		}
	}
	if lat == 0 && lng == 0 {
		return nil, httputil.NewBadRequest("no_position", "нет позиции для станции")
	}

	// Spacing: not too close to an existing plant.
	if dist, ok, err := s.repo.NearestDistanceM(ctx, lat, lng); err != nil {
		return nil, err
	} else if ok && dist < MinSpacingM {
		return nil, httputil.NewBadRequest("too_close", "слишком близко к другой станции")
	}

	// Per-area cap: at most MaxInRadius plants in the capacity zone.
	if n, err := s.repo.CountInRadius(ctx, lat, lng, BuildRadiusM); err != nil {
		return nil, err
	} else if n >= MaxInRadius {
		return nil, httputil.NewBadRequest("area_full",
			fmt.Sprintf("в этой зоне уже максимум станций (%d)", MaxInRadius))
	}

	if s.resources != nil {
		if _, err := s.resources.Spend(ctx, playerID, CostEnergy, CostMaterials); err != nil {
			return nil, err
		}
	}

	p := &PowerPlant{OwnerID: playerID, Lat: lat, Lng: lng, CapacityBonus: CapacityBonus}
	if err := s.repo.Create(ctx, p); err != nil {
		return nil, fmt.Errorf("create power plant: %w", err)
	}
	return p, nil
}

// Nearby lists plants around a point (for the map).
func (s *Service) Nearby(ctx context.Context, lat, lng, radiusM float64) ([]PowerPlant, error) {
	return s.repo.ListInRadius(ctx, lat, lng, radiusM)
}

// Upkeep services the nearest plant the player owns (charges materials, resets
// it to active). Errors with no_station when none is in range (T-778).
func (s *Service) Upkeep(ctx context.Context, playerID string, lat, lng float64) error {
	if lat == 0 && lng == 0 {
		return httputil.NewBadRequest("no_position", "нет позиции")
	}
	// Reset the station first so we never charge when there's nothing in range.
	ok, err := s.repo.Upkeep(ctx, playerID, lat, lng, BuildRadiusM)
	if err != nil {
		return err
	}
	if !ok {
		return httputil.NewBadRequest("no_station", "рядом нет вашей станции")
	}
	if s.resources != nil {
		if _, err := s.resources.Spend(ctx, playerID, 0, UpkeepCostMaterials); err != nil {
			return err // station already reset; charge failed → free this once (benign)
		}
	}
	return nil
}

// Tick advances the lifecycle (worker entrypoint).
func (s *Service) Tick(ctx context.Context) error {
	return s.repo.AdvanceLifecycle(ctx, ActiveDays, DegradeDays)
}

// SabotageResult reports the outcome of a Symbiont Overload strike against a
// power plant (T-804, campaign "Rebellion" — symbiont_geo_playstyle.md §8):
// a second Overload/Recon target type that doesn't require a foreign
// player's dome nearby, so Resonance growth isn't geo-locked to human
// density.
type SabotageResult struct {
	Found     bool   `json:"found"`
	StationID string `json:"station_id,omitempty"`
	State     string `json:"state,omitempty"` // post-strike lifecycle state
	Ruined    bool   `json:"ruined,omitempty"`
}

// Sabotage forces the nearest hostile (not attacker-owned, not fellow-
// Symbiont-owned, not already ruins) plant within radiusM one step down its
// lifecycle: active→degrading, degrading→ruins. Unlike Corrode this isn't
// HP damage — plants don't have HP, they have a lifecycle (T-778) — so a
// sabotage strike reads as "forced early degradation", not a health bar hit.
func (s *Service) Sabotage(ctx context.Context, lat, lng float64, attackerID string, radiusM int) (*SabotageResult, error) {
	target, _, err := s.findHostile(ctx, lat, lng, attackerID, radiusM)
	if err != nil {
		return nil, fmt.Errorf("sabotage: %w", err)
	}
	if target == nil {
		return &SabotageResult{Found: false}, nil
	}
	newState, err := s.repo.DegradeOneStep(ctx, target.ID)
	if err != nil {
		return nil, fmt.Errorf("sabotage: degrade: %w", err)
	}
	return &SabotageResult{Found: true, StationID: target.ID, State: newState, Ruined: newState == "ruins"}, nil
}

// NearestResult reports a scouted hostile plant (T-804 Recon extension) —
// intel only, no state change.
type NearestResult struct {
	Found     bool   `json:"found"`
	StationID string `json:"station_id,omitempty"`
	DistanceM int    `json:"distance_m"`
	State     string `json:"state,omitempty"`
}

// NearestHostile scouts the nearest hostile plant within radiusM without
// touching its lifecycle — the Symbiont's "Разведать слабое место сети" verb
// (Recon), extended to the Rebellion-campaign target type alongside beacons.
func (s *Service) NearestHostile(ctx context.Context, lat, lng float64, attackerID string, radiusM int) (*NearestResult, error) {
	target, dist, err := s.findHostile(ctx, lat, lng, attackerID, radiusM)
	if err != nil {
		return nil, fmt.Errorf("nearest hostile: %w", err)
	}
	if target == nil {
		return &NearestResult{Found: false}, nil
	}
	return &NearestResult{Found: true, StationID: target.ID, DistanceM: int(dist), State: target.State}, nil
}

// findHostile locates the nearest plant within radiusM that isn't the
// attacker's own and isn't a fellow Symbiont's (T-801-equivalent) and isn't
// already ruins — shared by Sabotage and NearestHostile so both verbs agree
// on what counts as a valid target.
func (s *Service) findHostile(ctx context.Context, lat, lng float64, attackerID string, radiusM int) (*PowerPlant, float64, error) {
	plants, err := s.repo.ListInRadius(ctx, lat, lng, float64(radiusM))
	if err != nil {
		return nil, 0, fmt.Errorf("plants in radius: %w", err)
	}
	var target *PowerPlant
	best := math.MaxFloat64
	for i := range plants {
		p := &plants[i]
		if p.State == "ruins" {
			continue // already fully sabotaged, nothing left to take
		}
		if p.OwnerID == attackerID {
			continue // never your own plant
		}
		if s.isFellowSymbiont(ctx, p.OwnerID) {
			continue // don't target a fellow Symbiont's plant
		}
		if d := geo.Haversine(lat, lng, p.Lat, p.Lng); d < best {
			best = d
			target = p
		}
	}
	if target == nil {
		return nil, 0, nil
	}
	return target, best, nil
}

// isFellowSymbiont reports whether ownerID is a Symbiont — mirrors
// tower.Service.isFellowSymbiont (T-801): Sabotage is only ever invoked by a
// Symbiont attacker, so a Symbiont-owned plant is always same-faction.
// nil-safe / fail-open on error.
func (s *Service) isFellowSymbiont(ctx context.Context, ownerID string) bool {
	if s.faction == nil {
		return false
	}
	isSymbiont, err := s.faction.IsSymbiont(ctx, ownerID)
	return err == nil && isSymbiont
}
