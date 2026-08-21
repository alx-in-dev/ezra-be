package hearth

import (
	"context"
	"time"

	"github.com/ezra-game/server/pkg/httputil"
)

// FactionChecker reports a player's faction — Summon is Symbiont-only.
type FactionChecker interface {
	IsSymbiont(ctx context.Context, playerID string) (bool, error)
}

// Service implements Ephemeral Hearth summon + buff-lookup logic.
type Service struct {
	repo    Repository
	faction FactionChecker
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// WithFaction wires the optional faction gate. Kept off the constructor so
// existing callers/tests stay unchanged.
func (s *Service) WithFaction(f FactionChecker) *Service {
	s.faction = f
	return s
}

// SummonResult is returned after a successful Summon.
type SummonResult struct {
	ID        string    `json:"id"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Summon plants a new hearth at (lat,lng): Symbiont-only, cap=1 active +
// CooldownMin between summons (RED LINE #1: it's an event, not a base — no
// upkeep, no upgrade, it just expires).
func (s *Service) Summon(ctx context.Context, playerID string, lat, lng float64) (*SummonResult, error) {
	if s.faction != nil {
		if isSymbiont, err := s.faction.IsSymbiont(ctx, playerID); err != nil || !isSymbiont {
			return nil, httputil.NewForbidden("not_symbiont", "доступно только Симбионтам")
		}
	}
	last, ok, err := s.repo.LastByOwner(ctx, playerID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if ok {
		if now.Before(last.ExpiresAt) {
			return nil, httputil.NewBadRequest("hearth_active", "у вас уже есть активный Очаг")
		}
		if wait := last.CreatedAt.Add(CooldownMin * time.Minute).Sub(now); wait > 0 {
			return nil, httputil.NewBadRequest("hearth_cooldown", "Очаг ещё не восстановился")
		}
	}
	h := &EphemeralHearth{OwnerID: playerID, Lat: lat, Lng: lng, ExpiresAt: now.Add(TTLMin * time.Minute)}
	if err := s.repo.Create(ctx, h); err != nil {
		return nil, err
	}
	return &SummonResult{ID: h.ID, ExpiresAt: h.ExpiresAt}, nil
}

// Buffed reports whether (lat,lng) is within range of any still-active
// hearth — any owner's; the buff is shared, that's the point (coordinated
// raid, Core Drive 5 Social).
func (s *Service) Buffed(ctx context.Context, lat, lng float64) (bool, error) {
	return s.repo.ActiveNearby(ctx, lat, lng, BuffRadiusM)
}
