// Package quickstart implements the onboarding quick-start path: at the very
// first login a player may skip the narrative chain and go straight to a
// side. See docs/feature/onboarding_quick_start.md.
package quickstart

import (
	"context"
	"fmt"

	"github.com/ezra-game/server/internal/canon"
	"github.com/ezra-game/server/internal/faction"
	"github.com/ezra-game/server/internal/nest"
	"github.com/ezra-game/server/internal/pet"
	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/internal/unit"
)

// Starter grants for the quick-start Human path mirror what a full onboarding
// awards for the steps it skips: 2 idle units (survivor_step's gate),
// battle.tutorialEncounter's loot (tutorial_battle_step), and the starter pet
// (pet_unlock_step). Kept here in sync manually — quick-start never runs the
// actual tutorial battle.
const (
	starterEnergy    = 40
	starterMaterials = 8
	starterBattleXP  = 40
)

// starterUnitTypes are recruited in order until the player has at least 2 —
// the count survivor.Recruit's own onboarding gate requires.
var starterUnitTypes = []string{"fighter", "engineer"}

type Service struct {
	units     unit.Repository
	pets      pet.Repository
	resources *player.ResourceService
	players   *player.Service
	faction   *faction.Service
	nest      *nest.Service
}

func NewService(units unit.Repository, pets pet.Repository, resources *player.ResourceService, players *player.Service, factionSvc *faction.Service, nestSvc *nest.Service) *Service {
	return &Service{units: units, pets: pets, resources: resources, players: players, faction: factionSvc, nest: nestSvc}
}

// StartHuman flags the player for the quick-start Human path and advances
// them to placing their first beacon — the one manual, GPS-bound action this
// path keeps. FinishHumanSetup grants the rest once they place it.
func (s *Service) StartHuman(ctx context.Context, playerID string) (*player.Player, error) {
	return s.players.StartHumanQuickStart(ctx, playerID)
}

// FinishHumanSetup grants the tutorial milestones a full onboarding would have
// earned (2nd survivor, tutorial-battle rewards, starter pet) and completes
// onboarding. Called by tower.Service right after a quick-start Human places
// their first beacon.
func (s *Service) FinishHumanSetup(ctx context.Context, playerID string) error {
	if err := s.grantStarterUnits(ctx, playerID); err != nil {
		return fmt.Errorf("grant starter units: %w", err)
	}
	if _, err := s.resources.Add(ctx, playerID, starterEnergy, starterMaterials); err != nil {
		return fmt.Errorf("grant tutorial battle resources: %w", err)
	}
	if _, err := s.players.AddXP(ctx, playerID, starterBattleXP); err != nil {
		return fmt.Errorf("grant tutorial battle xp: %w", err)
	}
	if err := s.grantStarterPet(ctx, playerID); err != nil {
		return fmt.Errorf("grant starter pet: %w", err)
	}
	if err := s.players.ForceCompleteOnboarding(ctx, playerID); err != nil {
		return fmt.Errorf("complete onboarding: %w", err)
	}
	return nil
}

func (s *Service) grantStarterUnits(ctx context.Context, playerID string) error {
	count, err := s.units.CountByPlayer(ctx, playerID)
	if err != nil {
		return err
	}
	for i := count; i < 2; i++ {
		t := starterUnitTypes[i%len(starterUnitTypes)]
		stats := unit.BaseStats[t]
		u := &unit.Unit{
			PlayerID: playerID,
			Type:     t,
			Level:    1,
			HP:       stats.HP,
			ATK:      stats.ATK,
			Status:   "idle",
		}
		if err := s.units.Create(ctx, u); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) grantStarterPet(ctx context.Context, playerID string) error {
	pets, err := s.pets.GetByPlayer(ctx, playerID)
	if err != nil {
		return err
	}
	if len(pets) > 0 {
		return nil
	}
	starter := &pet.Pet{
		PlayerID: playerID,
		Name:     "Scout",
		Type:     "scout",
		Level:    1,
		Status:   canon.PetStateIdle,
	}
	return s.pets.Create(ctx, starter)
}

// FinishSymbiont instantly commits the player to the Symbiont side (no Contact
// battle needed), completes onboarding, and opens their home Nest at their
// current position — a born-Symbiont needs no other setup (RED LINE #5:
// Symbionts never touch the human toolkit, so there's nothing else to grant).
func (s *Service) FinishSymbiont(ctx context.Context, playerID string) (*nest.Nest, error) {
	if _, err := s.players.RequireQuickStartEligible(ctx, playerID); err != nil {
		return nil, err
	}
	if _, err := s.faction.ChooseInstant(ctx, playerID); err != nil {
		return nil, fmt.Errorf("choose symbiont: %w", err)
	}
	if err := s.players.ForceCompleteOnboarding(ctx, playerID); err != nil {
		return nil, fmt.Errorf("complete onboarding: %w", err)
	}
	n, err := s.nest.OpenFirstNest(ctx, playerID, "")
	if err != nil {
		return nil, fmt.Errorf("open home nest: %w", err)
	}
	return n, nil
}
