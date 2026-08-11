// Package roster implements the Symbiont Resonance roster (R4-2,
// symbiont_roster.md). Layer 1: the one-time Resonance Pool conversion — when a
// human sides with the Symbionts, their progression (units / beacons / pets)
// leaves a "резонансный след" that seeds the starter roster.
package roster

import (
	"context"
	"time"

	"github.com/ezra-game/server/internal/canon"
	"github.com/ezra-game/server/internal/faction"
	"github.com/ezra-game/server/internal/pet"
	"github.com/ezra-game/server/internal/tower"
	"github.com/ezra-game/server/internal/unit"
)

// UnitLister lists a player's human line units (for the Pool conversion).
type UnitLister interface {
	GetByPlayer(ctx context.Context, playerID string) ([]unit.Unit, error)
}

// TowerLister lists a player's beacons.
type TowerLister interface {
	GetByOwner(ctx context.Context, playerID string) ([]tower.Tower, error)
}

// PetLister lists a player's pets/drones.
type PetLister interface {
	GetByPlayer(ctx context.Context, playerID string) ([]pet.Pet, error)
}

// PoolStore seeds the Resonance Pool once (idempotent across re-switches).
type PoolStore interface {
	SetResonancePool(ctx context.Context, id string, pool int) (int, error)
}

// ResonanceReader reads a player's Resonance progression (pool/level/xp), used
// to derive the entity control-slot cap (canon ResonanceControlCap).
type ResonanceReader interface {
	ResonanceProgressOf(ctx context.Context, id string) (pool, level, xp int, err error)
}

// Service computes and seeds the Resonance Pool and manages the entity roster
// (R4-2 L2). Implements faction.RosterInitializer so it runs on the side-choice.
type Service struct {
	units     UnitLister
	towers    TowerLister
	pets      PetLister
	pool      PoolStore
	entities  EntityRepository
	resonance ResonanceReader
	// effectors for the autonomous tick (L2b); nil-safe.
	corroder  TowerCorroder
	amplifier RiftAmplifier
	now       func() time.Time
}

func NewService(units UnitLister, towers TowerLister, pets PetLister, pool PoolStore) *Service {
	return &Service{units: units, towers: towers, pets: pets, pool: pool, now: time.Now}
}

// WithEntities wires the entity roster store and the Resonance reader (control
// cap). Optional dependency; entity endpoints/tick are no-ops without it.
func (s *Service) WithEntities(entities EntityRepository, resonance ResonanceReader) *Service {
	s.entities = entities
	s.resonance = resonance
	return s
}

var _ faction.RosterInitializer = (*Service)(nil)

// ComputePool sums a player's human progression into a Resonance Pool value
// (symbiont_roster.md weights). Missing sources are simply skipped.
func (s *Service) ComputePool(ctx context.Context, playerID string) (int, error) {
	total := 0
	if s.units != nil {
		units, err := s.units.GetByPlayer(ctx, playerID)
		if err != nil {
			return 0, err
		}
		for i := range units {
			total += canon.PoolPerUnit(units[i].Type)
		}
	}
	if s.towers != nil {
		towers, err := s.towers.GetByOwner(ctx, playerID)
		if err != nil {
			return 0, err
		}
		for i := range towers {
			total += canon.PoolPerBeacon(towers[i].Level)
		}
	}
	if s.pets != nil {
		pets, err := s.pets.GetByPlayer(ctx, playerID)
		if err != nil {
			return 0, err
		}
		for i := range pets {
			if pets[i].Level >= 3 {
				total += canon.PoolPerPetHigh
			}
		}
	}
	return total, nil
}

// OnFactionChosen seeds the Resonance Pool the first time a player sides with the
// Symbionts (no-op for Humans / already-seeded). Floored at PoolStarterMin so
// the guaranteed starter (1 assault entity) always unlocks. After seeding, the
// roster is synced up to what the stored pool grants — the tutorial may have
// already materialized the guaranteed assault at pool 0, and EnsureStarterRoster
// alone no-ops once any entity exists.
func (s *Service) OnFactionChosen(ctx context.Context, playerID, side string) error {
	if side != faction.Symbiont {
		return nil
	}
	pool, err := s.ComputePool(ctx, playerID)
	if err != nil {
		return err
	}
	if pool < canon.PoolStarterMin {
		pool = canon.PoolStarterMin
	}
	stored, err := s.pool.SetResonancePool(ctx, playerID, pool)
	if err != nil {
		return err
	}
	return s.SyncStarterRoster(ctx, playerID, stored)
}
