package survivor

import (
	"context"
	"testing"

	"github.com/ezra-game/server/internal/canon"
	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/internal/unit"
	"github.com/ezra-game/server/pkg/geo"
	"github.com/ezra-game/server/pkg/httputil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockUnitRepo struct{ mock.Mock }

func (m *mockUnitRepo) Create(ctx context.Context, u *unit.Unit) error {
	args := m.Called(ctx, u)
	if args.Error(0) == nil {
		u.ID = "unit-1"
	}
	return args.Error(0)
}
func (m *mockUnitRepo) GetByID(ctx context.Context, id string) (*unit.Unit, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*unit.Unit), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockUnitRepo) GetByPlayer(ctx context.Context, playerID string) ([]unit.Unit, error) {
	args := m.Called(ctx, playerID)
	if v := args.Get(0); v != nil {
		return v.([]unit.Unit), args.Error(1)
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
func (m *mockUnitRepo) UpdateProgress(ctx context.Context, u *unit.Unit) error {
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

func TestSpawnForPlayer_GuaranteesSurvivorsDuringOnboarding(t *testing.T) {
	svc := NewService(&mockUnitRepo{}, &mockPlayerRepo{})
	ctx := context.Background()
	playerRepo := svc.players.(*mockPlayerRepo)
	playerRepo.On("GetByID", ctx, "p1").Return(
		&player.Player{ID: "p1", OnboardingStep: canon.OnboardingSurvivorStep}, nil)

	lat, lng := 53.2, 50.18
	got := svc.SpawnForPlayer(ctx, "p1", lat, lng)

	assert.GreaterOrEqual(t, len(got), onboardingGuaranteedSurvivors,
		"onboarding must never present fewer than the guaranteed survivors")
	for _, s := range got {
		d := geo.Haversine(lat, lng, s.Lat, s.Lng)
		assert.LessOrEqual(t, d, 100.0, "survivor must be within recruit range")
	}
}

func TestSpawnForPlayer_NoGuaranteeOutsideOnboarding(t *testing.T) {
	svc := NewService(&mockUnitRepo{}, &mockPlayerRepo{})
	ctx := context.Background()
	playerRepo := svc.players.(*mockPlayerRepo)
	playerRepo.On("GetByID", ctx, "p1").Return(
		&player.Player{ID: "p1", OnboardingStep: canon.OnboardingCompleted}, nil)

	lat, lng := 53.2, 50.18
	got := svc.SpawnForPlayer(ctx, "p1", lat, lng)
	want := svc.SpawnNearby(lat, lng) // deterministic within the same 5-min window

	assert.Equal(t, len(want), len(got),
		"outside onboarding SpawnForPlayer must not add guaranteed survivors")
}

type fakeDomes struct{ cells []BuildingCell }

func (f *fakeDomes) DomedBuildingCells(_ context.Context, _ string, _, _, _ float64) ([]BuildingCell, error) {
	return f.cells, nil
}

func TestSpawnForPlayer_UsesDomedBuildingCells(t *testing.T) {
	domes := &fakeDomes{cells: []BuildingCell{{Lat: 53.20, Lng: 50.18}, {Lat: 53.21, Lng: 50.19}}}
	svc := NewService(&mockUnitRepo{}, &mockPlayerRepo{}).WithDomes(domes)
	ctx := context.Background()
	svc.players.(*mockPlayerRepo).On("GetByID", ctx, "p1").Return(
		&player.Player{ID: "p1", OnboardingStep: canon.OnboardingCompleted}, nil)

	got := svc.SpawnForPlayer(ctx, "p1", 53.205, 50.185)

	assert.Len(t, got, 2, "one survivor per domed building cell")
	for _, s := range got {
		atCell := (s.Lat == 53.20 && s.Lng == 50.18) || (s.Lat == 53.21 && s.Lng == 50.19)
		assert.True(t, atCell, "survivor must sit in a domed building cell, got %f,%f", s.Lat, s.Lng)
	}
}

func TestSpawnForPlayer_NoDomeNoSurvivors(t *testing.T) {
	svc := NewService(&mockUnitRepo{}, &mockPlayerRepo{}).WithDomes(&fakeDomes{cells: nil})
	ctx := context.Background()
	svc.players.(*mockPlayerRepo).On("GetByID", ctx, "p1").Return(
		&player.Player{ID: "p1", OnboardingStep: canon.OnboardingCompleted}, nil)

	got := svc.SpawnForPlayer(ctx, "p1", 53.2, 50.18)

	assert.Empty(t, got, "no dome → no survivors outside onboarding")
}

func TestRecruit_AdvancesOnboardingAfterSecondUnit(t *testing.T) {
	unitsRepo := &mockUnitRepo{}
	playerRepo := &mockPlayerRepo{}
	svc := NewService(unitsRepo, playerRepo)
	ctx := context.Background()

	p := &player.Player{
		ID:             "p1",
		ArmyLimit:      50,
		OnboardingStep: canon.OnboardingSurvivorStep,
		Position:       player.Position{Lat: 53.195, Lng: 50.100},
	}

	playerRepo.On("GetByID", ctx, "p1").Return(p, nil)
	unitsRepo.On("CountByPlayer", ctx, "p1").Return(1, nil)
	unitsRepo.On("Create", ctx, mock.AnythingOfType("*unit.Unit")).Return(nil)
	playerRepo.On("Update", ctx, p).Return(nil)

	result, err := svc.Recruit(ctx, "p1", "fighter", 53.1951, 50.1001)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, canon.OnboardingTutorialBattle, p.OnboardingStep)
	unitsRepo.AssertExpectations(t)
	playerRepo.AssertExpectations(t)
}

// fakeFaction is a stub FactionChecker (T-800).
type fakeFaction struct{ symbionts map[string]bool }

func (f fakeFaction) IsSymbiont(_ context.Context, playerID string) (bool, error) {
	return f.symbionts[playerID], nil
}

func TestRecruit_RejectsSymbiont(t *testing.T) {
	unitsRepo := &mockUnitRepo{}
	playerRepo := &mockPlayerRepo{}
	svc := NewService(unitsRepo, playerRepo).
		WithFaction(fakeFaction{symbionts: map[string]bool{"sym-1": true}})
	ctx := context.Background()

	_, err := svc.Recruit(ctx, "sym-1", "fighter", 53.1951, 50.1001)

	appErr, ok := err.(*httputil.AppError)
	if !ok {
		t.Fatalf("expected *httputil.AppError, got %T: %v", err, err)
	}
	assert.Equal(t, "symbiont_no_human_toolkit", appErr.Code)
	unitsRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}
