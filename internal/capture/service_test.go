package capture

import (
	"context"
	"testing"

	"github.com/ezra-game/server/internal/cell"
	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/internal/tower"
	"github.com/ezra-game/server/pkg/httputil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

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
func (m *mockPlayerRepo) GetAll(ctx context.Context) ([]player.Player, error) {
	args := m.Called(ctx)
	if v := args.Get(0); v != nil {
		return v.([]player.Player), args.Error(1)
	}
	return nil, args.Error(1)
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

func TestLockpick_FeatureLockedInMVP(t *testing.T) {
	svc := NewService(&mockTowerRepo{}, &mockPlayerRepo{}, &mockCellRepo{}, nil)

	_, err := svc.Lockpick(context.Background(), "tower-1", "player-1")

	assertAppErrorCode(t, err, "feature_locked")
}

func TestValidateForceCapture_FeatureLockedInMVP(t *testing.T) {
	svc := NewService(&mockTowerRepo{}, &mockPlayerRepo{}, &mockCellRepo{}, nil)

	_, err := svc.ValidateForceCapture(context.Background(), "tower-1", "player-1")

	assertAppErrorCode(t, err, "feature_locked")
}

func assertAppErrorCode(t *testing.T, err error, expectedCode string) {
	t.Helper()
	appErr, ok := err.(*httputil.AppError)
	if !ok {
		t.Fatalf("expected *httputil.AppError, got %T: %v", err, err)
	}
	assert.Equal(t, expectedCode, appErr.Code)
}
