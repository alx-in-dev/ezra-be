package pet

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ezra-game/server/internal/canon"
	"github.com/ezra-game/server/internal/cell"
	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/internal/tower"
	"github.com/ezra-game/server/pkg/httputil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockRepository struct{ mock.Mock }

func (m *MockRepository) Create(ctx context.Context, p *Pet) error { return m.Called(ctx, p).Error(0) }
func (m *MockRepository) GetByID(ctx context.Context, id string) (*Pet, error) {
	args := m.Called(ctx, id)
	if p, ok := args.Get(0).(*Pet); ok {
		return p, args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *MockRepository) GetByPlayer(ctx context.Context, playerID string) ([]Pet, error) {
	args := m.Called(ctx, playerID)
	if p, ok := args.Get(0).([]Pet); ok {
		return p, args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *MockRepository) ListActiveMissions(ctx context.Context) ([]Pet, error) {
	args := m.Called(ctx)
	if p, ok := args.Get(0).([]Pet); ok {
		return p, args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *MockRepository) Update(ctx context.Context, p *Pet) error { return m.Called(ctx, p).Error(0) }

type mockTowerRepo struct{ mock.Mock }

func (m *mockTowerRepo) Create(ctx context.Context, t *tower.Tower) error {
	return m.Called(ctx, t).Error(0)
}
func (m *mockTowerRepo) GetAll(ctx context.Context) ([]tower.Tower, error) {
	args := m.Called(ctx)
	if v := args.Get(0); v != nil {
		return v.([]tower.Tower), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockTowerRepo) GetByID(ctx context.Context, id string) (*tower.Tower, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*tower.Tower), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockTowerRepo) GetByIDs(ctx context.Context, ids []string) ([]tower.Tower, error) {
	args := m.Called(ctx, ids)
	if v := args.Get(0); v != nil {
		return v.([]tower.Tower), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockTowerRepo) GetByOwner(ctx context.Context, playerID string) ([]tower.Tower, error) {
	args := m.Called(ctx, playerID)
	if v := args.Get(0); v != nil {
		return v.([]tower.Tower), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockTowerRepo) GetInRadius(ctx context.Context, lat, lng, radiusM float64) ([]tower.Tower, error) {
	args := m.Called(ctx, lat, lng, radiusM)
	if v := args.Get(0); v != nil {
		return v.([]tower.Tower), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockTowerRepo) Update(ctx context.Context, t *tower.Tower) error {
	return m.Called(ctx, t).Error(0)
}
func (m *mockTowerRepo) UpdateOwnership(ctx context.Context, id, newOwnerID string, hp, hpMax int) error {
	return m.Called(ctx, id, newOwnerID, hp, hpMax).Error(0)
}
func (m *mockTowerRepo) Delete(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}
func (m *mockTowerRepo) CountInRadius(ctx context.Context, lat, lng, radiusM float64, ownerID string) (int, error) {
	args := m.Called(ctx, lat, lng, radiusM, ownerID)
	return args.Int(0), args.Error(1)
}
func (m *mockTowerRepo) CountAllInRadius(ctx context.Context, lat, lng, radiusM float64) (int, error) {
	args := m.Called(ctx, lat, lng, radiusM)
	return args.Int(0), args.Error(1)
}

type mockCellRepo struct{ mock.Mock }

func (m *mockCellRepo) GetAll(ctx context.Context) ([]cell.Cell, error) {
	args := m.Called(ctx)
	if v := args.Get(0); v != nil {
		return v.([]cell.Cell), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockCellRepo) GetInRadius(ctx context.Context, lat, lng, radiusM float64) ([]cell.Cell, error) {
	args := m.Called(ctx, lat, lng, radiusM)
	if v := args.Get(0); v != nil {
		return v.([]cell.Cell), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockCellRepo) GetByID(ctx context.Context, id string) (*cell.Cell, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*cell.Cell), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockCellRepo) GetNeighbors(ctx context.Context, cellID string) ([]cell.Cell, error) {
	args := m.Called(ctx, cellID)
	if v := args.Get(0); v != nil {
		return v.([]cell.Cell), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockCellRepo) Update(ctx context.Context, id string, infection float64) error {
	return m.Called(ctx, id, infection).Error(0)
}
func (m *mockCellRepo) SetTowerID(ctx context.Context, cellID string, towerID *string) error {
	return m.Called(ctx, cellID, towerID).Error(0)
}
func (m *mockCellRepo) StateSignature(ctx context.Context, lat, lng, radiusM float64) (string, error) {
	return "", nil
}

func (m *mockCellRepo) SetRiftID(ctx context.Context, cellID string, riftID *string) error {
	return m.Called(ctx, cellID, riftID).Error(0)
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

func newService(repo *MockRepository, towers *mockTowerRepo, cells *mockCellRepo, players *mockPlayerRepo) *Service {
	return NewService(repo, towers, cells, players, nil, nil, nil)
}

func testPlayer() *player.Player {
	return &player.Player{
		ID:        "player-1",
		ArmyLimit: 50,
		Position:  player.Position{Lat: 53.195, Lng: 50.100},
	}
}

func testTowerCell() *cell.Cell {
	return &cell.Cell{
		ID:        "cell-1",
		Lat:       53.195,
		Lng:       50.1001,
		Infection: 10,
		Terrain:   "building",
	}
}

func TestSend_SearchSuccess(t *testing.T) {
	repo := new(MockRepository)
	towers := &mockTowerRepo{}
	cells := &mockCellRepo{}
	players := &mockPlayerRepo{}
	svc := newService(repo, towers, cells, players)
	ctx := context.Background()

	pet := &Pet{ID: "pet-1", PlayerID: "player-1", Level: 2, Status: canon.PetStateIdle}
	repo.On("GetByID", ctx, "pet-1").Return(pet, nil)
	repo.On("Update", ctx, pet).Return(nil)

	result, err := svc.Send(ctx, "pet-1", "player-1", TaskTypeSearch)

	assert.NoError(t, err)
	assert.Equal(t, canon.PetStateMissionActive, result.Status)
	assert.Equal(t, TaskTypeSearch, result.Task.Type)
}

func TestClaimStarter_GrantsPetAndAdvancesToContact(t *testing.T) {
	repo := new(MockRepository)
	towers := &mockTowerRepo{}
	cells := &mockCellRepo{}
	players := &mockPlayerRepo{}
	svc := newService(repo, towers, cells, players)
	ctx := context.Background()

	playerState := &player.Player{ID: "player-1", OnboardingStep: canon.OnboardingPetUnlockStep}
	players.On("GetByID", ctx, "player-1").Return(playerState, nil)
	repo.On("GetByPlayer", ctx, "player-1").Return([]Pet{}, nil)
	repo.On("Create", ctx, mock.MatchedBy(func(p *Pet) bool {
		return p.PlayerID == "player-1" && p.Type == "scout" && p.Status == canon.PetStateIdle
	})).Return(nil)
	// Claiming the drone now hands off to the Contact bridge, not completion.
	players.On("Update", ctx, mock.MatchedBy(func(p *player.Player) bool {
		return p.OnboardingStep == canon.OnboardingContactStep
	})).Return(nil)

	petResult, updatedPlayer, err := svc.ClaimStarter(ctx, "player-1")

	assert.NoError(t, err)
	assert.NotNil(t, petResult)
	assert.Equal(t, "scout", petResult.Type)
	assert.Equal(t, canon.OnboardingContactStep, updatedPlayer.OnboardingStep)
}

func TestClaimStarter_IsIdempotentWhenPetExists(t *testing.T) {
	repo := new(MockRepository)
	towers := &mockTowerRepo{}
	cells := &mockCellRepo{}
	players := &mockPlayerRepo{}
	svc := newService(repo, towers, cells, players)
	ctx := context.Background()

	existingPet := Pet{ID: "pet-1", PlayerID: "player-1", Type: "scout", Status: canon.PetStateIdle}
	playerState := &player.Player{ID: "player-1", OnboardingStep: canon.OnboardingPetUnlockStep}
	players.On("GetByID", ctx, "player-1").Return(playerState, nil)
	repo.On("GetByPlayer", ctx, "player-1").Return([]Pet{existingPet}, nil)
	players.On("Update", ctx, mock.MatchedBy(func(p *player.Player) bool {
		return p.OnboardingStep == canon.OnboardingContactStep
	})).Return(nil)

	petResult, updatedPlayer, err := svc.ClaimStarter(ctx, "player-1")

	assert.NoError(t, err)
	assert.NotNil(t, petResult)
	assert.Equal(t, "pet-1", petResult.ID)
	assert.Equal(t, canon.OnboardingContactStep, updatedPlayer.OnboardingStep)
	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestSend_ScoutRequiresNearbySafeTower(t *testing.T) {
	repo := new(MockRepository)
	towers := &mockTowerRepo{}
	cells := &mockCellRepo{}
	players := &mockPlayerRepo{}
	svc := newService(repo, towers, cells, players)
	ctx := context.Background()

	pet := &Pet{ID: "pet-1", PlayerID: "player-1", Level: 1, Status: canon.PetStateIdle}
	tw := tower.Tower{ID: "tower-1", CellID: "cell-1", OwnerID: "player-1", RadiusM: 100}
	c := testTowerCell()

	repo.On("GetByID", ctx, "pet-1").Return(pet, nil)
	players.On("GetByID", ctx, "player-1").Return(testPlayer(), nil)
	towers.On("GetByOwner", ctx, "player-1").Return([]tower.Tower{tw}, nil)
	cells.On("GetByID", ctx, "cell-1").Return(c, nil)
	towers.On("CountAllInRadius", ctx, c.Lat, c.Lng, canon.TowerOverloadRadiusM).Return(1, nil)
	repo.On("Update", ctx, pet).Return(nil)

	result, err := svc.Send(ctx, "pet-1", "player-1", TaskTypeScout)

	assert.NoError(t, err)
	assert.Equal(t, TaskTypeScout, result.Task.Type)
	assert.Equal(t, "tower-1", result.Task.TargetTowerID)
}

func TestSend_ScoutFailsWithoutSafeTower(t *testing.T) {
	repo := new(MockRepository)
	towers := &mockTowerRepo{}
	cells := &mockCellRepo{}
	players := &mockPlayerRepo{}
	svc := newService(repo, towers, cells, players)
	ctx := context.Background()

	pet := &Pet{ID: "pet-1", PlayerID: "player-1", Level: 1, Status: canon.PetStateIdle}
	tw := tower.Tower{ID: "tower-1", CellID: "cell-1", OwnerID: "player-1", RadiusM: 100}
	c := testTowerCell()
	c.Infection = 45

	repo.On("GetByID", ctx, "pet-1").Return(pet, nil)
	players.On("GetByID", ctx, "player-1").Return(testPlayer(), nil)
	towers.On("GetByOwner", ctx, "player-1").Return([]tower.Tower{tw}, nil)
	cells.On("GetByID", ctx, "cell-1").Return(c, nil)

	_, err := svc.Send(ctx, "pet-1", "player-1", TaskTypeScout)

	assertAppErrorCode(t, err, "no_safe_tower")
	repo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
}

func TestSend_AlreadyOnTask(t *testing.T) {
	repo := new(MockRepository)
	svc := newService(repo, &mockTowerRepo{}, &mockCellRepo{}, &mockPlayerRepo{})
	ctx := context.Background()

	pet := &Pet{ID: "pet-1", PlayerID: "player-1", Status: canon.PetStateMissionActive, Level: 1}
	repo.On("GetByID", ctx, "pet-1").Return(pet, nil)

	_, err := svc.Send(ctx, "pet-1", "player-1", TaskTypeSearch)

	assert.Error(t, err)
	var appErr *httputil.AppError
	assert.True(t, errors.As(err, &appErr))
	assert.Equal(t, "not_idle", appErr.Code)
}

func TestRecall_SearchSuccess(t *testing.T) {
	repo := new(MockRepository)
	svc := newService(repo, &mockTowerRepo{}, &mockCellRepo{}, &mockPlayerRepo{})
	ctx := context.Background()

	pet := &Pet{
		ID:       "pet-1",
		PlayerID: "player-1",
		Level:    3,
		Status:   canon.PetStateMissionActive,
		Task: &PetTask{
			Type:      TaskTypeSearch,
			StartedAt: time.Now().Add(-1 * time.Hour),
			ReturnsAt: time.Now().Add(-10 * time.Minute),
			LootRoll:  0.01,
		},
	}

	repo.On("GetByID", ctx, "pet-1").Return(pet, nil)
	repo.On("Update", ctx, pet).Return(nil)

	result, loot, err := svc.Recall(ctx, "pet-1", "player-1")

	assert.NoError(t, err)
	assert.Equal(t, canon.PetStateIdle, result.Status)
	assert.Nil(t, result.Task)
	assert.NotNil(t, loot)
	assert.True(t, loot.Rare)
	assert.Nil(t, loot.Scout)
}

func TestRecall_ScoutReturnsBuildingSurvivors(t *testing.T) {
	repo := new(MockRepository)
	towers := &mockTowerRepo{}
	cells := &mockCellRepo{}
	svc := newService(repo, towers, cells, &mockPlayerRepo{})
	ctx := context.Background()

	pet := &Pet{
		ID:       "pet-1",
		PlayerID: "player-1",
		Level:    1,
		Status:   canon.PetStateMissionActive,
		Task: &PetTask{
			Type:          TaskTypeScout,
			TargetTowerID: "tower-1",
			StartedAt:     time.Now().Add(-3 * time.Hour),
			ReturnsAt:     time.Now().Add(-5 * time.Minute),
			LootRoll:      0.5,
		},
	}
	tw := &tower.Tower{ID: "tower-1", CellID: "tower-cell", RadiusM: 150}
	towerCell := &cell.Cell{ID: "tower-cell", Lat: 53.195, Lng: 50.100, Infection: 10, Terrain: "building"}
	buildings := []cell.Cell{
		{ID: "b1", Lat: 53.1952, Lng: 50.1001, Terrain: "building"},
		{ID: "b2", Lat: 53.1953, Lng: 50.1002, Terrain: "building"},
		{ID: "r1", Lat: 53.1954, Lng: 50.1003, Terrain: "road"},
	}

	repo.On("GetByID", ctx, "pet-1").Return(pet, nil)
	towers.On("GetByID", ctx, "tower-1").Return(tw, nil)
	cells.On("GetByID", ctx, "tower-cell").Return(towerCell, nil)
	cells.On("GetInRadius", ctx, towerCell.Lat, towerCell.Lng, tw.RadiusM).Return(buildings, nil)
	repo.On("Update", ctx, pet).Return(nil)

	result, loot, err := svc.Recall(ctx, "pet-1", "player-1")

	assert.NoError(t, err)
	assert.Equal(t, canon.PetStateIdle, result.Status)
	if assert.NotNil(t, loot.Scout) {
		assert.Equal(t, "tower-1", loot.Scout.TowerID)
		assert.NotEmpty(t, loot.Scout.Status)
		for _, found := range loot.Scout.Survivors {
			assert.Contains(t, []string{"fighter", "engineer"}, found.Type)
		}
	}
}

func TestRecall_EarlyReturnsNoReward(t *testing.T) {
	repo := new(MockRepository)
	svc := newService(repo, &mockTowerRepo{}, &mockCellRepo{}, &mockPlayerRepo{})
	ctx := context.Background()

	pet := &Pet{
		ID:       "pet-1",
		PlayerID: "player-1",
		Status:   canon.PetStateMissionActive,
		Level:    1,
		Task: &PetTask{
			Type:      TaskTypeSearch,
			StartedAt: time.Now(),
			ReturnsAt: time.Now().Add(30 * time.Minute),
			LootRoll:  0.5,
		},
	}

	repo.On("GetByID", ctx, "pet-1").Return(pet, nil)
	repo.On("Update", ctx, pet).Return(nil)

	// Recalling before the timer elapses brings the pet home with no reward.
	result, loot, err := svc.Recall(ctx, "pet-1", "player-1")

	assert.NoError(t, err)
	assert.Equal(t, canon.PetStateIdle, result.Status)
	assert.Nil(t, result.Task)
	assert.Equal(t, 0, loot.Energy)
	assert.Equal(t, 0, loot.Materials)
	assert.Nil(t, loot.Scout)
}

func TestAutoClaimReturned_SettlesElapsedMissions(t *testing.T) {
	repo := new(MockRepository)
	svc := newService(repo, &mockTowerRepo{}, &mockCellRepo{}, &mockPlayerRepo{})
	ctx := context.Background()

	done := Pet{
		ID: "done", PlayerID: "p1", Level: 1, Status: canon.PetStateMissionActive,
		Task: &PetTask{Type: TaskTypeSearch, ReturnsAt: time.Now().Add(-1 * time.Minute), LootRoll: 0.9},
	}
	pending := Pet{
		ID: "pending", PlayerID: "p1", Level: 1, Status: canon.PetStateMissionActive,
		Task: &PetTask{Type: TaskTypeSearch, ReturnsAt: time.Now().Add(20 * time.Minute), LootRoll: 0.9},
	}

	repo.On("ListActiveMissions", ctx).Return([]Pet{done, pending}, nil)
	repo.On("Update", ctx, mock.MatchedBy(func(p *Pet) bool { return p.Status == canon.PetStateIdle })).Return(nil).Once()

	claimed, err := svc.AutoClaimReturned(ctx)

	assert.NoError(t, err)
	assert.Equal(t, 1, claimed)
	repo.AssertExpectations(t)
}

func TestRecall_NotOnTask(t *testing.T) {
	repo := new(MockRepository)
	svc := newService(repo, &mockTowerRepo{}, &mockCellRepo{}, &mockPlayerRepo{})
	ctx := context.Background()

	pet := &Pet{ID: "pet-1", PlayerID: "player-1", Status: canon.PetStateIdle, Level: 1}
	repo.On("GetByID", ctx, "pet-1").Return(pet, nil)

	_, _, err := svc.Recall(ctx, "pet-1", "player-1")

	assertAppErrorCode(t, err, "not_on_task")
}

func assertAppErrorCode(t *testing.T, err error, expectedCode string) {
	t.Helper()
	var appErr *httputil.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *httputil.AppError, got %T: %v", err, err)
	}
	assert.Equal(t, expectedCode, appErr.Code)
}
