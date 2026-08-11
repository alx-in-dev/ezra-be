package hive

import (
	"context"
	"log/slog"

	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/internal/rift"
	"github.com/ezra-game/server/pkg/httputil"
)

// RiftSpawner seeds rifts in a hive's zone (satisfied by *rift.Service).
type RiftSpawner interface {
	MaybeSpawn(ctx context.Context, cellID string, infection float64) (*rift.Rift, error)
}

// FactionChecker gates the empower action to Symbiont players.
type FactionChecker interface {
	IsSymbiont(ctx context.Context, playerID string) (bool, error)
}

// ResourceSpender charges the empower energy cost.
type ResourceSpender interface {
	Spend(ctx context.Context, playerID string, energy, materials int) (*player.Player, error)
}

// EmpowerEnergyCost / EmpowerIntensity tune the Symbiont "усилить" verb.
const (
	EmpowerEnergyCost   = 100
	EmpowerIntensity    = 25
	ResonancePerEmpower = 5 // portable Symbiont Resonance per empower (#6)
)

// ResonanceGranter credits portable Symbiont Resonance (#6 baseless progression).
type ResonanceGranter interface {
	AddSymbiontResonance(ctx context.Context, playerID string, delta int) (int, error)
}

// Tuning for the hive lifecycle.
const (
	seedMinInfection = 95.0  // a cell must be this infected to grow a new hive
	seedExcludeM     = 700.0 // ...and have no open hive this close
	riftSeedMinInf   = 78.0  // infection a zone cell needs before the hive seeds a rift
	maxOpenHives     = 12    // safety cap so the world doesn't fill with hives

	// Each won squad assault drops the hive's intensity by this much; at 0 the
	// hive collapses. Full hive (100) → ~2 assaults to close.
	assaultIntensityDamage = 55
	// On collapse, infection in the zone is knocked down to this (a partial
	// cleanse — permanent stabilization is the resonance endgame, D1).
	collapseCleanseLevel = 25.0
)

// Service runs the Symbiont hive lifecycle (E1).
type Service struct {
	hives     Repository
	rifts     RiftSpawner
	faction   FactionChecker
	resources ResourceSpender
	resonance ResonanceGranter
}

// SetResonanceGranter wires portable Symbiont Resonance crediting on empower (#6).
func (s *Service) SetResonanceGranter(g ResonanceGranter) { s.resonance = g }

func NewService(hives Repository, rifts RiftSpawner) *Service {
	return &Service{hives: hives, rifts: rifts}
}

// SetEmpowerDeps wires the optional Symbiont-empower dependencies (R4-2 slice).
func (s *Service) SetEmpowerDeps(faction FactionChecker, resources ResourceSpender) {
	s.faction = faction
	s.resources = resources
}

// Empower is the Symbiont's verb: feed a hive to raise its intensity/level
// (the inverse of a human assault). Gated to Symbiont-aligned players.
func (s *Service) Empower(ctx context.Context, playerID, hiveID string) (*Hive, error) {
	if s.faction != nil {
		sym, err := s.faction.IsSymbiont(ctx, playerID)
		if err != nil {
			return nil, err
		}
		if !sym {
			return nil, httputil.NewForbidden("not_symbiont", "усиливать Очаг могут только Симбионты")
		}
	}
	h, err := s.hives.GetByID(ctx, hiveID)
	if err != nil {
		return nil, httputil.NewNotFound("not_found", "hive not found")
	}
	if s.resources != nil {
		if _, err := s.resources.Spend(ctx, playerID, EmpowerEnergyCost, 0); err != nil {
			return nil, err
		}
	}
	intensity, level, err := s.hives.Empower(ctx, hiveID, EmpowerIntensity)
	if err != nil {
		return nil, err
	}
	h.Intensity = intensity
	h.Level = level
	h.decorate()

	// #6 baseless progression: empowering a hive feeds portable Resonance.
	if s.resonance != nil {
		if _, err := s.resonance.AddSymbiontResonance(ctx, playerID, ResonancePerEmpower); err != nil {
			slog.Error("empower: grant resonance failed", "player_id", playerID, "error", err)
		}
	}
	return h, nil
}

// GetNearby returns open hives near a point (for the client map).
func (s *Service) GetNearby(ctx context.Context, lat, lng, radiusM float64) ([]Hive, error) {
	return s.hives.GetNearby(ctx, lat, lng, radiusM)
}

// GetByID returns a hive (for the battle system to load defenders).
func (s *Service) GetByID(ctx context.Context, id string) (*Hive, error) {
	return s.hives.GetByID(ctx, id)
}

// OnAssaultVictory applies one won squad assault: the hive loses intensity, and
// if depleted it collapses — closed and its region partially cleansed. Returns
// whether the hive was closed by this assault.
func (s *Service) OnAssaultVictory(ctx context.Context, hiveID string) (bool, error) {
	intensity, err := s.hives.ReduceIntensity(ctx, hiveID, assaultIntensityDamage)
	if err != nil {
		return false, err
	}
	if intensity > 0 {
		slog.Info("hive weakened by assault", "hive_id", hiveID, "intensity", intensity)
		return false, nil
	}
	// Collapsed.
	h, err := s.hives.GetByID(ctx, hiveID)
	if err != nil {
		return false, err
	}
	if err := s.hives.Close(ctx, hiveID); err != nil {
		return false, err
	}
	cleansed, err := s.hives.CleanseRegion(ctx, h, collapseCleanseLevel)
	if err != nil {
		slog.Error("hive cleanse failed", "hive_id", hiveID, "error", err)
	}
	slog.Info("symbiont hive collapsed", "hive_id", hiveID, "cells_cleansed", cleansed)
	return true, nil
}

// Pulse runs one tick: every open hive pushes infection into its zone and tries
// to seed a rift on its hottest cell. Then a new hive may grow in the worst
// uncovered infected spot. Mirror of how a Core powers its beacons.
func (s *Service) Pulse(ctx context.Context) error {
	open, err := s.hives.GetOpen(ctx)
	if err != nil {
		return err
	}
	for i := range open {
		h := &open[i]
		// Weaker (lower-intensity) hives pulse proportionally less.
		amount := Config(h.Level).PulsePerTick * float64(h.Intensity) / 100.0
		if amount <= 0 {
			continue
		}
		if _, err := s.hives.PulseInfection(ctx, h, amount); err != nil {
			slog.Error("hive pulse failed", "hive_id", h.ID, "error", err)
			continue
		}
		if s.rifts != nil {
			if cellID, _, _, inf, ok, hErr := s.hives.HottestCellInZone(ctx, h, riftSeedMinInf); hErr == nil && ok {
				if _, err := s.rifts.MaybeSpawn(ctx, cellID, inf); err != nil {
					slog.Warn("hive rift seed failed", "hive_id", h.ID, "cell_id", cellID, "error", err)
				}
			}
		}
	}

	if len(open) < maxOpenHives {
		s.maybeSeed(ctx)
	}
	return nil
}

// maybeSeed grows at most one new hive per tick in the worst infected spot.
func (s *Service) maybeSeed(ctx context.Context) {
	cellID, lat, lng, ok, err := s.hives.SeedCandidate(ctx, seedMinInfection, seedExcludeM)
	if err != nil {
		slog.Error("hive seed candidate failed", "error", err)
		return
	}
	if !ok {
		return
	}
	h := &Hive{CellID: cellID, Lat: lat, Lng: lng, Level: 1, Intensity: 100}
	if err := s.hives.Create(ctx, h); err != nil {
		slog.Error("hive create failed", "cell_id", cellID, "error", err)
		return
	}
	slog.Info("symbiont hive seeded", "hive_id", h.ID, "cell_id", cellID, "lat", lat, "lng", lng)
}
