package factionwar

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPointsForRiftClose(t *testing.T) {
	assert.Equal(t, 25, PointsForRiftClose("minor"))
	assert.Equal(t, 60, PointsForRiftClose("medium"))
	assert.Equal(t, 140, PointsForRiftClose("critical"))
	assert.Equal(t, 0, PointsForRiftClose("unknown"))
}

func TestSeasonFor(t *testing.T) {
	// 2026-06-15 is in ISO week 25.
	s := SeasonFor(time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC))
	assert.Equal(t, "2026-W25", s)
}

type fakeRepo struct {
	added     int
	balance   RegionBalance
	settled   bool
	rewards   [][2]int // {rank, crystals}
	unsettled []string
}

func (f *fakeRepo) AddPoints(ctx context.Context, p, s string, d int) (int, error) {
	f.added += d
	return f.added, nil
}
func (f *fakeRepo) GetPoints(ctx context.Context, p, s string) (int, error) { return f.added, nil }
func (f *fakeRepo) TopBySeason(ctx context.Context, s string, n int) ([]Standing, error) {
	return []Standing{{PlayerID: "p1", Username: "cactus", Points: f.added}}, nil
}
func (f *fakeRepo) RegionBalance(ctx context.Context, lat, lng, r float64) (RegionBalance, error) {
	return f.balance, nil
}
func (f *fakeRepo) IsSettled(ctx context.Context, season string) (bool, error) { return f.settled, nil }
func (f *fakeRepo) MarkSettled(ctx context.Context, season string) error       { f.settled = true; return nil }
func (f *fakeRepo) RecordReward(ctx context.Context, p, s string, rank, pts, cry int) error {
	f.rewards = append(f.rewards, [2]int{rank, cry})
	return nil
}
func (f *fakeRepo) UnsettledSeasons(ctx context.Context, except string) ([]string, error) {
	return f.unsettled, nil
}
func (f *fakeRepo) LastReward(ctx context.Context, p string) (*SeasonReward, error) { return nil, nil }

type fakeCrystals struct{ total int }

func (f *fakeCrystals) IncrementCrystals(ctx context.Context, id string, d int) error {
	f.total += d
	return nil
}

type fakeTitles struct{ grants int }

func (f *fakeTitles) Grant(ctx context.Context, p, t, v string, d int) (int, error) {
	f.grants++
	return 1, nil
}

func TestAwardHumanAndStatus(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{balance: RegionBalance{HumanRaw: 30, SymbiontRaw: 10, HumanPct: 75, SymbiontPct: 25}}
	svc := NewService(repo)
	svc.now = func() time.Time { return time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC) }

	assert.NoError(t, svc.AwardHuman(ctx, "p1", 140))
	assert.NoError(t, svc.AwardHuman(ctx, "p1", 0)) // no-op
	assert.Equal(t, 140, repo.added)

	st, err := svc.Status(ctx, "p1", 54.7, 20.5)
	assert.NoError(t, err)
	assert.Equal(t, "2026-W25", st.Season)
	assert.Equal(t, 75, st.Balance.HumanPct)
	assert.Equal(t, 140, st.YourContribution)
	assert.Len(t, st.Leaderboard, 1)
}

func TestCrystalsForRank(t *testing.T) {
	assert.Equal(t, 500, CrystalsForRank(1))
	assert.Equal(t, 300, CrystalsForRank(2))
	assert.Equal(t, 150, CrystalsForRank(3))
	assert.Equal(t, 50, CrystalsForRank(9))
}

func TestSettleSeason_RewardsAndIdempotent(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{added: 60} // TopBySeason returns one player with 60 pts
	cr := &fakeCrystals{}
	ti := &fakeTitles{}
	svc := NewService(repo)
	svc.now = func() time.Time { return time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC) } // current = 2026-W25
	svc.SetRewarders(cr, ti)

	// Settling the CURRENT season is refused.
	assert.Error(t, svc.SettleSeason(ctx, "2026-W25"))

	// Settle a past season → rank-1 player gets 500 crystals + a title.
	assert.NoError(t, svc.SettleSeason(ctx, "2026-W24"))
	assert.Equal(t, 500, cr.total)
	assert.Equal(t, 1, ti.grants)
	assert.True(t, repo.settled)
	assert.Len(t, repo.rewards, 1)
	assert.Equal(t, [2]int{1, 500}, repo.rewards[0])

	// Idempotent: already settled → no double payout.
	repo.settled = true
	cr.total = 0
	assert.NoError(t, svc.SettleSeason(ctx, "2026-W24"))
	assert.Equal(t, 0, cr.total)
}
