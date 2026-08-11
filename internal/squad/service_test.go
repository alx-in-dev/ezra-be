package squad

import (
	"context"
	"testing"
	"time"

	"github.com/ezra-game/server/internal/canon"
	"github.com/ezra-game/server/internal/cell"
	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/internal/unit"
	"github.com/ezra-game/server/pkg/httputil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockSquadRepo struct{ mock.Mock }

func (m *mockSquadRepo) Create(ctx context.Context, s *Squad) error {
	args := m.Called(ctx, s)
	if args.Error(0) == nil {
		s.ID = "squad-1"
	}
	return args.Error(0)
}
func (m *mockSquadRepo) GetByID(ctx context.Context, id string) (*Squad, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*Squad), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockSquadRepo) GetByPlayer(ctx context.Context, playerID string) ([]Squad, error) {
	args := m.Called(ctx, playerID)
	if v := args.Get(0); v != nil {
		return v.([]Squad), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockSquadRepo) Update(ctx context.Context, s *Squad) error {
	return m.Called(ctx, s).Error(0)
}
func (m *mockSquadRepo) Delete(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}
func (m *mockSquadRepo) GetDueMissions(ctx context.Context, before time.Time) ([]Squad, error) {
	args := m.Called(ctx, before)
	if v := args.Get(0); v != nil {
		return v.([]Squad), args.Error(1)
	}
	return nil, args.Error(1)
}

// --- Fakes for mission-completion deps ---

type fakeResAdder struct {
	energy, materials int
	player            string
}

func (f *fakeResAdder) Add(_ context.Context, playerID string, e, m int) (*player.Player, error) {
	f.energy += e
	f.materials += m
	f.player = playerID
	return &player.Player{ID: playerID}, nil
}

type fakeMissionCells struct{ cleansed bool }

func (f *fakeMissionCells) GetByID(_ context.Context, id string) (*cell.Cell, error) {
	return &cell.Cell{ID: id, Lat: 1, Lng: 2}, nil
}
func (f *fakeMissionCells) ClearInfectionInRadius(_ context.Context, _, _, _, _ float64) (int64, error) {
	f.cleansed = true
	return 3, nil
}

type fakeMissionNotifier struct {
	called bool
	mtype  string
}

func (f *fakeMissionNotifier) NotifySquadReturned(_ context.Context, _, missionType string) error {
	f.called = true
	f.mtype = missionType
	return nil
}

type mockUnitRepo struct{ mock.Mock }

func (m *mockUnitRepo) Create(ctx context.Context, u *unit.Unit) error {
	return m.Called(ctx, u).Error(0)
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

func TestCreate_AssignsUnitsAndReturnsCount(t *testing.T) {
	repo := &mockSquadRepo{}
	units := &mockUnitRepo{}
	svc := NewService(repo, units)
	ctx := context.Background()

	unit1 := &unit.Unit{ID: "u1", PlayerID: "p1", Status: "idle"}
	unit2 := &unit.Unit{ID: "u2", PlayerID: "p1", Status: "idle"}
	units.On("GetByID", ctx, "u1").Return(unit1, nil)
	units.On("GetByID", ctx, "u2").Return(unit2, nil)
	repo.On("Create", ctx, mock.AnythingOfType("*squad.Squad")).Return(nil)
	units.On("AssignSquad", ctx, "u1", mock.AnythingOfType("*string")).Return(nil)
	units.On("AssignSquad", ctx, "u2", mock.AnythingOfType("*string")).Return(nil)

	sq, err := svc.Create(ctx, "p1", "Alpha", []string{"u1", "u2"})

	assert.NoError(t, err)
	assert.Equal(t, "squad-1", sq.ID)
	assert.Equal(t, 2, sq.UnitCount)
}

func TestList_FillsUnitCount(t *testing.T) {
	repo := &mockSquadRepo{}
	units := &mockUnitRepo{}
	svc := NewService(repo, units)
	ctx := context.Background()

	squads := []Squad{
		{ID: "s1", PlayerID: "p1", Name: "Alpha", Status: "idle", CreatedAt: time.Now()},
		{ID: "s2", PlayerID: "p1", Name: "Beta", Status: "idle", CreatedAt: time.Now()},
	}
	s1 := "s1"
	repo.On("GetByPlayer", ctx, "p1").Return(squads, nil)
	units.On("GetByPlayer", ctx, "p1").Return([]unit.Unit{
		{ID: "u1", PlayerID: "p1", SquadID: &s1, Status: "idle"},
		{ID: "u2", PlayerID: "p1", SquadID: &s1, Status: "idle"},
		{ID: "u3", PlayerID: "p1", Status: "idle"},
	}, nil)

	result, err := svc.List(ctx, "p1")

	assert.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, 2, result[0].UnitCount)
	assert.Equal(t, 0, result[1].UnitCount)
}

func TestReturn_ResetsMission(t *testing.T) {
	repo := &mockSquadRepo{}
	units := &mockUnitRepo{}
	svc := NewService(repo, units)
	ctx := context.Background()

	sq := &Squad{
		ID:       "s1",
		PlayerID: "p1",
		Name:     "Alpha",
		Status:   "on_mission",
		Mission:  &Mission{Type: "attack_rift", TargetID: "r1"},
	}
	repo.On("GetByID", ctx, "s1").Return(sq, nil)
	repo.On("Update", ctx, sq).Return(nil)
	units.On("GetByPlayer", ctx, "p1").Return([]unit.Unit{}, nil)

	result, err := svc.Return(ctx, "s1", "p1")

	assert.NoError(t, err)
	assert.Equal(t, "idle", result.Status)
	assert.Nil(t, result.Mission)
}

func TestDisband_FreesUnitsAndDeletes(t *testing.T) {
	repo := &mockSquadRepo{}
	units := &mockUnitRepo{}
	svc := NewService(repo, units)
	ctx := context.Background()

	s1 := "s1"
	repo.On("GetByID", ctx, "s1").Return(&Squad{ID: "s1", PlayerID: "p1", Status: "idle"}, nil)
	units.On("GetByPlayer", ctx, "p1").Return([]unit.Unit{
		{ID: "u1", PlayerID: "p1", SquadID: &s1, Status: "idle"},
		{ID: "u2", PlayerID: "p1", Status: "idle"}, // not in this squad
	}, nil)
	units.On("AssignSquad", ctx, "u1", mock.Anything).Return(nil)
	repo.On("Delete", ctx, "s1").Return(nil)

	err := svc.Disband(ctx, "s1", "p1")

	assert.NoError(t, err)
	units.AssertCalled(t, "AssignSquad", ctx, "u1", mock.Anything)
	units.AssertNotCalled(t, "AssignSquad", ctx, "u2", mock.Anything)
	repo.AssertCalled(t, "Delete", ctx, "s1")
}

func TestDisband_RejectsOnMission(t *testing.T) {
	repo := &mockSquadRepo{}
	units := &mockUnitRepo{}
	svc := NewService(repo, units)
	ctx := context.Background()

	repo.On("GetByID", ctx, "s1").Return(&Squad{ID: "s1", PlayerID: "p1", Status: "on_mission"}, nil)

	err := svc.Disband(ctx, "s1", "p1")

	var appErr *httputil.AppError
	assert.ErrorAs(t, err, &appErr)
	assert.Equal(t, "squad_busy", appErr.Code)
	repo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
}

func TestUpdateComposition_ReconcilesMembership(t *testing.T) {
	repo := &mockSquadRepo{}
	units := &mockUnitRepo{}
	svc := NewService(repo, units)
	ctx := context.Background()

	s1 := "s1"
	repo.On("GetByID", ctx, "s1").Return(&Squad{ID: "s1", PlayerID: "p1", Name: "Alpha", Status: "idle"}, nil)
	units.On("GetByPlayer", ctx, "p1").Return([]unit.Unit{
		{ID: "u1", PlayerID: "p1", SquadID: &s1, Status: "idle", Type: "fighter"}, // keep
		{ID: "u2", PlayerID: "p1", SquadID: &s1, Status: "idle", Type: "fighter"}, // remove
		{ID: "u3", PlayerID: "p1", Status: "idle", Type: "engineer"},              // add (free)
	}, nil)
	units.On("AssignSquad", ctx, "u2", mock.Anything).Return(nil) // freed
	units.On("AssignSquad", ctx, "u3", mock.Anything).Return(nil) // added

	out, err := svc.UpdateComposition(ctx, "s1", "p1", "Alpha", []string{"u1", "u3"})

	assert.NoError(t, err)
	assert.Equal(t, 2, out.UnitCount)
	units.AssertCalled(t, "AssignSquad", ctx, "u2", mock.Anything)
	units.AssertCalled(t, "AssignSquad", ctx, "u3", mock.Anything)
	units.AssertNotCalled(t, "AssignSquad", ctx, "u1", mock.Anything)
}

func TestUpdateComposition_RejectsUnitInAnotherSquad(t *testing.T) {
	repo := &mockSquadRepo{}
	units := &mockUnitRepo{}
	svc := NewService(repo, units)
	ctx := context.Background()

	other := "s2"
	repo.On("GetByID", ctx, "s1").Return(&Squad{ID: "s1", PlayerID: "p1", Name: "Alpha", Status: "idle"}, nil)
	units.On("GetByPlayer", ctx, "p1").Return([]unit.Unit{
		{ID: "u9", PlayerID: "p1", SquadID: &other, Status: "idle", Type: "fighter"},
	}, nil)

	_, err := svc.UpdateComposition(ctx, "s1", "p1", "", []string{"u9"})

	var appErr *httputil.AppError
	assert.ErrorAs(t, err, &appErr)
	assert.Equal(t, "unit_assigned", appErr.Code)
}

func TestCompleteDueMissions_PatrolGrantsRewardCleansesAndIdles(t *testing.T) {
	repo := &mockSquadRepo{}
	units := &mockUnitRepo{}
	svc := NewService(repo, units)
	res := &fakeResAdder{}
	cells := &fakeMissionCells{}
	notif := &fakeMissionNotifier{}
	svc.SetMissionDeps(res, cells, notif)
	ctx := context.Background()

	due := []Squad{{
		ID: "s1", PlayerID: "p1", Status: "on_mission",
		Mission: &Mission{Type: "patrol", TargetID: "c1", ReturnsAt: time.Now().Add(-time.Minute)},
	}}
	repo.On("GetDueMissions", ctx, mock.Anything).Return(due, nil)
	repo.On("Update", ctx, mock.AnythingOfType("*squad.Squad")).Return(nil)

	err := svc.CompleteDueMissions(ctx)

	assert.NoError(t, err)
	assert.Equal(t, canon.SquadPatrolEnergy, res.energy)
	assert.Equal(t, canon.SquadPatrolMaterials, res.materials)
	assert.True(t, cells.cleansed, "patrol should cleanse infection at the target")
	assert.True(t, notif.called)
	assert.Equal(t, "patrol", notif.mtype)
}

func TestCompleteDueMissions_CaptureGrantsBiggerHaul(t *testing.T) {
	repo := &mockSquadRepo{}
	units := &mockUnitRepo{}
	svc := NewService(repo, units)
	res := &fakeResAdder{}
	svc.SetMissionDeps(res, &fakeMissionCells{}, &fakeMissionNotifier{})
	ctx := context.Background()

	due := []Squad{{
		ID: "s2", PlayerID: "p1", Status: "on_mission",
		Mission: &Mission{Type: "capture", TargetID: "c2", ReturnsAt: time.Now().Add(-time.Minute)},
	}}
	repo.On("GetDueMissions", ctx, mock.Anything).Return(due, nil)
	repo.On("Update", ctx, mock.AnythingOfType("*squad.Squad")).Return(nil)

	err := svc.CompleteDueMissions(ctx)

	assert.NoError(t, err)
	assert.Equal(t, canon.SquadCaptureEnergy, res.energy)
	assert.Equal(t, canon.SquadCaptureMaterials, res.materials)
}

func TestCreate_RejectsBusyUnit(t *testing.T) {
	repo := &mockSquadRepo{}
	units := &mockUnitRepo{}
	svc := NewService(repo, units)
	ctx := context.Background()

	units.On("GetByID", ctx, "u1").Return(&unit.Unit{ID: "u1", PlayerID: "p1", Status: "recovering"}, nil)

	_, err := svc.Create(ctx, "p1", "Alpha", []string{"u1"})

	var appErr *httputil.AppError
	assert.ErrorAs(t, err, &appErr)
	assert.Equal(t, "unit_busy", appErr.Code)
}
