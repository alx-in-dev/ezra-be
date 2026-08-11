package infection

import (
	"context"
	"testing"

	"github.com/ezra-game/server/internal/cell"
	"github.com/ezra-game/server/internal/rift"
	"github.com/ezra-game/server/internal/tower"
	"github.com/ezra-game/server/pkg/geo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// --- Mock cell.Repository ---

type mockCellRepo struct {
	mock.Mock
}

func (m *mockCellRepo) GetAll(ctx context.Context) ([]cell.Cell, error) {
	args := m.Called(ctx)
	return args.Get(0).([]cell.Cell), args.Error(1)
}

func (m *mockCellRepo) GetByID(ctx context.Context, id string) (*cell.Cell, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*cell.Cell), args.Error(1)
}

func (m *mockCellRepo) GetNeighbors(ctx context.Context, cellID string) ([]cell.Cell, error) {
	args := m.Called(ctx, cellID)
	return args.Get(0).([]cell.Cell), args.Error(1)
}

func (m *mockCellRepo) Update(ctx context.Context, id string, infection float64) error {
	args := m.Called(ctx, id, infection)
	return args.Error(0)
}

func (m *mockCellRepo) SetTowerID(ctx context.Context, cellID string, towerID *string) error {
	args := m.Called(ctx, cellID, towerID)
	return args.Error(0)
}

func (m *mockCellRepo) StateSignature(ctx context.Context, lat, lng, radiusM float64) (string, error) {
	return "", nil
}

func (m *mockCellRepo) SetRiftID(ctx context.Context, cellID string, riftID *string) error {
	args := m.Called(ctx, cellID, riftID)
	return args.Error(0)
}

func (m *mockCellRepo) GetInRadius(ctx context.Context, lat, lng, radiusM float64) ([]cell.Cell, error) {
	args := m.Called(ctx, lat, lng, radiusM)
	return args.Get(0).([]cell.Cell), args.Error(1)
}

// --- Mock tower.Repository ---

type mockTowerRepo struct {
	mock.Mock
}

func (m *mockTowerRepo) Create(ctx context.Context, t *tower.Tower) error {
	args := m.Called(ctx, t)
	return args.Error(0)
}

func (m *mockTowerRepo) GetAll(ctx context.Context) ([]tower.Tower, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]tower.Tower), args.Error(1)
}

func (m *mockTowerRepo) GetByID(ctx context.Context, id string) (*tower.Tower, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*tower.Tower), args.Error(1)
}

func (m *mockTowerRepo) GetByIDs(ctx context.Context, ids []string) ([]tower.Tower, error) {
	args := m.Called(ctx, ids)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]tower.Tower), args.Error(1)
}

func (m *mockTowerRepo) GetByOwner(ctx context.Context, playerID string) ([]tower.Tower, error) {
	args := m.Called(ctx, playerID)
	return args.Get(0).([]tower.Tower), args.Error(1)
}

func (m *mockTowerRepo) GetInRadius(ctx context.Context, lat, lng, radiusM float64) ([]tower.Tower, error) {
	args := m.Called(ctx, lat, lng, radiusM)
	return args.Get(0).([]tower.Tower), args.Error(1)
}

func (m *mockTowerRepo) Update(ctx context.Context, t *tower.Tower) error {
	args := m.Called(ctx, t)
	return args.Error(0)
}

func (m *mockTowerRepo) UpdateOwnership(ctx context.Context, id, newOwnerID string, hp, hpMax int) error {
	args := m.Called(ctx, id, newOwnerID, hp, hpMax)
	return args.Error(0)
}

func (m *mockTowerRepo) Delete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *mockTowerRepo) CountInRadius(ctx context.Context, lat, lng, radiusM float64, ownerID string) (int, error) {
	args := m.Called(ctx, lat, lng, radiusM, ownerID)
	return args.Int(0), args.Error(1)
}

func (m *mockTowerRepo) CountAllInRadius(ctx context.Context, lat, lng, radiusM float64) (int, error) {
	args := m.Called(ctx, lat, lng, radiusM)
	return args.Int(0), args.Error(1)
}

// --- Mock rift.Repository ---

type mockRiftRepo struct {
	mock.Mock
}

func (m *mockRiftRepo) Create(ctx context.Context, r *rift.Rift) error {
	args := m.Called(ctx, r)
	return args.Error(0)
}

func (m *mockRiftRepo) GetByID(ctx context.Context, id string) (*rift.Rift, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*rift.Rift), args.Error(1)
}

func (m *mockRiftRepo) GetOpen(ctx context.Context) ([]rift.Rift, error) {
	args := m.Called(ctx)
	return args.Get(0).([]rift.Rift), args.Error(1)
}

func (m *mockRiftRepo) GetNearby(ctx context.Context, lat, lng, radiusM float64) ([]rift.Rift, error) {
	args := m.Called(ctx, lat, lng, radiusM)
	return args.Get(0).([]rift.Rift), args.Error(1)
}

func (m *mockRiftRepo) Update(ctx context.Context, r *rift.Rift) error {
	args := m.Called(ctx, r)
	return args.Error(0)
}

func (m *mockRiftRepo) Close(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// --- Tests ---

func TestRecalculateCell_BaseGrowth(t *testing.T) {
	cells := &mockCellRepo{}
	rifts := &mockRiftRepo{}
	towers := &mockTowerRepo{}
	svc := NewService(cells, rifts, towers)
	ctx := context.Background()

	targetCell := &cell.Cell{ID: "c1", Lat: 0.0, Lng: 0.0, Infection: 10.0, Terrain: "plain"}

	cells.On("GetByID", ctx, "c1").Return(targetCell, nil)
	cells.On("GetNeighbors", ctx, "c1").Return([]cell.Cell{}, nil)
	rifts.On("GetNearby", ctx, 0.0, 0.0, maxRiftSearchM).Return([]rift.Rift{}, nil)
	towers.On("GetInRadius", ctx, 0.0, 0.0, maxTowerSearchM).Return([]tower.Tower{}, nil)
	cells.On("Update", ctx, "c1", mock.AnythingOfType("float64")).Return(nil)

	result, err := svc.RecalculateCell(ctx, "c1")

	assert.NoError(t, err)
	// growth = 0.8 * (5/60) ≈ 0.0667
	expectedInfection := 10.0 + 0.8*(5.0/60.0)
	assert.InDelta(t, expectedInfection, result.NewInfection, 0.01)
	assert.InDelta(t, 10.0, result.OldInfection, 0.01)
	assert.InDelta(t, 0.0667, result.Growth, 0.01)
	assert.InDelta(t, 0.0, result.TowerEffect, 0.01)
	cells.AssertExpectations(t)
	rifts.AssertExpectations(t)
	towers.AssertExpectations(t)
}

func TestRecalculateCell_TowerEffect(t *testing.T) {
	cells := &mockCellRepo{}
	rifts := &mockRiftRepo{}
	towers := &mockTowerRepo{}
	svc := NewService(cells, rifts, towers)
	ctx := context.Background()

	// Main cell at (0, 0).
	targetCell := &cell.Cell{ID: "c1", Lat: 0.0, Lng: 0.0, Infection: 50.0, Terrain: "plain"}

	// Tower's cell ~50m north. At equator, ~0.00045 deg ≈ 50m.
	towerCellLat := 0.00045
	actualDist := geo.Haversine(0.0, 0.0, towerCellLat, 0.0)

	towerCell := &cell.Cell{ID: "tc1", Lat: towerCellLat, Lng: 0.0, Infection: 0.0, Terrain: "plain"}

	tw := tower.Tower{
		ID: "t1", CellID: "tc1", OwnerID: "p1", Level: 1,
		RadiusM: 100.0, EffectPerHour: 3.0, HP: 100, HPMax: 100,
	}

	cells.On("GetByID", ctx, "c1").Return(targetCell, nil)
	cells.On("GetByID", ctx, "tc1").Return(towerCell, nil)
	cells.On("GetNeighbors", ctx, "c1").Return([]cell.Cell{}, nil)
	rifts.On("GetNearby", ctx, 0.0, 0.0, maxRiftSearchM).Return([]rift.Rift{}, nil)
	towers.On("GetInRadius", ctx, 0.0, 0.0, maxTowerSearchM).Return([]tower.Tower{tw}, nil)
	cells.On("Update", ctx, "c1", mock.AnythingOfType("float64")).Return(nil)

	result, err := svc.RecalculateCell(ctx, "c1")

	assert.NoError(t, err)

	// falloff = 1 - 0.4*(dist/100)
	falloff := 1.0 - 0.4*(actualDist/100.0)
	towerEffect := 3.0 * falloff * (5.0 / 60.0)
	growthDelta := 0.8 * (5.0 / 60.0)
	expected := 50.0 + growthDelta - towerEffect

	assert.InDelta(t, expected, result.NewInfection, 0.01)
	assert.InDelta(t, 50.0, result.OldInfection, 0.01)
	cells.AssertExpectations(t)
	rifts.AssertExpectations(t)
	towers.AssertExpectations(t)
}

func TestRecalculateCell_NeighborBonus(t *testing.T) {
	cells := &mockCellRepo{}
	rifts := &mockRiftRepo{}
	towers := &mockTowerRepo{}
	svc := NewService(cells, rifts, towers)
	ctx := context.Background()

	targetCell := &cell.Cell{ID: "c1", Lat: 0.0, Lng: 0.0, Infection: 30.0, Terrain: "plain"}

	neighbors := []cell.Cell{
		{ID: "n1", Lat: 0.001, Lng: 0.0, Infection: 60.0, Terrain: "plain"},
		{ID: "n2", Lat: 0.0, Lng: 0.001, Infection: 70.0, Terrain: "plain"},
		{ID: "n3", Lat: -0.001, Lng: 0.0, Infection: 80.0, Terrain: "plain"},
	}

	cells.On("GetByID", ctx, "c1").Return(targetCell, nil)
	cells.On("GetNeighbors", ctx, "c1").Return(neighbors, nil)
	rifts.On("GetNearby", ctx, 0.0, 0.0, maxRiftSearchM).Return([]rift.Rift{}, nil)
	towers.On("GetInRadius", ctx, 0.0, 0.0, maxTowerSearchM).Return([]tower.Tower{}, nil)
	cells.On("Update", ctx, "c1", mock.AnythingOfType("float64")).Return(nil)

	result, err := svc.RecalculateCell(ctx, "c1")

	assert.NoError(t, err)
	// growth = 0.8 + 0.3*3 = 1.7; delta = 1.7 * (5/60) ≈ 0.1417
	expectedGrowth := 1.7 * (5.0 / 60.0)
	expectedInfection := 30.0 + expectedGrowth
	assert.InDelta(t, expectedInfection, result.NewInfection, 0.01)
	assert.InDelta(t, expectedGrowth, result.Growth, 0.01)
	assert.InDelta(t, 0.0, result.TowerEffect, 0.01)
	cells.AssertExpectations(t)
	rifts.AssertExpectations(t)
	towers.AssertExpectations(t)
}

func TestRecalculateCell_ClampMin(t *testing.T) {
	cells := &mockCellRepo{}
	rifts := &mockRiftRepo{}
	towers := &mockTowerRepo{}
	svc := NewService(cells, rifts, towers)
	ctx := context.Background()

	// Cell with very low infection.
	targetCell := &cell.Cell{ID: "c1", Lat: 0.0, Lng: 0.0, Infection: 0.01, Terrain: "plain"}

	// Place a strong tower very close (same cell position -> dist ≈ 0 -> falloff = 1.0).
	towerCell := &cell.Cell{ID: "tc1", Lat: 0.0, Lng: 0.0, Infection: 0.0, Terrain: "plain"}
	tw := tower.Tower{
		ID: "t1", CellID: "tc1", OwnerID: "p1", Level: 5,
		RadiusM: 200.0, EffectPerHour: 20.0, HP: 100, HPMax: 100,
	}

	cells.On("GetByID", ctx, "c1").Return(targetCell, nil)
	cells.On("GetByID", ctx, "tc1").Return(towerCell, nil)
	cells.On("GetNeighbors", ctx, "c1").Return([]cell.Cell{}, nil)
	rifts.On("GetNearby", ctx, 0.0, 0.0, maxRiftSearchM).Return([]rift.Rift{}, nil)
	towers.On("GetInRadius", ctx, 0.0, 0.0, maxTowerSearchM).Return([]tower.Tower{tw}, nil)
	cells.On("Update", ctx, "c1", mock.AnythingOfType("float64")).Return(nil)

	result, err := svc.RecalculateCell(ctx, "c1")

	assert.NoError(t, err)
	// Tower effect = 20.0 * 1.0 * (5/60) = 1.667, growth = 0.0667
	// Raw = 0.01 + 0.0667 - 1.667 = -1.59 → clamped to 0.0
	assert.InDelta(t, 0.0, result.NewInfection, 0.01)
	cells.AssertExpectations(t)
	rifts.AssertExpectations(t)
	towers.AssertExpectations(t)
}

func TestRecalculateCell_ClampMax(t *testing.T) {
	cells := &mockCellRepo{}
	rifts := &mockRiftRepo{}
	towers := &mockTowerRepo{}
	svc := NewService(cells, rifts, towers)
	ctx := context.Background()

	// Cell near 100% with highly-infected neighbors.
	targetCell := &cell.Cell{ID: "c1", Lat: 0.0, Lng: 0.0, Infection: 99.95, Terrain: "plain"}

	neighbors := []cell.Cell{
		{ID: "n1", Infection: 90.0, Terrain: "plain"},
		{ID: "n2", Infection: 80.0, Terrain: "plain"},
		{ID: "n3", Infection: 70.0, Terrain: "plain"},
		{ID: "n4", Infection: 60.0, Terrain: "plain"},
		{ID: "n5", Infection: 55.0, Terrain: "plain"},
	}

	cells.On("GetByID", ctx, "c1").Return(targetCell, nil)
	cells.On("GetNeighbors", ctx, "c1").Return(neighbors, nil)
	rifts.On("GetNearby", ctx, 0.0, 0.0, maxRiftSearchM).Return([]rift.Rift{}, nil)
	towers.On("GetInRadius", ctx, 0.0, 0.0, maxTowerSearchM).Return([]tower.Tower{}, nil)
	cells.On("Update", ctx, "c1", mock.AnythingOfType("float64")).Return(nil)

	result, err := svc.RecalculateCell(ctx, "c1")

	assert.NoError(t, err)
	// growth = 0.8 + 0.3*5 = 2.3; delta = 2.3*(5/60) ≈ 0.1917
	// Raw = 99.95 + 0.1917 = 100.1417 → clamped to 100.0
	assert.InDelta(t, 100.0, result.NewInfection, 0.01)
	cells.AssertExpectations(t)
	rifts.AssertExpectations(t)
	towers.AssertExpectations(t)
}

func TestRecalculateCell_RiftBonus(t *testing.T) {
	cells := &mockCellRepo{}
	rifts := &mockRiftRepo{}
	towers := &mockTowerRepo{}
	svc := NewService(cells, rifts, towers)
	ctx := context.Background()

	targetCell := &cell.Cell{ID: "c1", Lat: 10.0, Lng: 20.0, Infection: 40.0, Terrain: "plain"}
	nearbyRifts := []rift.Rift{
		{ID: "r1", Type: "minor"},
		{ID: "r2", Type: "medium"},
	}

	cells.On("GetByID", ctx, "c1").Return(targetCell, nil)
	cells.On("GetNeighbors", ctx, "c1").Return([]cell.Cell{}, nil)
	rifts.On("GetNearby", ctx, 10.0, 20.0, maxRiftSearchM).Return(nearbyRifts, nil)
	towers.On("GetInRadius", ctx, 10.0, 20.0, maxTowerSearchM).Return([]tower.Tower{}, nil)
	cells.On("Update", ctx, "c1", mock.AnythingOfType("float64")).Return(nil)

	result, err := svc.RecalculateCell(ctx, "c1")

	assert.NoError(t, err)
	expectedGrowth := (0.8 + 2.5 + 5.0) * (5.0 / 60.0)
	assert.InDelta(t, 40.0+expectedGrowth, result.NewInfection, 0.01)
	assert.InDelta(t, (2.5+5.0)*(5.0/60.0), result.RiftEffect, 0.01)
	cells.AssertExpectations(t)
	rifts.AssertExpectations(t)
	towers.AssertExpectations(t)
}
