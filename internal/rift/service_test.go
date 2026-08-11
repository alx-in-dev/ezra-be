package rift

import (
	"context"
	"testing"

	"github.com/ezra-game/server/internal/cell"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// --- Mock rift.Repository ---

type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) Create(ctx context.Context, r *Rift) error {
	args := m.Called(ctx, r)
	if args.Error(0) == nil {
		r.ID = "rift-1"
	}
	return args.Error(0)
}

func (m *MockRepository) GetByID(ctx context.Context, id string) (*Rift, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Rift), args.Error(1)
}

func (m *MockRepository) GetOpen(ctx context.Context) ([]Rift, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]Rift), args.Error(1)
}

func (m *MockRepository) GetNearby(ctx context.Context, lat, lng, radiusM float64) ([]Rift, error) {
	args := m.Called(ctx, lat, lng, radiusM)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]Rift), args.Error(1)
}

func (m *MockRepository) Update(ctx context.Context, r *Rift) error {
	args := m.Called(ctx, r)
	return args.Error(0)
}

func (m *MockRepository) Close(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// --- Mock cell.Repository ---

type MockCellRepository struct {
	mock.Mock
}

func (m *MockCellRepository) GetAll(ctx context.Context) ([]cell.Cell, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]cell.Cell), args.Error(1)
}

func (m *MockCellRepository) GetInRadius(ctx context.Context, lat, lng, radiusM float64) ([]cell.Cell, error) {
	args := m.Called(ctx, lat, lng, radiusM)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]cell.Cell), args.Error(1)
}

func (m *MockCellRepository) GetByID(ctx context.Context, id string) (*cell.Cell, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*cell.Cell), args.Error(1)
}

func (m *MockCellRepository) GetNeighbors(ctx context.Context, cellID string) ([]cell.Cell, error) {
	args := m.Called(ctx, cellID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]cell.Cell), args.Error(1)
}

func (m *MockCellRepository) Update(ctx context.Context, id string, infection float64) error {
	args := m.Called(ctx, id, infection)
	return args.Error(0)
}

func (m *MockCellRepository) SetTowerID(ctx context.Context, cellID string, towerID *string) error {
	args := m.Called(ctx, cellID, towerID)
	return args.Error(0)
}

func (m *MockCellRepository) StateSignature(ctx context.Context, lat, lng, radiusM float64) (string, error) {
	return "", nil
}

func (m *MockCellRepository) SetRiftID(ctx context.Context, cellID string, riftID *string) error {
	args := m.Called(ctx, cellID, riftID)
	return args.Error(0)
}

// --- Tests ---

func TestMaybeSpawn_InfectionTooLow(t *testing.T) {
	riftRepo := new(MockRepository)
	cellRepo := new(MockCellRepository)
	svc := NewService(riftRepo, cellRepo, nil)
	ctx := context.Background()

	result, err := svc.MaybeSpawn(ctx, "cell-1", 60)

	assert.NoError(t, err)
	assert.Nil(t, result)
	// Should not even check the cell
	cellRepo.AssertNotCalled(t, "GetByID", mock.Anything, mock.Anything)
}

func TestMaybeSpawn_CellHasRift(t *testing.T) {
	riftRepo := new(MockRepository)
	cellRepo := new(MockCellRepository)
	svc := NewService(riftRepo, cellRepo, nil)
	ctx := context.Background()

	existingRiftID := "existing-rift"
	c := &cell.Cell{ID: "cell-1", RiftID: &existingRiftID, Infection: 80}
	cellRepo.On("GetByID", ctx, "cell-1").Return(c, nil)

	result, err := svc.MaybeSpawn(ctx, "cell-1", 80)

	assert.NoError(t, err)
	assert.Nil(t, result)
	// Should not attempt to create a rift
	riftRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	cellRepo.AssertExpectations(t)
}

func TestClose_Success(t *testing.T) {
	riftRepo := new(MockRepository)
	cellRepo := new(MockCellRepository)
	svc := NewService(riftRepo, cellRepo, nil)
	ctx := context.Background()

	r := &Rift{ID: "rift-1", CellID: "cell-1", Type: "minor"}
	riftRepo.On("GetByID", ctx, "rift-1").Return(r, nil)
	riftRepo.On("Close", ctx, "rift-1").Return(nil)
	cellRepo.On("SetRiftID", ctx, "cell-1", (*string)(nil)).Return(nil)

	err := svc.Close(ctx, "rift-1")

	assert.NoError(t, err)
	riftRepo.AssertExpectations(t)
	cellRepo.AssertExpectations(t)
}
