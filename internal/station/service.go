package station

import (
	"context"
	"fmt"

	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/pkg/httputil"
)

// ResourceSpender charges the build cost.
type ResourceSpender interface {
	Spend(ctx context.Context, playerID string, energy, materials int) (*player.Player, error)
}

// Service implements power-plant build logic.
type Service struct {
	repo      Repository
	resources ResourceSpender
}

func NewService(repo Repository, resources ResourceSpender) *Service {
	return &Service{repo: repo, resources: resources}
}

// Build places a power plant at (lat,lng): spacing + per-radius cap checks, then
// charges the cost. The capacity boost is bounded by MaxInRadius and the global
// HARD_CAP, so a sparse area improves without overtaking a dense centre.
func (s *Service) Build(ctx context.Context, playerID string, lat, lng float64) (*PowerPlant, error) {
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
