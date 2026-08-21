package tower

import (
	"context"
	"testing"

	"github.com/ezra-game/server/internal/canon"
	"github.com/ezra-game/server/internal/cell"
	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/pkg/httputil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// ---- Mocks ----

type mockTowerRepo struct{ mock.Mock }

func (m *mockTowerRepo) Create(ctx context.Context, t *Tower) error {
	args := m.Called(ctx, t)
	if args.Error(0) == nil {
		t.ID = "tower-1"
	}
	return args.Error(0)
}
func (m *mockTowerRepo) GetAll(ctx context.Context) ([]Tower, error) {
	args := m.Called(ctx)
	if v := args.Get(0); v != nil {
		return v.([]Tower), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockTowerRepo) GetByID(ctx context.Context, id string) (*Tower, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*Tower), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockTowerRepo) GetByIDs(ctx context.Context, ids []string) ([]Tower, error) {
	args := m.Called(ctx, ids)
	if v := args.Get(0); v != nil {
		return v.([]Tower), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockTowerRepo) GetByOwner(ctx context.Context, playerID string) ([]Tower, error) {
	args := m.Called(ctx, playerID)
	return args.Get(0).([]Tower), args.Error(1)
}
func (m *mockTowerRepo) GetInRadius(ctx context.Context, lat, lng, radiusM float64) ([]Tower, error) {
	args := m.Called(ctx, lat, lng, radiusM)
	return args.Get(0).([]Tower), args.Error(1)
}
func (m *mockTowerRepo) Update(ctx context.Context, t *Tower) error {
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
	return args.Get(0).([]cell.Cell), args.Error(1)
}
func (m *mockCellRepo) GetInRadius(ctx context.Context, lat, lng, radiusM float64) ([]cell.Cell, error) {
	args := m.Called(ctx, lat, lng, radiusM)
	return args.Get(0).([]cell.Cell), args.Error(1)
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
	return args.Get(0).([]cell.Cell), args.Error(1)
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

type mockQuestProgressor struct{ mock.Mock }

func (m *mockQuestProgressor) UpdateProgress(ctx context.Context, playerID, eventType string) error {
	return m.Called(ctx, playerID, eventType).Error(0)
}

type mockPushNotifier struct{ mock.Mock }

func (m *mockPushNotifier) NotifyTowerPlaced(ctx context.Context, playerID string) error {
	return m.Called(ctx, playerID).Error(0)
}

func (m *mockPushNotifier) NotifyTowerUnderAttack(ctx context.Context, defenderID, attackerName string) error {
	return m.Called(ctx, defenderID, attackerName).Error(0)
}

// ---- Helpers ----

func setupService() (*Service, *mockTowerRepo, *mockCellRepo, *mockPlayerRepo, *mockQuestProgressor, *mockPushNotifier) {
	towerRepo := &mockTowerRepo{}
	cellRepo := &mockCellRepo{}
	playerRepo := &mockPlayerRepo{}
	questProgressor := &mockQuestProgressor{}
	pushNotifier := &mockPushNotifier{}
	questProgressor.On("UpdateProgress", mock.Anything, mock.Anything, mock.Anything).Maybe().Return(nil)
	pushNotifier.On("NotifyTowerPlaced", mock.Anything, mock.Anything).Maybe().Return(nil)
	pushNotifier.On("NotifyTowerUnderAttack", mock.Anything, mock.Anything, mock.Anything).Maybe().Return(nil)
	resourceSvc := player.NewResourceService(playerRepo)
	svc := NewService(towerRepo, cellRepo, playerRepo, resourceSvc, questProgressor, pushNotifier)
	return svc, towerRepo, cellRepo, playerRepo, questProgressor, pushNotifier
}

func defaultPlayer() *player.Player {
	return &player.Player{
		ID:        "player-1",
		Energy:    500,
		Materials: 100,
		Position:  player.Position{Lat: 53.195, Lng: 50.100},
	}
}

func defaultCell() *cell.Cell {
	return &cell.Cell{
		ID:                "cell-1",
		Lat:               53.195,
		Lng:               50.1001,
		Terrain:           "building",
		TowerID:           nil,
		HasNearbyBuilding: true, // valid power source for tower placement (decision 8.2)
	}
}

func assertAppErrorCode(t *testing.T, err error, expectedCode string) {
	t.Helper()
	appErr, ok := err.(*httputil.AppError)
	if !ok {
		t.Fatalf("expected *httputil.AppError, got %T: %v", err, err)
	}
	assert.Equal(t, expectedCode, appErr.Code)
}

// ---- Place Tests ----

func TestPlace_Success(t *testing.T) {
	svc, towerRepo, cellRepo, playerRepo, questProgressor, pushNotifier := setupService()
	ctx := context.Background()
	p := defaultPlayer()
	c := defaultCell()

	playerRepo.On("GetByID", ctx, "player-1").Return(p, nil)
	cellRepo.On("GetByID", ctx, "cell-1").Return(c, nil)
	towerRepo.On("CountAllInRadius", ctx, c.Lat, c.Lng, canon.NetworkMinSpacingM).Return(0, nil)
	towerRepo.On("CountAllInRadius", ctx, c.Lat, c.Lng, canon.TowerOverloadRadiusM).Return(2, nil)
	// ResourceService.Spend calls GetByID then Update
	playerRepo.On("GetByID", ctx, "player-1").Return(p, nil)
	playerRepo.On("Update", ctx, mock.AnythingOfType("*player.Player")).Return(nil)
	towerRepo.On("Create", ctx, mock.AnythingOfType("*tower.Tower")).Return(nil)
	cellRepo.On("SetTowerID", ctx, "cell-1", mock.AnythingOfType("*string")).Return(nil)
	questProgressor.On("UpdateProgress", ctx, "player-1", "place_beacon").Return(nil)
	pushNotifier.On("NotifyTowerPlaced", ctx, "player-1").Return(nil)

	tower, err := svc.Place(ctx, "player-1", "cell-1")

	assert.NoError(t, err)
	assert.NotNil(t, tower)
	assert.Equal(t, 1, tower.Level)
	assert.Equal(t, "player-1", tower.OwnerID)
	assert.Equal(t, "cell-1", tower.CellID)
	assert.Equal(t, canon.TowerTypeStandard, tower.Type)
	assert.Equal(t, 100, tower.HPMax)
	assert.Equal(t, 100.0, tower.RadiusM)
	towerRepo.AssertExpectations(t)
	cellRepo.AssertExpectations(t)
}

func TestPlace_UsesStarterBeaconWithoutResourceSpend(t *testing.T) {
	svc, towerRepo, cellRepo, playerRepo, questProgressor, pushNotifier := setupService()
	ctx := context.Background()
	p := defaultPlayer()
	p.OnboardingStep = canon.OnboardingFirstTowerStep
	p.StarterBeaconAvailable = true
	c := defaultCell()

	playerRepo.On("GetByID", ctx, "player-1").Return(p, nil)
	cellRepo.On("GetByID", ctx, "cell-1").Return(c, nil)
	towerRepo.On("CountAllInRadius", ctx, c.Lat, c.Lng, canon.NetworkMinSpacingM).Return(0, nil)
	towerRepo.On("CountAllInRadius", ctx, c.Lat, c.Lng, canon.TowerOverloadRadiusM).Return(1, nil)
	towerRepo.On("Create", ctx, mock.AnythingOfType("*tower.Tower")).Return(nil)
	cellRepo.On("SetTowerID", ctx, "cell-1", mock.AnythingOfType("*string")).Return(nil)
	playerRepo.On("Update", ctx, mock.AnythingOfType("*player.Player")).Return(nil)
	questProgressor.On("UpdateProgress", ctx, "player-1", "place_beacon").Return(nil)
	pushNotifier.On("NotifyTowerPlaced", ctx, "player-1").Return(nil)

	tower, err := svc.Place(ctx, "player-1", "cell-1")

	assert.NoError(t, err)
	assert.NotNil(t, tower)
	assert.False(t, p.StarterBeaconAvailable)
	assert.Equal(t, canon.OnboardingSurvivorStep, p.OnboardingStep)
}

func TestPlace_TooFar(t *testing.T) {
	svc, _, cellRepo, playerRepo, _, _ := setupService()
	ctx := context.Background()

	p := defaultPlayer()
	// Cell ~50m away (different lng)
	c := &cell.Cell{
		ID:      "cell-1",
		Lat:     53.195,
		Lng:     50.1007, // ~50m away at this latitude
		Terrain: "building",
	}

	playerRepo.On("GetByID", ctx, "player-1").Return(p, nil)
	cellRepo.On("GetByID", ctx, "cell-1").Return(c, nil)

	_, err := svc.Place(ctx, "player-1", "cell-1")

	assert.Error(t, err)
	assertAppErrorCode(t, err, "too_far")
}

func TestPlace_NoPowerSource(t *testing.T) {
	svc, _, cellRepo, playerRepo, _, _ := setupService()
	ctx := context.Background()

	p := defaultPlayer()
	c := defaultCell()
	// Cell has no nearby building and no nearby road → no power source.
	c.HasNearbyBuilding = false
	c.HasNearbyRoad = false
	c.Terrain = "open"

	playerRepo.On("GetByID", ctx, "player-1").Return(p, nil)
	cellRepo.On("GetByID", ctx, "cell-1").Return(c, nil)

	_, err := svc.Place(ctx, "player-1", "cell-1")

	assert.Error(t, err)
	assertAppErrorCode(t, err, "no_power_source")
}

func TestPlace_CellOccupied(t *testing.T) {
	svc, _, cellRepo, playerRepo, _, _ := setupService()
	ctx := context.Background()

	p := defaultPlayer()
	existingTowerID := "existing-tower"
	c := defaultCell()
	c.TowerID = &existingTowerID

	playerRepo.On("GetByID", ctx, "player-1").Return(p, nil)
	cellRepo.On("GetByID", ctx, "cell-1").Return(c, nil)

	_, err := svc.Place(ctx, "player-1", "cell-1")

	assert.Error(t, err)
	assertAppErrorCode(t, err, "cell_occupied")
}

func TestPlace_Overloaded(t *testing.T) {
	svc, towerRepo, cellRepo, playerRepo, _, _ := setupService()
	ctx := context.Background()

	p := defaultPlayer()
	c := defaultCell()

	playerRepo.On("GetByID", ctx, "player-1").Return(p, nil)
	cellRepo.On("GetByID", ctx, "cell-1").Return(c, nil)
	towerRepo.On("CountAllInRadius", ctx, c.Lat, c.Lng, canon.NetworkMinSpacingM).Return(0, nil)
	towerRepo.On("CountAllInRadius", ctx, c.Lat, c.Lng, canon.TowerOverloadRadiusM).Return(canon.TowerOverloadMaxCount, nil)

	_, err := svc.Place(ctx, "player-1", "cell-1")

	assert.Error(t, err)
	assertAppErrorCode(t, err, "overloaded")
}

// ---- Upgrade Tests ----

func TestUpgrade_Success(t *testing.T) {
	svc, towerRepo, _, playerRepo, _, _ := setupService()
	ctx := context.Background()

	existing := &Tower{
		ID:      "tower-1",
		CellID:  "cell-1",
		OwnerID: "player-1",
		Level:   1,
		HP:      100,
		HPMax:   100,
	}
	p := &player.Player{
		ID:        "player-1",
		Energy:    500,
		Materials: 100,
	}

	towerRepo.On("GetByID", ctx, "tower-1").Return(existing, nil)
	// ResourceService.Spend calls GetByID then Update
	playerRepo.On("GetByID", ctx, "player-1").Return(p, nil)
	playerRepo.On("Update", ctx, mock.AnythingOfType("*player.Player")).Return(nil)
	towerRepo.On("Update", ctx, mock.AnythingOfType("*tower.Tower")).Return(nil)

	tower, err := svc.Upgrade(ctx, "tower-1", "player-1")

	assert.NoError(t, err)
	assert.Equal(t, 2, tower.Level)
	assert.Equal(t, 150.0, tower.RadiusM)
	assert.Equal(t, 5.5, tower.EffectPerHour)
	assert.Equal(t, 150, tower.HPMax)
	assert.Equal(t, 150, tower.HP)
	towerRepo.AssertExpectations(t)
}

func TestUpgrade_Level3UsesCanonBalance(t *testing.T) {
	svc, towerRepo, cellRepo, playerRepo, _, _ := setupService()
	ctx := context.Background()

	existing := &Tower{
		ID:      "tower-1",
		CellID:  "cell-1",
		OwnerID: "player-1",
		Level:   2,
		HP:      150,
		HPMax:   150,
	}
	p := &player.Player{
		ID:        "player-1",
		Energy:    1000,
		Materials: 500,
	}
	c := defaultCell()

	towerRepo.On("GetByID", ctx, "tower-1").Return(existing, nil)
	playerRepo.On("GetByID", ctx, "player-1").Return(defaultPlayer(), nil)
	cellRepo.On("GetByID", ctx, "cell-1").Return(c, nil)
	towerRepo.On("CountInRadius", ctx, c.Lat, c.Lng, canon.TowerL3NearbyRadiusM, "player-1").Return(2, nil)
	playerRepo.On("GetByID", ctx, "player-1").Return(p, nil)
	playerRepo.On("Update", ctx, mock.AnythingOfType("*player.Player")).Return(nil)
	towerRepo.On("Update", ctx, mock.AnythingOfType("*tower.Tower")).Return(nil)

	tower, err := svc.Upgrade(ctx, "tower-1", "player-1")

	assert.NoError(t, err)
	assert.Equal(t, 3, tower.Level)
	assert.Equal(t, 220, tower.HPMax)
	assert.Equal(t, 220, tower.HP)
	assert.Equal(t, 200.0, tower.RadiusM)
	assert.Equal(t, 8.0, tower.EffectPerHour)
}

func TestPlace_RoadTerrainAllowed(t *testing.T) {
	svc, towerRepo, cellRepo, playerRepo, _, _ := setupService()
	ctx := context.Background()
	p := defaultPlayer()
	c := defaultCell()
	c.Terrain = "road"

	playerRepo.On("GetByID", ctx, "player-1").Return(p, nil)
	cellRepo.On("GetByID", ctx, "cell-1").Return(c, nil)
	towerRepo.On("CountAllInRadius", ctx, c.Lat, c.Lng, canon.NetworkMinSpacingM).Return(0, nil)
	towerRepo.On("CountAllInRadius", ctx, c.Lat, c.Lng, canon.TowerOverloadRadiusM).Return(0, nil)
	playerRepo.On("GetByID", ctx, "player-1").Return(p, nil)
	playerRepo.On("Update", ctx, mock.AnythingOfType("*player.Player")).Return(nil)
	towerRepo.On("Create", ctx, mock.AnythingOfType("*tower.Tower")).Return(nil)
	cellRepo.On("SetTowerID", ctx, "cell-1", mock.AnythingOfType("*string")).Return(nil)

	tower, err := svc.Place(ctx, "player-1", "cell-1")

	assert.NoError(t, err)
	assert.NotNil(t, tower)
	assert.Equal(t, canon.TowerTypeStandard, tower.Type)
}

func TestUpgrade_Level3RequiresFieldPresence(t *testing.T) {
	svc, towerRepo, cellRepo, playerRepo, _, _ := setupService()
	ctx := context.Background()

	existing := &Tower{
		ID:      "tower-1",
		CellID:  "cell-1",
		OwnerID: "player-1",
		Level:   2,
		HP:      150,
		HPMax:   150,
	}
	c := defaultCell()
	farPlayer := &player.Player{
		ID:        "player-1",
		Energy:    1000,
		Materials: 500,
		Position:  player.Position{Lat: 53.195, Lng: 50.1010},
	}

	towerRepo.On("GetByID", ctx, "tower-1").Return(existing, nil)
	playerRepo.On("GetByID", ctx, "player-1").Return(farPlayer, nil)
	cellRepo.On("GetByID", ctx, "cell-1").Return(c, nil)

	_, err := svc.Upgrade(ctx, "tower-1", "player-1")

	assert.Error(t, err)
	assertAppErrorCode(t, err, "too_far")
}

func TestUpgrade_Level4AvailableAtV1_0(t *testing.T) {
	// Phase D (v1.0): L3→L4 is no longer feature-locked. Upgrade now reaches the
	// field-presence check — a far player fails with "too_far", proving the gate
	// is open (not "feature_locked").
	svc, towerRepo, cellRepo, playerRepo, _, _ := setupService()
	ctx := context.Background()

	existing := &Tower{
		ID:      "tower-1",
		CellID:  "cell-1",
		OwnerID: "player-1",
		Level:   3,
		HP:      220,
		HPMax:   220,
	}
	c := defaultCell()
	farPlayer := &player.Player{
		ID:        "player-1",
		Energy:    100000,
		Materials: 100000,
		Position:  player.Position{Lat: 53.195, Lng: 50.1010},
	}

	towerRepo.On("GetByID", ctx, "tower-1").Return(existing, nil)
	playerRepo.On("GetByID", ctx, "player-1").Return(farPlayer, nil)
	cellRepo.On("GetByID", ctx, "cell-1").Return(c, nil)

	_, err := svc.Upgrade(ctx, "tower-1", "player-1")

	assert.Error(t, err)
	assertAppErrorCode(t, err, "too_far")
}

func TestUpgrade_MaxLevel(t *testing.T) {
	svc, towerRepo, _, _, _, _ := setupService()
	ctx := context.Background()

	existing := &Tower{
		ID:      "tower-1",
		CellID:  "cell-1",
		OwnerID: "player-1",
		Level:   5,
	}

	towerRepo.On("GetByID", ctx, "tower-1").Return(existing, nil)

	_, err := svc.Upgrade(ctx, "tower-1", "player-1")

	assert.Error(t, err)
	assertAppErrorCode(t, err, "max_level")
}

func TestUpgrade_NotOwner(t *testing.T) {
	svc, towerRepo, _, _, _, _ := setupService()
	ctx := context.Background()

	existing := &Tower{
		ID:      "tower-1",
		CellID:  "cell-1",
		OwnerID: "player-1",
		Level:   1,
	}

	towerRepo.On("GetByID", ctx, "tower-1").Return(existing, nil)

	_, err := svc.Upgrade(ctx, "tower-1", "player-2")

	assert.Error(t, err)
	assertAppErrorCode(t, err, "not_owner")
}

// ---- Remove Tests ----

func TestRemove_Success(t *testing.T) {
	svc, towerRepo, cellRepo, playerRepo, _, _ := setupService()
	ctx := context.Background()

	existing := &Tower{
		ID:      "tower-1",
		CellID:  "cell-1",
		OwnerID: "player-1",
		Level:   1,
	}
	p := &player.Player{
		ID:        "player-1",
		Energy:    100,
		Materials: 50,
	}

	towerRepo.On("GetByID", ctx, "tower-1").Return(existing, nil)
	cellRepo.On("SetTowerID", ctx, "cell-1", (*string)(nil)).Return(nil)
	towerRepo.On("Delete", ctx, "tower-1").Return(nil)
	// ResourceService.Add calls GetByID then Update
	playerRepo.On("GetByID", ctx, "player-1").Return(p, nil)
	playerRepo.On("Update", ctx, mock.AnythingOfType("*player.Player")).Return(nil)

	energy, materials, err := svc.Remove(ctx, "tower-1", "player-1")

	assert.NoError(t, err)
	assert.Equal(t, 25, energy)
	assert.Equal(t, 5, materials)
	towerRepo.AssertExpectations(t)
	cellRepo.AssertExpectations(t)
}

func TestRemove_NotOwner(t *testing.T) {
	svc, towerRepo, _, _, _, _ := setupService()
	ctx := context.Background()

	existing := &Tower{
		ID:      "tower-1",
		CellID:  "cell-1",
		OwnerID: "player-1",
		Level:   1,
	}

	towerRepo.On("GetByID", ctx, "tower-1").Return(existing, nil)

	_, _, err := svc.Remove(ctx, "tower-1", "player-2")

	assert.Error(t, err)
	assertAppErrorCode(t, err, "not_owner")
}

func TestRepair_RestoresHPForEnergy(t *testing.T) {
	svc, towerRepo, cellRepo, playerRepo, _, _ := setupService()
	ctx := context.Background()

	p := defaultPlayer() // Energy 500, position 53.195, 50.100
	tw := &Tower{ID: "t1", OwnerID: "player-1", CellID: "cell-1", HP: 40, HPMax: 100}
	towerRepo.On("GetByID", ctx, "t1").Return(tw, nil)
	playerRepo.On("GetByID", ctx, "player-1").Return(p, nil)
	cellRepo.On("GetByID", ctx, "cell-1").Return(&cell.Cell{ID: "cell-1", Lat: 53.195, Lng: 50.100}, nil)
	playerRepo.On("Update", ctx, mock.AnythingOfType("*player.Player")).Return(nil)
	towerRepo.On("Update", ctx, mock.AnythingOfType("*tower.Tower")).Return(nil)

	out, err := svc.Repair(ctx, "t1", "player-1")

	assert.NoError(t, err)
	assert.Equal(t, 100, out.HP)
	// missing 60 HP → cost ceil(60/2) = 30 energy → 500 - 30 = 470
	assert.Equal(t, 470, p.Energy)
}

func TestRepair_RejectsFullHP(t *testing.T) {
	svc, towerRepo, _, _, _, _ := setupService()
	ctx := context.Background()

	towerRepo.On("GetByID", ctx, "t1").Return(
		&Tower{ID: "t1", OwnerID: "player-1", HP: 100, HPMax: 100}, nil)

	_, err := svc.Repair(ctx, "t1", "player-1")

	assert.Error(t, err)
	appErr, ok := err.(*httputil.AppError)
	assert.True(t, ok)
	assert.Equal(t, "not_damaged", appErr.Code)
}

func TestRepair_RejectsNotOwner(t *testing.T) {
	svc, towerRepo, _, _, _, _ := setupService()
	ctx := context.Background()

	towerRepo.On("GetByID", ctx, "t1").Return(
		&Tower{ID: "t1", OwnerID: "someone-else", HP: 10, HPMax: 100}, nil)

	_, err := svc.Repair(ctx, "t1", "player-1")

	assert.Error(t, err)
	appErr, ok := err.(*httputil.AppError)
	assert.True(t, ok)
	assert.Equal(t, "not_owner", appErr.Code)
}

func TestApplyPressure_WarnsWhenBeaconCrossesLowHP(t *testing.T) {
	svc, towerRepo, cellRepo, _, _, pushNotifier := setupService()
	ctx := context.Background()

	// HPMax 100 → warn band at 30. Infection 100 → 25 dmg/h. HP 40 → 15 (≤30,
	// crossing from above) → owner is alerted to run and repair.
	tw := Tower{ID: "t1", OwnerID: "player-1", CellID: "cell-1", HP: 40, HPMax: 100}
	towerRepo.On("GetAll", ctx).Return([]Tower{tw}, nil)
	cellRepo.On("GetByID", ctx, "cell-1").Return(&cell.Cell{ID: "cell-1", Infection: 100}, nil)
	towerRepo.On("Update", ctx, mock.AnythingOfType("*tower.Tower")).Return(nil)

	err := svc.ApplyPressure(ctx)

	assert.NoError(t, err)
	pushNotifier.AssertCalled(t, "NotifyTowerUnderAttack", ctx, "player-1", "Заражение")
}

// fakeEventPublisher captures realtime events for assertions (R4-6).
type fakeEventPublisher struct {
	lastPlayer string
	lastType   string
	lastData   map[string]any
	calls      int
}

func (f *fakeEventPublisher) PublishEvent(playerID, eventType string, data map[string]any) {
	f.calls++
	f.lastPlayer = playerID
	f.lastType = eventType
	f.lastData = data
}

func TestCorrodeTower_PublishesDestroyedEvent(t *testing.T) {
	ctx := context.Background()
	svc, towerRepo, cellRepo, _, _, pushNotifier := setupService()
	events := &fakeEventPublisher{}
	svc.WithEvents(events)

	target := &Tower{ID: "tower-9", OwnerID: "owner-1", HP: 10, HPMax: 100, CellID: "cell-1", Lat: 53.1, Lng: 50.1}
	towerRepo.On("GetByID", ctx, "tower-9").Return(target, nil)
	cellRepo.On("SetTowerID", ctx, "cell-1", (*string)(nil)).Return(nil)
	towerRepo.On("Delete", ctx, "tower-9").Return(nil)
	pushNotifier.On("NotifyTowerUnderAttack", mock.Anything, mock.Anything, mock.Anything).Maybe().Return(nil)

	// Damage exceeds HP → destroyed → live event to the owner.
	res, err := svc.CorrodeTower(ctx, "tower-9", "sym-1", 25)
	assert.NoError(t, err)
	assert.True(t, res.Found)
	assert.True(t, res.Destroyed)
	assert.Equal(t, 1, events.calls)
	assert.Equal(t, "owner-1", events.lastPlayer)
	assert.Equal(t, "tower_destroyed", events.lastType)
	assert.Equal(t, "symbiont", events.lastData["cause"])
}

func TestApplyPressure_PublishesRichUnderAttackEvent(t *testing.T) {
	svc, towerRepo, cellRepo, _, _, pushNotifier := setupService()
	ctx := context.Background()
	events := &fakeEventPublisher{}
	svc.WithEvents(events)
	pushNotifier.On("NotifyTowerUnderAttack", mock.Anything, mock.Anything, mock.Anything).Maybe().Return(nil)

	// HPMax 100 → warn band 30. Infection 100 → 25 dmg/h. HP 40 → 15 (crosses
	// into the band) → owner gets a rich realtime event to navigate + a countdown.
	tw := Tower{ID: "t1", OwnerID: "player-1", CellID: "cell-1", HP: 40, HPMax: 100, Lat: 53.2, Lng: 50.1}
	towerRepo.On("GetAll", ctx).Return([]Tower{tw}, nil)
	cellRepo.On("GetByID", ctx, "cell-1").Return(&cell.Cell{ID: "cell-1", Infection: 100}, nil)
	towerRepo.On("Update", ctx, mock.AnythingOfType("*tower.Tower")).Return(nil)

	err := svc.ApplyPressure(ctx)
	assert.NoError(t, err)

	assert.Equal(t, "tower_under_attack", events.lastType)
	assert.Equal(t, "player-1", events.lastPlayer)
	assert.Equal(t, "t1", events.lastData["tower_id"])
	assert.Equal(t, 53.2, events.lastData["lat"])
	assert.Equal(t, 50.1, events.lastData["lng"])
	assert.Equal(t, 100, events.lastData["hp_max"])
	assert.Equal(t, "infection", events.lastData["cause"])
	// eta_seconds = remaining HP(15) / 25 dmg/h * 3600 = 2160
	assert.Equal(t, 2160, events.lastData["eta_seconds"])
}

// fakeAllies is a stub AllyResolver for ally-repair tests (R4-4b).
type fakeAllies struct{ linked bool }

func (f fakeAllies) AreLinked(context.Context, string, string) (bool, error) { return f.linked, nil }
func (f fakeAllies) LinkedPartnerIDs(context.Context, string) ([]string, error) {
	return []string{"ally-1"}, nil
}

func TestRepair_AllyMayRepairLinkedBeacon(t *testing.T) {
	ctx := context.Background()
	svc, towerRepo, cellRepo, playerRepo, _, _ := setupService()
	svc.WithAllies(fakeAllies{linked: true})

	// Beacon owned by someone else; the repairer is a linked ally standing on it.
	beacon := &Tower{ID: "tw-1", OwnerID: "owner-2", HP: 40, HPMax: 100, CellID: "cell-1", Lat: 53.195, Lng: 50.1001}
	towerRepo.On("GetByID", ctx, "tw-1").Return(beacon, nil)
	cellRepo.On("GetByID", ctx, "cell-1").Return(defaultCell(), nil)
	playerRepo.On("GetByID", ctx, "ally-player").Return(defaultPlayer(), nil)
	playerRepo.On("Update", ctx, mock.AnythingOfType("*player.Player")).Return(nil)
	towerRepo.On("Update", ctx, mock.AnythingOfType("*tower.Tower")).Return(nil)

	out, err := svc.Repair(ctx, "tw-1", "ally-player")
	assert.NoError(t, err)
	assert.Equal(t, out.HPMax, out.HP) // restored to full

	// Without a link, the same repair is forbidden.
	svc2, towerRepo2, _, _, _, _ := setupService()
	svc2.WithAllies(fakeAllies{linked: false})
	towerRepo2.On("GetByID", ctx, "tw-1").Return(beacon, nil)
	_, err = svc2.Repair(ctx, "tw-1", "stranger")
	assertAppErrorCode(t, err, "not_owner")
}

// fakeFaction is a stub FactionChecker (T-800/T-801). Players listed in
// symbionts are Symbiont; everyone else is Human.
type fakeFaction struct{ symbionts map[string]bool }

func (f fakeFaction) IsSymbiont(_ context.Context, playerID string) (bool, error) {
	return f.symbionts[playerID], nil
}

// fakeNetworkHook is a NetworkHook stub that fails the test if called — used
// to prove the T-800 gate short-circuits PlaceAtPlayer before touching it.
type fakeNetworkHook struct{ t *testing.T }

func (f fakeNetworkHook) Recompute(context.Context, string) error {
	f.t.Fatal("unexpected call")
	return nil
}
func (f fakeNetworkHook) CanConnect(context.Context, string, float64, float64) (bool, bool, error) {
	f.t.Fatal("unexpected call")
	return false, false, nil
}
func (f fakeNetworkHook) NearestCellID(context.Context, float64, float64) (string, error) {
	f.t.Fatal("unexpected call")
	return "", nil
}

func TestPlace_RejectsSymbiont(t *testing.T) {
	svc, _, _, _, _, _ := setupService()
	svc.WithFaction(fakeFaction{symbionts: map[string]bool{"sym-1": true}})
	ctx := context.Background()

	_, err := svc.Place(ctx, "sym-1", "cell-1")

	assertAppErrorCode(t, err, "symbiont_no_human_toolkit")
}

func TestPlaceAtPlayer_RejectsSymbiont(t *testing.T) {
	svc, _, _, _, _, _ := setupService()
	svc.WithFaction(fakeFaction{symbionts: map[string]bool{"sym-1": true}})
	svc.WithNetwork(fakeNetworkHook{t: t}) // must not be reached
	ctx := context.Background()

	_, err := svc.PlaceAtPlayer(ctx, "sym-1")

	assertAppErrorCode(t, err, "symbiont_no_human_toolkit")
}

func TestPlace_HumanNotBlockedByFactionGate(t *testing.T) {
	svc, towerRepo, cellRepo, playerRepo, questProgressor, pushNotifier := setupService()
	svc.WithFaction(fakeFaction{symbionts: map[string]bool{"sym-1": true}}) // player-1 is Human
	ctx := context.Background()
	p := defaultPlayer()
	c := defaultCell()

	playerRepo.On("GetByID", ctx, "player-1").Return(p, nil)
	cellRepo.On("GetByID", ctx, "cell-1").Return(c, nil)
	towerRepo.On("CountAllInRadius", ctx, c.Lat, c.Lng, canon.NetworkMinSpacingM).Return(0, nil)
	towerRepo.On("CountAllInRadius", ctx, c.Lat, c.Lng, canon.TowerOverloadRadiusM).Return(2, nil)
	playerRepo.On("Update", ctx, mock.AnythingOfType("*player.Player")).Return(nil)
	towerRepo.On("Create", ctx, mock.AnythingOfType("*tower.Tower")).Return(nil)
	cellRepo.On("SetTowerID", ctx, "cell-1", mock.AnythingOfType("*string")).Return(nil)
	questProgressor.On("UpdateProgress", ctx, "player-1", "place_beacon").Return(nil)
	pushNotifier.On("NotifyTowerPlaced", ctx, "player-1").Return(nil)

	_, err := svc.Place(ctx, "player-1", "cell-1")

	assert.NoError(t, err)
}

func TestCorrode_SkipsFellowSymbiontBeacon(t *testing.T) {
	svc, towerRepo, _, _, _, _ := setupService()
	svc.WithFaction(fakeFaction{symbionts: map[string]bool{"sym-2": true}})
	ctx := context.Background()

	// sym-2's beacon is the only one in radius, but it's a fellow Symbiont's —
	// not a valid Overload target (T-801).
	fellow := Tower{ID: "tw-fellow", OwnerID: "sym-2", HP: 100, HPMax: 100, Lat: 53.1, Lng: 50.1}
	towerRepo.On("GetInRadius", ctx, 53.1, 50.1, 150.0).Return([]Tower{fellow}, nil)

	res, err := svc.Corrode(ctx, 53.1, 50.1, "sym-1", 25, 150)

	assert.NoError(t, err)
	assert.False(t, res.Found)
}

func TestWeakestHostile_SkipsFellowSymbiontBeacon(t *testing.T) {
	svc, towerRepo, _, _, _, _ := setupService()
	svc.WithFaction(fakeFaction{symbionts: map[string]bool{"sym-2": true}})
	ctx := context.Background()

	fellow := Tower{ID: "tw-fellow", OwnerID: "sym-2", HP: 10, HPMax: 100, Lat: 53.1, Lng: 50.1}
	towerRepo.On("GetInRadius", ctx, 53.1, 50.1, 300.0).Return([]Tower{fellow}, nil)

	res, err := svc.WeakestHostile(ctx, 53.1, 50.1, "sym-1", 300)

	assert.NoError(t, err)
	assert.False(t, res.Found)
}

func TestCorrodeTower_SkipsFellowSymbiontBeacon(t *testing.T) {
	svc, towerRepo, _, _, _, _ := setupService()
	svc.WithFaction(fakeFaction{symbionts: map[string]bool{"sym-2": true}})
	ctx := context.Background()

	fellow := &Tower{ID: "tw-fellow", OwnerID: "sym-2", HP: 100, HPMax: 100}
	towerRepo.On("GetByID", ctx, "tw-fellow").Return(fellow, nil)

	res, err := svc.CorrodeTower(ctx, "tw-fellow", "sym-1", 25)

	assert.NoError(t, err)
	assert.False(t, res.Found)
}
