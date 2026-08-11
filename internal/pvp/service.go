package pvp

import (
	"context"
	"log/slog"

	"github.com/ezra-game/server/internal/factionwar"
	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/internal/push"
	"github.com/ezra-game/server/internal/rift"
	"github.com/ezra-game/server/pkg/geo"
	"github.com/ezra-game/server/pkg/httputil"
)

type FactionChecker interface {
	IsSymbiont(ctx context.Context, playerID string) (bool, error)
}

type RiftForcer interface {
	ForceSpawn(ctx context.Context, cellID, riftType string) (*rift.Rift, error)
}

type ResourceSpender interface {
	Spend(ctx context.Context, playerID string, energy, materials int) (*player.Player, error)
}

type Notifier interface {
	Send(ctx context.Context, msg push.PushMessage) error
}

type PlayerProvider interface {
	GetByID(ctx context.Context, id string) (*player.Player, error)
}

// EventPublisher pushes a realtime event to a player's SSE stream (R4-6).
// Implemented by realtime.Hub.PublishEvent.
type EventPublisher interface {
	PublishEvent(playerID, eventType string, data map[string]any)
}

// WarScorer credits Symbiont faction-war points (factionwar.Service).
type WarScorer interface {
	AwardSymbiont(ctx context.Context, playerID string, points int) error
}

// Service implements the v1.1 Symbiont→Human PvP: Dome Breach.
type Service struct {
	repo      Repository
	faction   FactionChecker
	rifts     RiftForcer
	resources ResourceSpender
	push      Notifier
	players   PlayerProvider
	events    EventPublisher
	war       WarScorer
}

func NewService(repo Repository, faction FactionChecker, rifts RiftForcer, resources ResourceSpender, push Notifier, players PlayerProvider) *Service {
	return &Service{repo: repo, faction: faction, rifts: rifts, resources: resources, push: push, players: players}
}

// WithEvents wires the realtime publisher so a breach reaches the defender live
// (R4-6), not just via FCM / next map refetch. Optional.
func (s *Service) WithEvents(p EventPublisher) *Service { s.events = p; return s }

// WithWar wires season scoring for a landed breach (canon faction_war.md:
// Dome Breach +45). Optional.
func (s *Service) WithWar(w WarScorer) *Service { s.war = w; return s }

// Targets lists enemy domes a Symbiont can breach near a point.
func (s *Service) Targets(ctx context.Context, attackerID string, lat, lng float64) ([]Target, error) {
	sym, err := s.faction.IsSymbiont(ctx, attackerID)
	if err != nil {
		return nil, err
	}
	if !sym {
		return nil, httputil.NewForbidden("not_symbiont", "прокол купола доступен только Симбионтам")
	}
	targets, err := s.repo.NearbyEnemyDomes(ctx, attackerID, lat, lng, TargetRadiusM)
	if err != nil {
		return nil, err
	}
	if targets == nil {
		targets = []Target{}
	}
	return targets, nil
}

// Breach punctures an enemy dome: opens a rift under it and alerts the defender.
func (s *Service) Breach(ctx context.Context, attackerID, cellID string) (*BreachResult, error) {
	sym, err := s.faction.IsSymbiont(ctx, attackerID)
	if err != nil {
		return nil, err
	}
	if !sym {
		return nil, httputil.NewForbidden("not_symbiont", "прокол купола доступен только Симбионтам")
	}
	defenderID, ok, err := s.repo.DefenderOf(ctx, cellID, attackerID)
	if err != nil {
		return nil, err
	}
	if !ok {
		// Cell is undomed or only the attacker domes it — nothing enemy to breach.
		return nil, httputil.NewBadRequest("not_domed", "клетка не под чужим куполом")
	}
	// Range gate: Breach took a raw cellID with no distance check, so any dome
	// on the planet was breachable from the couch. The target must be within
	// scouting range of the attacker's server-side position.
	if coords, ok := s.repo.(interface {
		CellCoords(ctx context.Context, cellID string) (float64, float64, error)
	}); ok && s.players != nil {
		if att, aErr := s.players.GetByID(ctx, attackerID); aErr == nil && att != nil &&
			!(att.Position.Lat == 0 && att.Position.Lng == 0) {
			if cLat, cLng, cErr := coords.CellCoords(ctx, cellID); cErr == nil {
				if geo.Haversine(att.Position.Lat, att.Position.Lng, cLat, cLng) > TargetRadiusM {
					return nil, httputil.NewBadRequest("too_far", "цель вне зоны досягаемости — подойдите ближе")
				}
			}
		}
	}

	if s.resources != nil {
		if _, err := s.resources.Spend(ctx, attackerID, BreachEnergyCost, 0); err != nil {
			return nil, err
		}
	}

	r, err := s.rifts.ForceSpawn(ctx, cellID, BreachRiftType)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, httputil.NewBadRequest("already_breached", "под этим куполом уже есть разлом")
	}
	if err := s.repo.RaiseInfection(ctx, cellID, BreachInfection); err != nil {
		slog.Error("breach raise infection failed", "cell", cellID, "error", err)
	}

	defenderName := "Защитник"
	attackerName := "Симбионт"
	if s.players != nil {
		if d, e := s.players.GetByID(ctx, defenderID); e == nil && d != nil {
			defenderName = d.Username
		}
		if a, e := s.players.GetByID(ctx, attackerID); e == nil && a != nil && a.Username != "" {
			attackerName = a.Username
		}
	}
	if s.push != nil {
		_ = s.push.Send(ctx, push.PushMessage{
			PlayerID: defenderID,
			Title:    "⚠ Купол пробит!",
			Body:     attackerName + " открыл разлом под вашим куполом. Закройте его, пока заражение не разлилось!",
			Data:     map[string]string{"type": "dome_breach", "cell_id": cellID},
		})
	}
	// R4-6: reach the defender live (SSE) so the breach shows without a refetch.
	if s.events != nil {
		s.events.PublishEvent(defenderID, "dome_breached", map[string]any{
			"cell_id":  cellID,
			"attacker": attackerName,
		})
	}

	// #6 baseless progression: a successful breach feeds the Symbiont's portable
	// Resonance. Best-effort — the breach already succeeded.
	if rg, ok := s.players.(resonanceGranter); ok {
		if _, err := rg.AddSymbiontResonance(ctx, attackerID, ResonancePerBreach); err != nil {
			slog.Error("breach: grant resonance failed", "player_id", attackerID, "error", err)
		}
	}
	// E2: a landed breach is a Symbiont war feat (canon +45). Best-effort.
	if s.war != nil {
		_ = s.war.AwardSymbiont(ctx, attackerID, factionwar.PointsDomeBreach)
	}

	return &BreachResult{CellID: cellID, RiftID: r.ID, DefenderName: defenderName}, nil
}

// resonanceGranter is optionally implemented by the player provider so a breach
// can credit portable Symbiont Resonance (#6) without new wiring.
type resonanceGranter interface {
	AddSymbiontResonance(ctx context.Context, playerID string, delta int) (int, error)
}
