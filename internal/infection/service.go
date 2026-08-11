package infection

import (
	"context"
	"log/slog"
	"math"

	"github.com/ezra-game/server/internal/cell"
	"github.com/ezra-game/server/internal/rift"
	"github.com/ezra-game/server/internal/tower"
	"github.com/ezra-game/server/pkg/geo"
)

const (
	baseGrowthPerHour = 0.8
	neighborThreshold = 50.0
	neighborBonus     = 0.3
	neighborRadiusM   = 75.0 // diagonal of the 50x50m grid, same as cell.GetNeighbors
	recalcIntervalMin = 5.0
	minutesPerHour    = 60.0
	maxRiftSearchM    = 200.0
	maxTowerSearchM   = 400.0 // max tower radius at level 5
	towerFalloff      = 0.4   // effect loss at the edge of the tower radius

	// Civic Spire suppression per hour for cells in an active Spire's radius.
	// Mirrors spire.SuppressionPerHour (kept local to avoid an import cycle).
	spireSuppressionPerHour = 1.5

	// D2 antagonist tide (directed infection advance toward player beacons).
	tideStepPerTick       = 6.0   // infection points added to an advancing cell per tick
	tideAdvanceCeiling    = 90.0  // only push cells below this (criticals still form via rifts)
	tideRangeM            = 250.0 // operate this close to a beacon
	tideFrontierThreshold = 55.0  // a neighbour must be this infected to push inward
)

// Rift bonuses per type (percent per hour).
var riftBonusPerHour = map[string]float64{
	"minor":    2.5,
	"medium":   5.0,
	"critical": 10.0,
}

// Service encapsulates the infection recalculation logic.
type Service struct {
	cells  cell.Repository
	rifts  rift.Repository
	towers tower.Repository
	batch  Repository
}

// NewService creates a new infection Service.
func NewService(cells cell.Repository, rifts rift.Repository, towers tower.Repository) *Service {
	return &Service{cells: cells, rifts: rifts, towers: towers}
}

// WithBatchRepository enables set-based recalculation for RecalculateAll.
func (s *Service) WithBatchRepository(r Repository) *Service {
	s.batch = r
	return s
}

// RecalculateCell recomputes the infection level for a single cell.
// suppressionChecker is optionally implemented by the cell repository (the
// production PgRepository does). Lets the per-cell path honour dome/stabilized
// suppression like BatchRecalculate, without putting it on the Repository
// interface (which every mock would then have to implement).
type suppressionChecker interface {
	IsSuppressed(ctx context.Context, cellID string) (bool, error)
}

func (s *Service) RecalculateCell(ctx context.Context, cellID string) (*RecalcResult, error) {
	c, err := s.cells.GetByID(ctx, cellID)
	if err != nil {
		return nil, err
	}

	// Domed / counter-wave-stabilized cells never grow — mirror the batch path
	// so the (dev-only) per-cell fallback doesn't silently re-infect protected
	// zones. Suppressed cells decay toward 0 instead.
	if sc, ok := s.cells.(suppressionChecker); ok {
		if suppressed, sErr := sc.IsSuppressed(ctx, cellID); sErr == nil && suppressed {
			decay := domeSuppressionPerHour() * (recalcIntervalMin / minutesPerHour)
			newInfection := math.Max(0, c.Infection-decay)
			if err := s.cells.Update(ctx, cellID, newInfection); err != nil {
				return nil, err
			}
			return &RecalcResult{
				CellID:       cellID,
				OldInfection: c.Infection,
				NewInfection: newInfection,
			}, nil
		}
	}

	// --- Growth ---
	growth := baseGrowthPerHour

	// Neighbor bonus: +0.3 %/hr per neighbor with infection > 50%
	neighbors, err := s.cells.GetNeighbors(ctx, cellID)
	if err == nil {
		for _, n := range neighbors {
			if n.Infection > neighborThreshold {
				growth += neighborBonus
			}
		}
	}

	riftEffect := 0.0
	if s.rifts != nil {
		nearbyRifts, err := s.rifts.GetNearby(ctx, c.Lat, c.Lng, maxRiftSearchM)
		if err == nil {
			for _, nearbyRift := range nearbyRifts {
				riftEffect += riftBonusForType(nearbyRift.Type)
			}
		}
	}

	// --- Tower effect ---
	towerEffect := 0.0
	nearbyTowers, err := s.towers.GetInRadius(ctx, c.Lat, c.Lng, maxTowerSearchM)
	if err == nil {
		for _, t := range nearbyTowers {
			tc, tcErr := s.cells.GetByID(ctx, t.CellID)
			if tcErr != nil {
				continue
			}
			dist := geo.Haversine(c.Lat, c.Lng, tc.Lat, tc.Lng)
			if dist > t.RadiusM {
				continue
			}
			falloff := 1.0 - towerFalloff*(dist/t.RadiusM)
			towerEffect += t.EffectPerHour * falloff
		}
	}

	// Scale hourly rates to 5-minute interval (1/12 of an hour).
	intervalFraction := recalcIntervalMin / minutesPerHour
	deltaGrowth := (growth + riftEffect) * intervalFraction
	deltaTower := towerEffect * intervalFraction

	newInfection := c.Infection + deltaGrowth - deltaTower
	newInfection = math.Max(0, math.Min(100, newInfection))

	if err := s.cells.Update(ctx, cellID, newInfection); err != nil {
		return nil, err
	}

	return &RecalcResult{
		CellID:       cellID,
		OldInfection: c.Infection,
		NewInfection: newInfection,
		Growth:       deltaGrowth,
		RiftEffect:   riftEffect * intervalFraction,
		TowerEffect:  deltaTower,
	}, nil
}

func riftBonusForType(riftType string) float64 {
	if bonus, ok := riftBonusPerHour[riftType]; ok {
		return bonus
	}
	return 0.0
}

// RecalculateAll recomputes infection for every cell in the database.
// With a batch repository it runs as one set-based SQL statement; the
// per-cell loop remains as a fallback (O(N) queries — does not scale past
// a few thousand cells, see gotchas W-04).
func (s *Service) RecalculateAll(ctx context.Context) error {
	if s.batch != nil {
		updated, err := s.batch.BatchRecalculate(ctx)
		if err != nil {
			return err
		}
		slog.Info("infection batch recalculation done", "cells", updated)
		return nil
	}

	allCells, err := s.cells.GetAll(ctx)
	if err != nil {
		return err
	}

	for _, c := range allCells {
		if _, err := s.RecalculateCell(ctx, c.ID); err != nil {
			slog.Error("recalculate cell failed", "cell_id", c.ID, "error", err)
			continue
		}
	}

	return nil
}

// AdvanceTide runs one tick of the D2 active antagonist — infection channels
// toward player beacons. No-op without the set-based repository.
func (s *Service) AdvanceTide(ctx context.Context) error {
	if s.batch == nil {
		return nil
	}
	advanced, err := s.batch.AdvanceTide(ctx)
	if err != nil {
		return err
	}
	slog.Info("antagonist tide advanced", "cells", advanced)
	return nil
}
