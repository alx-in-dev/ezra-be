package quest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/pkg/httputil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// --- Mock QuestRepository ---

type MockQuestRepository struct {
	mock.Mock
}

func (m *MockQuestRepository) Create(ctx context.Context, q *DailyQuest) error {
	args := m.Called(ctx, q)
	if args.Error(0) == nil {
		q.ID = "quest-" + q.Type // simulate DB-assigned ID
	}
	return args.Error(0)
}

func (m *MockQuestRepository) GetByPlayerAndDate(ctx context.Context, playerID string, date time.Time) ([]DailyQuest, error) {
	args := m.Called(ctx, playerID, date)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]DailyQuest), args.Error(1)
}

func (m *MockQuestRepository) UpdateProgress(ctx context.Context, id string, progress int) error {
	args := m.Called(ctx, id, progress)
	return args.Error(0)
}

func (m *MockQuestRepository) Claim(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockQuestRepository) GetByID(ctx context.Context, id string) (*DailyQuest, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*DailyQuest), args.Error(1)
}

// --- Mock player.Repository ---

type MockPlayerRepository struct {
	mock.Mock
}

func (m *MockPlayerRepository) Create(ctx context.Context, p *player.Player) error {
	args := m.Called(ctx, p)
	return args.Error(0)
}

func (m *MockPlayerRepository) GetByID(ctx context.Context, id string) (*player.Player, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*player.Player), args.Error(1)
}

func (m *MockPlayerRepository) GetAll(ctx context.Context) ([]player.Player, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]player.Player), args.Error(1)
}

func (m *MockPlayerRepository) GetByFirebaseUID(ctx context.Context, uid string) (*player.Player, error) {
	args := m.Called(ctx, uid)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*player.Player), args.Error(1)
}

func (m *MockPlayerRepository) GetByLogin(ctx context.Context, login string) (*player.Player, error) {
	args := m.Called(ctx, login)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*player.Player), args.Error(1)
}

func (m *MockPlayerRepository) CreateWithLogin(ctx context.Context, p *player.Player) error {
	args := m.Called(ctx, p)
	return args.Error(0)
}

func (m *MockPlayerRepository) Update(ctx context.Context, p *player.Player) error {
	args := m.Called(ctx, p)
	return args.Error(0)
}

func (m *MockPlayerRepository) UpdatePosition(ctx context.Context, id string, lat, lng float64) error {
	args := m.Called(ctx, id, lat, lng)
	return args.Error(0)
}

func (m *MockPlayerRepository) UpdateLastActive(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockPlayerRepository) SetQuickStartHuman(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// --- Tests ---

func TestGenerate3Daily_NewDay(t *testing.T) {
	questRepo := new(MockQuestRepository)
	playerRepo := new(MockPlayerRepository)
	resourceSvc := player.NewResourceService(playerRepo)
	svc := NewQuestService(questRepo, resourceSvc)
	ctx := context.Background()

	today := time.Now().Truncate(24 * time.Hour)

	// No quests exist for today
	questRepo.On("GetByPlayerAndDate", ctx, "p1", today).Return([]DailyQuest(nil), nil)
	questRepo.On("Create", ctx, mock.AnythingOfType("*quest.DailyQuest")).Return(nil).Times(3)

	quests, err := svc.Generate3Daily(ctx, "p1")

	assert.NoError(t, err)
	assert.Len(t, quests, 3)
	for _, q := range quests {
		assert.Equal(t, "p1", q.PlayerID)
		assert.Equal(t, today, q.Date)
		assert.False(t, q.Claimed)
		assert.Equal(t, 0, q.Progress)
		assert.NotEmpty(t, q.ID)
	}
	questRepo.AssertExpectations(t)
}

func TestGenerate3Daily_AlreadyExists(t *testing.T) {
	questRepo := new(MockQuestRepository)
	playerRepo := new(MockPlayerRepository)
	resourceSvc := player.NewResourceService(playerRepo)
	svc := NewQuestService(questRepo, resourceSvc)
	ctx := context.Background()

	today := time.Now().Truncate(24 * time.Hour)

	existing := []DailyQuest{
		{ID: "q1", PlayerID: "p1", Type: "walk_500m", Date: today},
		{ID: "q2", PlayerID: "p1", Type: "place_beacon", Date: today},
		{ID: "q3", PlayerID: "p1", Type: "win_3_battles", Date: today},
	}
	questRepo.On("GetByPlayerAndDate", ctx, "p1", today).Return(existing, nil)

	quests, err := svc.Generate3Daily(ctx, "p1")

	assert.NoError(t, err)
	assert.Len(t, quests, 3)
	assert.Equal(t, "q1", quests[0].ID)
	// Create should NOT have been called
	questRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	questRepo.AssertExpectations(t)
}

func TestUpdateProgress_IncrementsExistingQuest(t *testing.T) {
	questRepo := new(MockQuestRepository)
	playerRepo := new(MockPlayerRepository)
	svc := NewQuestService(questRepo, player.NewResourceService(playerRepo))
	ctx := context.Background()
	today := time.Now().Truncate(24 * time.Hour)

	existing := []DailyQuest{
		{ID: "q1", PlayerID: "p1", Type: "place_beacon", Progress: 0, Target: 1, Date: today},
		{ID: "q2", PlayerID: "p1", Type: "send_pet", Progress: 0, Target: 1, Date: today},
	}
	// Returned both for the ensure (Generate3Daily) and the increment lookup.
	questRepo.On("GetByPlayerAndDate", ctx, "p1", today).Return(existing, nil)
	questRepo.On("UpdateProgress", ctx, "q1", 1).Return(nil)

	err := svc.UpdateProgress(ctx, "p1", "place_beacon")

	assert.NoError(t, err)
	questRepo.AssertCalled(t, "UpdateProgress", ctx, "q1", 1)
	questRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestUpdateProgress_GeneratesDailiesIfMissing(t *testing.T) {
	questRepo := new(MockQuestRepository)
	playerRepo := new(MockPlayerRepository)
	svc := NewQuestService(questRepo, player.NewResourceService(playerRepo))
	ctx := context.Background()
	today := time.Now().Truncate(24 * time.Hour)

	// No quests yet — the event fires before the player ever opened the quests
	// tab (e.g. placing the starter beacon during onboarding). The fix: ensure
	// today's set is generated so the progress isn't silently lost.
	questRepo.On("GetByPlayerAndDate", ctx, "p1", today).Return([]DailyQuest(nil), nil)
	questRepo.On("Create", ctx, mock.AnythingOfType("*quest.DailyQuest")).Return(nil).Times(3)

	err := svc.UpdateProgress(ctx, "p1", "place_beacon")

	assert.NoError(t, err)
	questRepo.AssertNumberOfCalls(t, "Create", 3) // dailies were generated, not lost
}

func TestClaim_Success(t *testing.T) {
	questRepo := new(MockQuestRepository)
	playerRepo := new(MockPlayerRepository)
	resourceSvc := player.NewResourceService(playerRepo)
	svc := NewQuestService(questRepo, resourceSvc)
	ctx := context.Background()

	q := &DailyQuest{
		ID:       "q1",
		PlayerID: "p1",
		Type:     "walk_500m",
		Progress: 500,
		Target:   500,
		Reward:   Reward{Energy: 30, Materials: 0},
		Claimed:  false,
	}
	questRepo.On("GetByID", ctx, "q1").Return(q, nil)
	questRepo.On("Claim", ctx, "q1").Return(nil)

	p := &player.Player{ID: "p1", Energy: 100, Materials: 50}
	playerRepo.On("GetByID", ctx, "p1").Return(p, nil)
	playerRepo.On("Update", ctx, p).Return(nil)

	result, err := svc.Claim(ctx, "q1", "p1")

	assert.NoError(t, err)
	assert.True(t, result.Claimed)
	assert.Equal(t, 130, p.Energy) // 100 + 30
	questRepo.AssertExpectations(t)
	playerRepo.AssertExpectations(t)
}

func TestClaim_NotCompleted(t *testing.T) {
	questRepo := new(MockQuestRepository)
	playerRepo := new(MockPlayerRepository)
	resourceSvc := player.NewResourceService(playerRepo)
	svc := NewQuestService(questRepo, resourceSvc)
	ctx := context.Background()

	q := &DailyQuest{
		ID:       "q1",
		PlayerID: "p1",
		Progress: 1,
		Target:   3,
		Claimed:  false,
	}
	questRepo.On("GetByID", ctx, "q1").Return(q, nil)

	_, err := svc.Claim(ctx, "q1", "p1")

	assert.Error(t, err)
	var appErr *httputil.AppError
	assert.True(t, errors.As(err, &appErr))
	assert.Equal(t, "not_complete", appErr.Code)
}

func TestClaim_AlreadyClaimed(t *testing.T) {
	questRepo := new(MockQuestRepository)
	playerRepo := new(MockPlayerRepository)
	resourceSvc := player.NewResourceService(playerRepo)
	svc := NewQuestService(questRepo, resourceSvc)
	ctx := context.Background()

	q := &DailyQuest{
		ID:       "q1",
		PlayerID: "p1",
		Progress: 3,
		Target:   3,
		Claimed:  true,
	}
	questRepo.On("GetByID", ctx, "q1").Return(q, nil)

	_, err := svc.Claim(ctx, "q1", "p1")

	assert.Error(t, err)
	var appErr *httputil.AppError
	assert.True(t, errors.As(err, &appErr))
	assert.Equal(t, "already_claimed", appErr.Code)
}
