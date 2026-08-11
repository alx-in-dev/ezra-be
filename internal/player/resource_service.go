package player

import (
	"context"
	"log/slog"
	"time"

	"github.com/ezra-game/server/pkg/httputil"
)

const (
	MaxEnergy    = 2000
	MaxMaterials = 500
	// EnergyCapPerLevel is the free storage allowance added per player level
	// above 1 (anti-P2W: capacity scales with progression, not purchases). See
	// Player.EnergyMax and docs/feature/balance_tables.md (C6).
	EnergyCapPerLevel = 100
	// MaterialsCapPerLevel is the 🔩 counterpart — see Player.MaterialsMax.
	MaterialsCapPerLevel = 15
	// OfflineIncomeWindow caps how long passive accruals (tower income) keep
	// flowing while a player is away. After the window the worker stops
	// adding income until the player returns (last_active updates) or frees
	// storage by spending. See docs/feature/balance_tables.md "Offline income".
	OfflineIncomeWindow = 24 * time.Hour
)

type ResourceService struct {
	repo Repository
}

func NewResourceService(repo Repository) *ResourceService {
	return &ResourceService{repo: repo}
}

func (s *ResourceService) Spend(ctx context.Context, playerID string, energy, materials int) (*Player, error) {
	p, err := s.repo.GetByID(ctx, playerID)
	if err != nil {
		return nil, err
	}

	if p.Energy < energy || p.Materials < materials {
		return nil, httputil.NewBadRequest("insufficient_resources", "not enough energy or materials")
	}

	p.Energy -= energy
	p.Materials -= materials
	if err := s.repo.Update(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *ResourceService) Add(ctx context.Context, playerID string, energy, materials int) (*Player, error) {
	p, err := s.repo.GetByID(ctx, playerID)
	if err != nil {
		return nil, err
	}

	p.Energy = min(p.Energy+energy, p.EnergyMax())
	p.Materials = min(p.Materials+materials, p.MaterialsMax())
	if err := s.repo.Update(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// AddPassiveIncome credits tower passive income to a player while honoring
// the offline income window from balance_tables.md: after the player has been
// inactive for more than OfflineIncomeWindow, accruals pause until the player
// returns (last_active updates). Returns (nil, nil) when the player is paused
// — that is not an error, just a no-op.
//
// The storage cap is enforced by Add via min(); overflow is silently
// discarded as canon dictates ("всё, что выше cap, отсекается и не уходит в долг").
func (s *ResourceService) AddPassiveIncome(ctx context.Context, playerID string, energy, materials int) (*Player, error) {
	p, err := s.repo.GetByID(ctx, playerID)
	if err != nil {
		return nil, err
	}

	if !p.LastActive.IsZero() && time.Since(p.LastActive) > OfflineIncomeWindow {
		slog.Debug("passive income paused (offline > 24h)",
			"player_id", playerID,
			"last_active", p.LastActive)
		return nil, nil
	}

	p.Energy = min(p.Energy+energy, p.EnergyMax())
	p.Materials = min(p.Materials+materials, p.MaterialsMax())
	if err := s.repo.Update(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}
