package capture

import (
	"context"
	"log/slog"

	"github.com/ezra-game/server/internal/canon"
	"github.com/ezra-game/server/internal/cell"
	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/internal/tower"
	"github.com/ezra-game/server/pkg/geo"
	"github.com/ezra-game/server/pkg/httputil"
)

// PushNotifier abstracts the push service so capture can notify defenders
// without taking a hard dependency on internal/push.
type PushNotifier interface {
	NotifyTowerCaptured(ctx context.Context, defenderID, attackerName string) error
	NotifyTowerUnderAttack(ctx context.Context, defenderID, attackerName string) error
}

// Service implements tower capture logic.
type Service struct {
	towers  tower.Repository
	players player.Repository
	cells   cell.Repository
	push    PushNotifier
}

// NewService creates a new capture service.
func NewService(towers tower.Repository, players player.Repository, cells cell.Repository, push PushNotifier) *Service {
	return &Service{towers: towers, players: players, cells: cells, push: push}
}

// Lockpick transfers tower ownership to the attacking player if within 50m.
// MVP: no lockpick resource check — proximity is enough.
func (s *Service) Lockpick(ctx context.Context, towerID, playerID string) (*tower.Tower, error) {
	if !canon.IsFeatureAvailable(canon.FeatureTowerPressure) {
		return nil, httputil.NewBadRequest("feature_locked", "tower pressure flow is not available in the current release")
	}

	t, err := s.towers.GetByID(ctx, towerID)
	if err != nil {
		return nil, httputil.NewNotFound("not_found", "tower not found")
	}

	if t.OwnerID == playerID {
		return nil, httputil.NewBadRequest("own_tower", "cannot capture own tower")
	}

	p, err := s.players.GetByID(ctx, playerID)
	if err != nil {
		return nil, httputil.NewInternal("failed to get player")
	}

	c, err := s.cells.GetByID(ctx, t.CellID)
	if err != nil {
		return nil, httputil.NewInternal("failed to get cell")
	}

	dist := geo.Haversine(p.Position.Lat, p.Position.Lng, c.Lat, c.Lng)
	if dist > 50 {
		return nil, httputil.NewBadRequest("too_far", "must be within 50m")
	}

	previousOwnerID := t.OwnerID

	// Transfer ownership and reset HP to max
	if err := s.towers.UpdateOwnership(ctx, t.ID, playerID, t.HPMax, t.HPMax); err != nil {
		return nil, httputil.NewInternal("failed to transfer tower")
	}

	t.OwnerID = playerID
	t.HP = t.HPMax

	// T-581: notify the previous owner. Best-effort — push failure must not
	// roll the capture back.
	if s.push != nil && previousOwnerID != "" {
		if err := s.push.NotifyTowerCaptured(ctx, previousOwnerID, p.Username); err != nil {
			slog.Warn("capture: notify tower captured failed",
				"tower_id", t.ID, "defender_id", previousOwnerID, "error", err)
		}
	}

	return t, nil
}

// ValidateForceCapture checks distance and ownership before delegating to battle service.
// Returns the tower so the handler can create a battle with target_type="tower".
func (s *Service) ValidateForceCapture(ctx context.Context, towerID, playerID string) (*tower.Tower, error) {
	if !canon.IsFeatureAvailable(canon.FeatureTowerPressure) {
		return nil, httputil.NewBadRequest("feature_locked", "tower pressure flow is not available in the current release")
	}

	t, err := s.towers.GetByID(ctx, towerID)
	if err != nil {
		return nil, httputil.NewNotFound("not_found", "tower not found")
	}

	if t.OwnerID == playerID {
		return nil, httputil.NewBadRequest("own_tower", "cannot attack own tower")
	}

	p, err := s.players.GetByID(ctx, playerID)
	if err != nil {
		return nil, httputil.NewInternal("failed to get player")
	}

	c, err := s.cells.GetByID(ctx, t.CellID)
	if err != nil {
		return nil, httputil.NewInternal("failed to get cell")
	}

	dist := geo.Haversine(p.Position.Lat, p.Position.Lng, c.Lat, c.Lng)
	if dist > 50 {
		return nil, httputil.NewBadRequest("too_far", "must be within 50m")
	}

	// T-581: pre-battle warning to the defender. Best-effort.
	if s.push != nil && t.OwnerID != "" {
		if err := s.push.NotifyTowerUnderAttack(ctx, t.OwnerID, p.Username); err != nil {
			slog.Warn("capture: notify tower under attack failed",
				"tower_id", t.ID, "defender_id", t.OwnerID, "error", err)
		}
	}

	return t, nil
}
