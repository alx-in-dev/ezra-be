package unit

import (
	"context"
	"testing"
	"time"

	"github.com/ezra-game/server/internal/player"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockUnitRepo struct{ mock.Mock }

func (m *mockUnitRepo) Create(ctx context.Context, u *Unit) error { return m.Called(ctx, u).Error(0) }
func (m *mockUnitRepo) GetByID(ctx context.Context, id string) (*Unit, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*Unit), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockUnitRepo) GetByPlayer(ctx context.Context, playerID string) ([]Unit, error) {
	args := m.Called(ctx, playerID)
	if v := args.Get(0); v != nil {
		return v.([]Unit), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockUnitRepo) CountByPlayer(ctx context.Context, playerID string) (int, error) {
	args := m.Called(ctx, playerID)
	return args.Int(0), args.Error(1)
}
func (m *mockUnitRepo) UpdateStatus(ctx context.Context, id, status string) error {
	return m.Called(ctx, id, status).Error(0)
}
func (m *mockUnitRepo) UpdateProgress(ctx context.Context, u *Unit) error {
	return m.Called(ctx, u).Error(0)
}
func (m *mockUnitRepo) AssignSquad(ctx context.Context, unitID string, squadID *string) error {
	return m.Called(ctx, unitID, squadID).Error(0)
}
func (m *mockUnitRepo) DeleteLostByPlayer(ctx context.Context, playerID string, count int) (int, error) {
	args := m.Called(ctx, playerID, count)
	return args.Int(0), args.Error(1)
}

type mockPlayerRepo struct{ mock.Mock }

func (m *mockPlayerRepo) Create(ctx context.Context, p *player.Player) error {
	return m.Called(ctx, p).Error(0)
}
func (m *mockPlayerRepo) GetByID(ctx context.Context, id string) (*player.Player, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*player.Player), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockPlayerRepo) GetAll(ctx context.Context) ([]player.Player, error) {
	args := m.Called(ctx)
	if v := args.Get(0); v != nil {
		return v.([]player.Player), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockPlayerRepo) GetByFirebaseUID(ctx context.Context, uid string) (*player.Player, error) {
	args := m.Called(ctx, uid)
	if v := args.Get(0); v != nil {
		return v.(*player.Player), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockPlayerRepo) GetByLogin(ctx context.Context, login string) (*player.Player, error) {
	args := m.Called(ctx, login)
	if v := args.Get(0); v != nil {
		return v.(*player.Player), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockPlayerRepo) CreateWithLogin(ctx context.Context, p *player.Player) error {
	return m.Called(ctx, p).Error(0)
}
func (m *mockPlayerRepo) Update(ctx context.Context, p *player.Player) error {
	return m.Called(ctx, p).Error(0)
}
func (m *mockPlayerRepo) UpdatePosition(ctx context.Context, id string, lat, lng float64) error {
	return m.Called(ctx, id, lat, lng).Error(0)
}
func (m *mockPlayerRepo) UpdateLastActive(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}
func (m *mockPlayerRepo) SetQuickStartHuman(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}

func TestListByPlayer_ReturnsDecaySummary(t *testing.T) {
	unitsRepo := &mockUnitRepo{}
	playerRepo := &mockPlayerRepo{}
	svc := NewService(unitsRepo, playerRepo)
	ctx := context.Background()

	unitsRepo.On("GetByPlayer", ctx, "p1").Return([]Unit{
		{ID: "u1", PlayerID: "p1", Type: "fighter", Status: "idle"},
		{ID: "u2", PlayerID: "p1", Type: "engineer", Status: "idle"},
	}, nil)
	playerRepo.On("GetByID", ctx, "p1").Return(&player.Player{
		ID:         "p1",
		ArmyLimit:  50,
		LastActive: time.Now().Add(-48 * time.Hour),
	}, nil)

	units, count, limit, retention, err := svc.ListByPlayer(ctx, "p1")

	assert.NoError(t, err)
	assert.Len(t, units, 2)
	assert.Equal(t, 2, count)
	assert.Equal(t, 50, limit)
	assert.NotNil(t, retention)
	assert.Equal(t, "decaying", retention.State)
	assert.Equal(t, 10, retention.DecayPercent)
	assert.True(t, retention.WelcomeBack)
}

func TestBuildRetentionSummary_FrozenAfterSevenDays(t *testing.T) {
	summary := buildRetentionSummary(time.Now().Add(-8 * 24 * time.Hour))

	assert.Equal(t, "frozen", summary.State)
	assert.Equal(t, 30, summary.DecayPercent)
	assert.True(t, summary.WelcomeBack)
}

func TestXPForLevel_Curve(t *testing.T) {
	// 20 * n^1.4, rounded. Level 1 has no cost (starting level).
	assert.Equal(t, 0, XPForLevel(1))
	assert.Equal(t, 53, XPForLevel(2))  // 20 * 2^1.4 = 52.78
	assert.Equal(t, 93, XPForLevel(3))  // 20 * 3^1.4 = 93.13
	assert.Equal(t, 139, XPForLevel(4)) // 20 * 4^1.4 = 139.25
}

func TestStatsForLevel_GrowsFromBase(t *testing.T) {
	hp1, atk1 := StatsForLevel("fighter", 1)
	assert.Equal(t, 50, hp1)
	assert.Equal(t, 10, atk1)
	// +15% of base per level, linear.
	hp5, atk5 := StatsForLevel("fighter", 5)
	assert.Equal(t, 80, hp5)  // 50 * (1 + 0.15*4) = 80
	assert.Equal(t, 16, atk5) // 10 * 1.6 = 16
}

func TestGainXP_LevelsUpAndRescales(t *testing.T) {
	u := &Unit{Type: "fighter", Level: 1, XP: 0, HP: 50, ATK: 10}
	// Enough to cross L1->L2 (53) with leftover.
	gained := u.GainXP(60)
	assert.Equal(t, 1, gained)
	assert.Equal(t, 2, u.Level)
	assert.Equal(t, 7, u.XP) // 60 - 53
	hp2, atk2 := StatsForLevel("fighter", 2)
	assert.Equal(t, hp2, u.HP)
	assert.Equal(t, atk2, u.ATK)
}

func TestGainXP_MultiLevelAndCap(t *testing.T) {
	u := &Unit{Type: "scout", Level: 1, XP: 0, HP: 35, ATK: 7}
	gained := u.GainXP(100000)
	assert.Equal(t, MaxLevel-1, gained)
	assert.Equal(t, MaxLevel, u.Level)
	assert.Equal(t, 0, u.XP) // zeroed at cap
	// No further XP once maxed.
	assert.Equal(t, 0, u.GainXP(500))
}
