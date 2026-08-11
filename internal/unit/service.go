package unit

import (
	"context"
	"math"
	"time"

	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/pkg/httputil"
)

type RetentionSummary struct {
	State           string `json:"state"`
	InactiveHours   int    `json:"inactive_hours"`
	DecayPercent    int    `json:"decay_percent"`
	MaxDecayPercent int    `json:"max_decay_percent"`
	WelcomeBack     bool   `json:"welcome_back"`
	Message         string `json:"message"`
}

// Service handles unit business logic.
type Service struct {
	units   Repository
	players player.Repository
}

// NewService creates a new unit service.
func NewService(units Repository, players player.Repository) *Service {
	return &Service{units: units, players: players}
}

// Recruit creates a new unit for the player if they haven't reached their army limit.
func (s *Service) Recruit(ctx context.Context, playerID, unitType string) (*Unit, error) {
	stats, ok := BaseStats[unitType]
	if !ok {
		return nil, httputil.NewBadRequest("invalid_unit_type", "type must be fighter, engineer, scout, or medic")
	}

	p, err := s.players.GetByID(ctx, playerID)
	if err != nil {
		return nil, httputil.NewInternal("failed to get player")
	}

	count, err := s.units.CountByPlayer(ctx, playerID)
	if err != nil {
		return nil, httputil.NewInternal("failed to count units")
	}

	if count >= p.ArmyLimit {
		return nil, httputil.NewBadRequest("army_limit_reached", "you have reached your army limit")
	}

	u := &Unit{
		PlayerID: playerID,
		Type:     unitType,
		Level:    1,
		XP:       0,
		HP:       stats.HP,
		ATK:      stats.ATK,
		Status:   "idle",
	}

	if err := s.units.Create(ctx, u); err != nil {
		return nil, httputil.NewInternal("failed to create unit")
	}

	return u, nil
}

// ListByPlayer returns all active units for the player plus army count/limit.
func (s *Service) ListByPlayer(ctx context.Context, playerID string) ([]Unit, int, int, *RetentionSummary, error) {
	units, err := s.units.GetByPlayer(ctx, playerID)
	if err != nil {
		return nil, 0, 0, nil, err
	}

	for i := range units {
		if units[i].Level < MaxLevel {
			units[i].XPMax = XPForLevel(units[i].Level + 1)
		}
	}

	p, err := s.players.GetByID(ctx, playerID)
	if err != nil {
		return nil, 0, 0, nil, err
	}

	return units, len(units), p.ArmyLimit, buildRetentionSummary(p.LastActive), nil
}

// UnitLevelUp summarizes a unit that gained one or more levels in a battle.
type UnitLevelUp struct {
	UnitID string `json:"unit_id"`
	Type   string `json:"type"`
	Level  int    `json:"level"`
	Gained int    `json:"gained"`
}

// AwardSquadBattleXP grants xpEach to every surviving unit in the squad, applies
// level-ups (rescaling HP/ATK), and persists each. Returns the units that
// gained a level so the caller can surface it to the player.
func (s *Service) AwardSquadBattleXP(ctx context.Context, playerID, squadID string, xpEach int) ([]UnitLevelUp, error) {
	if xpEach <= 0 {
		return nil, nil
	}
	units, err := s.units.GetByPlayer(ctx, playerID)
	if err != nil {
		return nil, err
	}
	var ups []UnitLevelUp
	for i := range units {
		u := &units[i]
		if u.SquadID == nil || *u.SquadID != squadID {
			continue
		}
		gained := u.GainXP(xpEach)
		if err := s.units.UpdateProgress(ctx, u); err != nil {
			return nil, err
		}
		if gained > 0 {
			ups = append(ups, UnitLevelUp{UnitID: u.ID, Type: u.Type, Level: u.Level, Gained: gained})
		}
	}
	return ups, nil
}

func buildRetentionSummary(lastActive time.Time) *RetentionSummary {
	if lastActive.IsZero() {
		return &RetentionSummary{
			State:           "active",
			MaxDecayPercent: 30,
			Message:         "Армия стабильна.",
		}
	}

	inactive := int(time.Since(lastActive).Hours())
	summary := &RetentionSummary{
		InactiveHours:   inactive,
		MaxDecayPercent: 30,
	}

	switch {
	case inactive < 24:
		summary.State = "active"
		summary.Message = "Армия стабильна."
	case inactive >= 24*7:
		summary.State = "frozen"
		summary.DecayPercent = 30
		summary.WelcomeBack = true
		summary.Message = "Вы давно отсутствовали: decay заморожен на лимите 30%."
	default:
		summary.State = "decaying"
		summary.DecayPercent = int(math.Min(math.Floor(5.0*float64(inactive)/24.0), 30))
		summary.WelcomeBack = true
		summary.Message = "После долгого отсутствия армия теряет до 5% idle-юнитов за 24ч."
	}

	return summary
}
