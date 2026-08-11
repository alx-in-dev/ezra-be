package faction

import (
	"context"
	"fmt"
	"time"

	"github.com/ezra-game/server/internal/canon"
	"github.com/ezra-game/server/pkg/httputil"
)

// SymbiontPlayable gates the full symbiont playstyle. The R4-2 roster (entities,
// verbs, Attunement, Resonance) is built and the onboarding now walks the player
// through it, so the full path is open.
const SymbiontPlayable = true

// ResonanceReader reads a player's portable Symbiont Resonance (#6). Optional
// (nil-safe) so tests and the base service stay decoupled from the player repo.
type ResonanceReader interface {
	SymbiontResonanceOf(ctx context.Context, playerID string) (int, error)
}

// ProgressReader reads the Resonance roster progression (R4-2). Optional.
type ProgressReader interface {
	ResonanceProgressOf(ctx context.Context, playerID string) (pool, level, xp int, err error)
}

// RosterInitializer seeds the Symbiont roster (Resonance Pool) when a player
// declares allegiance (R4-2). Optional; nil-safe. Implemented by roster.Service.
type RosterInitializer interface {
	OnFactionChosen(ctx context.Context, playerID, side string) error
}

// OnboardingFinisher closes out the onboarding faction-choice step (sets
// onboarding to completed when the player was choosing). Optional; nil-safe.
// Implemented by player.Service. See docs/feature/symbiont_onboarding.md.
type OnboardingFinisher interface {
	FinishFactionChoice(ctx context.Context, playerID string) error
}

type Service struct {
	repo       Repository
	resonance  ResonanceReader
	progress   ProgressReader
	roster     RosterInitializer
	onboarding OnboardingFinisher
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// SetResonanceReader wires live Symbiont Resonance into the faction status (#6).
func (s *Service) SetResonanceReader(r ResonanceReader) { s.resonance = r }

// SetProgressReader wires the Resonance roster progression into the status (R4-2).
func (s *Service) SetProgressReader(p ProgressReader) { s.progress = p }

// SetRosterInitializer wires the Resonance Pool conversion on side-choice (R4-2).
func (s *Service) SetRosterInitializer(r RosterInitializer) { s.roster = r }

// SetOnboardingFinisher wires the onboarding faction-choice completion.
func (s *Service) SetOnboardingFinisher(f OnboardingFinisher) { s.onboarding = f }

func (s *Service) Status(ctx context.Context, playerID string) (*State, error) {
	fac, contacted, chosen, err := s.repo.Get(ctx, playerID)
	if err != nil {
		return nil, err
	}
	resonance := 0
	if s.resonance != nil {
		if r, rErr := s.resonance.SymbiontResonanceOf(ctx, playerID); rErr == nil {
			resonance = r
		}
	}
	st := &State{
		Faction:           fac,
		Contacted:         contacted,
		Chosen:            chosen,
		CanChoose:         contacted,
		SymbiontPlayable:  SymbiontPlayable,
		FactionRu:         FactionRu(fac),
		SymbiontResonance: resonance,
	}
	if s.progress != nil {
		if pool, level, xp, pErr := s.progress.ResonanceProgressOf(ctx, playerID); pErr == nil {
			if level < 1 {
				level = 1
			}
			st.ResonancePool = pool
			st.ResonanceLevel = level
			st.ResonanceXP = xp
			st.ResonanceControl = canon.ResonanceControlCap(level)
			st.ResonanceXPNext = canon.ResonanceXPForNextLevel(level)
		}
	}
	return st, nil
}

// MarkContacted records that the player experienced Contact (e.g. collapsed a
// Hive), unlocking the side choice. Idempotent.
func (s *Service) MarkContacted(ctx context.Context, playerID string) error {
	return s.repo.SetContacted(ctx, playerID)
}

// EnterTemporarySymbiont flips the player into a temporary symbiont (faction=
// symbiont, chosen=false). The tutorial entity comes from the roster's lazy
// EnsureStarterRoster (a zero pool still yields the guaranteed assault) — the
// Resonance Pool itself is deliberately NOT seeded here: the tutorial fires
// before the player has any progress, and the pool write is one-shot, so
// seeding at this point froze every account at the floor value forever. The
// real conversion happens on the first actual Choose(symbiont).
func (s *Service) EnterTemporarySymbiont(ctx context.Context, playerID string) error {
	return s.repo.SetFaction(ctx, playerID, Symbiont, false)
}

// Choose declares allegiance. Requires Contact first. The choice is switchable
// until the v1.1 symbiont path lands (then it becomes no-rollback per canon).
func (s *Service) Choose(ctx context.Context, playerID, side string) (*State, error) {
	if !ValidSide(side) {
		return nil, httputil.NewBadRequest("invalid_side", "сторона должна быть human или symbiont")
	}
	current, contacted, chosen, err := s.repo.Get(ctx, playerID)
	if err != nil {
		return nil, err
	}
	if !contacted {
		return nil, httputil.NewBadRequest("not_contacted", "сначала нужно пережить Контакт с Эзра (закрыть Очаг)")
	}
	// A flip (not the first choice, not a same-side no-op) honors the switch
	// cooldown — instant re-flipping reset the legacy-decay timer forever.
	if chosen && current != side {
		if reader, ok := s.repo.(interface {
			LastChangeAt(ctx context.Context, playerID string) (time.Time, bool, error)
		}); ok {
			if at, found, err := reader.LastChangeAt(ctx, playerID); err == nil && found {
				if wait := SwitchCooldown - time.Since(at); wait > 0 {
					return nil, httputil.NewBadRequest("faction_cooldown",
						fmt.Sprintf("сменить сторону можно через %s", wait.Round(time.Minute)))
				}
			}
		}
	}
	if err := s.repo.Choose(ctx, playerID, side); err != nil {
		return nil, err
	}
	// Seed the Resonance Pool from human progression on the first symbiont choice
	// (R4-2). Best-effort: a conversion hiccup must not block the side-choice.
	if s.roster != nil {
		_ = s.roster.OnFactionChosen(ctx, playerID, side)
	}
	// If this choice is the onboarding's final step, finish onboarding. Choosing
	// "human" here reverts the temporary symbiont; "symbiont" confirms it.
	if s.onboarding != nil {
		_ = s.onboarding.FinishFactionChoice(ctx, playerID)
	}
	return s.Status(ctx, playerID)
}

// FactionOf returns the player's current side (for the faction war).
func (s *Service) FactionOf(ctx context.Context, playerID string) (string, error) {
	fac, _, _, err := s.repo.Get(ctx, playerID)
	return fac, err
}

// IsSymbiont reports whether the player has sided with the Symbionts.
func (s *Service) IsSymbiont(ctx context.Context, playerID string) (bool, error) {
	fac, err := s.FactionOf(ctx, playerID)
	return fac == Symbiont, err
}
