package player

import (
	"context"
	"math"
	"strings"

	"github.com/ezra-game/server/internal/canon"
	"github.com/ezra-game/server/pkg/httputil"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) GetByID(ctx context.Context, id string) (*Player, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) GetByFirebaseUID(ctx context.Context, uid string) (*Player, error) {
	return s.repo.GetByFirebaseUID(ctx, uid)
}

func (s *Service) GetByLogin(ctx context.Context, login string) (*Player, error) {
	return s.repo.GetByLogin(ctx, login)
}

func (s *Service) CreatePlayerWithLogin(ctx context.Context, login, passwordHash string) (*Player, error) {
	p := &Player{
		Login:                  login,
		PasswordHash:           passwordHash,
		Username:               login,
		UsernameIsCustom:       false,
		OnboardingStep:         canon.OnboardingLoreStep,
		StarterBeaconAvailable: true,
		Level:                  1,
		XP:                     0,
		Energy:                 200,
		Materials:              50,
		ArmyLimit:              50,
		Skills:                 SkillPoints{},
		Position:               Position{},
	}
	if err := s.repo.CreateWithLogin(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *Service) CreatePlayer(ctx context.Context, firebaseUID, username string) (*Player, error) {
	p := &Player{
		FirebaseUID:            firebaseUID,
		Username:               username,
		UsernameIsCustom:       strings.TrimSpace(username) != "",
		OnboardingStep:         canon.OnboardingLoreStep,
		StarterBeaconAvailable: true,
		Level:                  1,
		XP:                     0,
		Energy:                 200,
		Materials:              50,
		ArmyLimit:              50,
		Skills:                 SkillPoints{},
		Position:               Position{},
	}
	if err := s.repo.Create(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *Service) UpdateUsername(ctx context.Context, playerID, username string) (*Player, error) {
	username = strings.TrimSpace(username)
	if len(username) < 3 {
		return nil, httputil.NewBadRequest("invalid_username", "username must be at least 3 characters")
	}

	p, err := s.repo.GetByID(ctx, playerID)
	if err != nil {
		return nil, err
	}
	p.Username = username
	p.UsernameIsCustom = true

	if err := s.repo.Update(ctx, p); err != nil {
		return nil, httputil.NewBadRequest("username_taken", "username already taken or invalid")
	}
	return p, nil
}

func (s *Service) AdvanceOnboarding(ctx context.Context, playerID, action string) (*Player, error) {
	p, err := s.repo.GetByID(ctx, playerID)
	if err != nil {
		return nil, err
	}

	switch action {
	case "complete_lore", "skip_lore":
		if p.OnboardingStep == canon.OnboardingNotStarted || p.OnboardingStep == canon.OnboardingLoreStep {
			p.OnboardingStep = canon.OnboardingFirstTowerStep
		}
	case "complete_symbiont_intro":
		// Symbiont-tutorial lore screen → first verb. See
		// docs/feature/symbiont_onboarding.md.
		if p.OnboardingStep == canon.OnboardingSymbiontIntro {
			p.OnboardingStep = canon.OnboardingSymbiontVerb
		}
	case "complete_symbiont_verb":
		// Verb tutorial (explained the pocket-carving verb) → entity tutorial.
		// Instructional: the techno verbs are spatially gated (foreign dome), so
		// the step advances on acknowledgement, not on a live cast.
		if p.OnboardingStep == canon.OnboardingSymbiontVerb {
			p.OnboardingStep = canon.OnboardingSymbiontEntity
		}
	case "complete_symbiont_entity":
		// Entity tutorial (explained manifesting an autonomous entity) → final
		// faction choice.
		if p.OnboardingStep == canon.OnboardingSymbiontEntity {
			p.OnboardingStep = canon.OnboardingFactionChoice
		}
	default:
		return nil, httputil.NewBadRequest("invalid_action", "unsupported onboarding action")
	}

	if err := s.repo.Update(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// OnboardingStep returns the player's current onboarding step (for the symbiont
// tutorial bridge). See docs/feature/symbiont_onboarding.md.
func (s *Service) OnboardingStep(ctx context.Context, playerID string) (string, error) {
	p, err := s.repo.GetByID(ctx, playerID)
	if err != nil {
		return "", err
	}
	return p.OnboardingStep, nil
}

// AdvanceOnboardingAction is the error-only wrapper around AdvanceOnboarding used
// by the symbiont service to advance the tutorial on a successful taught action.
func (s *Service) AdvanceOnboardingAction(ctx context.Context, playerID, action string) error {
	_, err := s.AdvanceOnboarding(ctx, playerID, action)
	return err
}

// FinishFactionChoice completes onboarding once the player makes the final
// faction choice (the faction_choice_step). No-op on any other step, so a later
// side-switch never re-triggers it. See docs/feature/symbiont_onboarding.md.
func (s *Service) FinishFactionChoice(ctx context.Context, playerID string) error {
	p, err := s.repo.GetByID(ctx, playerID)
	if err != nil {
		return err
	}
	if p.OnboardingStep != canon.OnboardingFactionChoice {
		return nil
	}
	p.OnboardingStep = canon.OnboardingCompleted
	return s.repo.Update(ctx, p)
}

// AddXP adds experience and handles level ups.
// XP formula: XP_for_next_level(n) = 100 * n^1.6
func (s *Service) AddXP(ctx context.Context, playerID string, amount int) (*Player, error) {
	p, err := s.repo.GetByID(ctx, playerID)
	if err != nil {
		return nil, err
	}

	if amount <= 0 {
		return p, nil
	}
	if p.Level >= maxPlayerLevel {
		p.XP += amount
		maxXP := xpForNextLevel(maxPlayerLevel)
		if p.XP > maxXP {
			p.XP = maxXP
		}
		if err := s.repo.Update(ctx, p); err != nil {
			return nil, err
		}
		return p, nil
	}

	p.XP += amount
	for p.Level < maxPlayerLevel && p.XP >= xpForNextLevel(p.Level) {
		p.XP -= xpForNextLevel(p.Level)
		p.Level++
	}
	if p.Level >= maxPlayerLevel {
		maxXP := xpForNextLevel(maxPlayerLevel)
		if p.XP > maxXP {
			p.XP = maxXP
		}
	}

	if err := s.repo.Update(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// xpForNextLevel returns XP needed to advance from level n.
// Formula: 100 * n^1.6
func xpForNextLevel(level int) int {
	return int(100 * math.Pow(float64(level), 1.6))
}

// XPForNextLevel is exported for use in API responses.
func XPForNextLevel(level int) int {
	return xpForNextLevel(level)
}
