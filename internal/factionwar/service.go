package factionwar

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ezra-game/server/internal/item"
	"github.com/ezra-game/server/pkg/httputil"
)

// CrystalGranter credits season-reward crystals (player.Repository).
type CrystalGranter interface {
	IncrementCrystals(ctx context.Context, id string, delta int) error
}

// TitleGranter grants the season-title item (item.Service).
type TitleGranter interface {
	Grant(ctx context.Context, playerID, itemType, variant string, delta int) (int, error)
}

// FactionReader resolves a player's chosen side (faction.Repository.Get).
// Optional: when wired, awards are side-gated so a Symbiont closing rifts
// can't farm Human war points (and vice versa).
type FactionReader interface {
	Get(ctx context.Context, playerID string) (faction string, contacted, chosen bool, err error)
}

// RegionRadiusM is how far around the player the faction balance is measured.
const RegionRadiusM = 1500

// Clock lets tests inject time; production uses time.Now.
type Clock func() time.Time

type Service struct {
	repo     Repository
	now      Clock
	crystals CrystalGranter
	titles   TitleGranter
	factions FactionReader
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

// SetRewarders wires the optional season-reward dependencies (crystals + title).
func (s *Service) SetRewarders(c CrystalGranter, t TitleGranter) {
	s.crystals = c
	s.titles = t
}

// SetFactionReader wires side-gating of war-point awards. Optional.
func (s *Service) SetFactionReader(f FactionReader) { s.factions = f }

// CurrentSeason is the weekly season key.
func (s *Service) CurrentSeason() string {
	return SeasonFor(s.now())
}

// sideOf resolves the player's faction, defaulting to human when no reader is
// wired (legacy behaviour in tests). Faction literals match faction/model.go.
func (s *Service) sideOf(ctx context.Context, playerID string) string {
	if s.factions == nil {
		return "human"
	}
	fac, _, _, err := s.factions.Get(ctx, playerID)
	if err != nil || fac == "" {
		return "human"
	}
	return fac
}

// AwardHuman credits a player's Human contribution in the current season.
// No-op for Symbionts: their rift/hive victories are their own business, not
// war feats for the side they fight against.
func (s *Service) AwardHuman(ctx context.Context, playerID string, points int) error {
	if points <= 0 {
		return nil
	}
	if s.sideOf(ctx, playerID) != "human" {
		return nil
	}
	_, err := s.repo.AddPoints(ctx, playerID, s.CurrentSeason(), points)
	return err
}

// AwardSymbiont credits a player's Symbiont contribution (Overload, Dome
// Breach — canon faction_war.md scoring). No-op for humans.
func (s *Service) AwardSymbiont(ctx context.Context, playerID string, points int) error {
	if points <= 0 {
		return nil
	}
	if s.sideOf(ctx, playerID) != "symbiont" {
		return nil
	}
	_, err := s.repo.AddPoints(ctx, playerID, s.CurrentSeason(), points)
	return err
}

// Status assembles the faction-war view around a point.
func (s *Service) Status(ctx context.Context, playerID string, lat, lng float64) (*Status, error) {
	season := s.CurrentSeason()
	balance, err := s.repo.RegionBalance(ctx, lat, lng, RegionRadiusM)
	if err != nil {
		return nil, err
	}
	mine, err := s.repo.GetPoints(ctx, playerID, season)
	if err != nil {
		return nil, err
	}
	top, err := s.repo.TopBySeason(ctx, season, 10)
	if err != nil {
		return nil, err
	}
	if top == nil {
		top = []Standing{}
	}
	lastReward, err := s.repo.LastReward(ctx, playerID)
	if err != nil {
		return nil, err
	}
	return &Status{
		Season:           season,
		Balance:          balance,
		YourContribution: mine,
		Leaderboard:      top,
		LastReward:       lastReward,
	}, nil
}

// SettleSeason pays out a finished season's standings: top contributors get
// crystals (by rank) and a season title. Idempotent; refuses the current season.
func (s *Service) SettleSeason(ctx context.Context, season string) error {
	if season == s.CurrentSeason() {
		return httputil.NewBadRequest("season_active", "сезон ещё идёт")
	}
	settled, err := s.repo.IsSettled(ctx, season)
	if err != nil {
		return err
	}
	if settled {
		return nil
	}
	top, err := s.repo.TopBySeason(ctx, season, 100)
	if err != nil {
		return err
	}
	for i, st := range top {
		rank := i + 1
		crystals := CrystalsForRank(rank)
		if s.crystals != nil {
			if err := s.crystals.IncrementCrystals(ctx, st.PlayerID, crystals); err != nil {
				slog.Error("faction reward crystals failed", "player", st.PlayerID, "season", season, "error", err)
			}
		}
		if s.titles != nil {
			_, _ = s.titles.Grant(ctx, st.PlayerID, item.TypeSeasonTitle, season, 1)
		}
		if err := s.repo.RecordReward(ctx, st.PlayerID, season, rank, st.Points, crystals); err != nil {
			slog.Error("faction record reward failed", "player", st.PlayerID, "season", season, "error", err)
		}
	}
	if err := s.repo.MarkSettled(ctx, season); err != nil {
		return err
	}
	slog.Info("faction season settled", "season", season, "rewarded", len(top))
	return nil
}

// SettleDue settles every past season that still has unsettled scores.
func (s *Service) SettleDue(ctx context.Context) error {
	seasons, err := s.repo.UnsettledSeasons(ctx, s.CurrentSeason())
	if err != nil {
		return err
	}
	for _, season := range seasons {
		if err := s.SettleSeason(ctx, season); err != nil {
			return fmt.Errorf("settle %s: %w", season, err)
		}
	}
	return nil
}
