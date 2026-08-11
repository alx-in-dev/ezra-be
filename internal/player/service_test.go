package player

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/ezra-game/server/internal/canon"
	"github.com/ezra-game/server/pkg/httputil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// --- Mock Repository ---

type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) Create(ctx context.Context, p *Player) error {
	args := m.Called(ctx, p)
	return args.Error(0)
}

func (m *MockRepository) GetByID(ctx context.Context, id string) (*Player, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Player), args.Error(1)
}

func (m *MockRepository) GetAll(ctx context.Context) ([]Player, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]Player), args.Error(1)
}

func (m *MockRepository) GetByFirebaseUID(ctx context.Context, uid string) (*Player, error) {
	args := m.Called(ctx, uid)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Player), args.Error(1)
}

func (m *MockRepository) GetByLogin(ctx context.Context, login string) (*Player, error) {
	args := m.Called(ctx, login)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Player), args.Error(1)
}

func (m *MockRepository) CreateWithLogin(ctx context.Context, p *Player) error {
	args := m.Called(ctx, p)
	return args.Error(0)
}

func (m *MockRepository) Update(ctx context.Context, p *Player) error {
	args := m.Called(ctx, p)
	return args.Error(0)
}

func (m *MockRepository) UpdatePosition(ctx context.Context, id string, lat, lng float64) error {
	args := m.Called(ctx, id, lat, lng)
	return args.Error(0)
}

func (m *MockRepository) UpdateLastActive(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// --- ResourceService Tests ---

func TestSpend_Success(t *testing.T) {
	repo := new(MockRepository)
	svc := NewResourceService(repo)
	ctx := context.Background()

	p := &Player{ID: "p1", Energy: 500, Materials: 100}
	repo.On("GetByID", ctx, "p1").Return(p, nil)
	repo.On("Update", ctx, p).Return(nil)

	result, err := svc.Spend(ctx, "p1", 100, 20)

	assert.NoError(t, err)
	assert.Equal(t, 400, result.Energy)
	assert.Equal(t, 80, result.Materials)
	repo.AssertExpectations(t)
}

func TestSpend_InsufficientEnergy(t *testing.T) {
	repo := new(MockRepository)
	svc := NewResourceService(repo)
	ctx := context.Background()

	p := &Player{ID: "p1", Energy: 30, Materials: 100}
	repo.On("GetByID", ctx, "p1").Return(p, nil)

	_, err := svc.Spend(ctx, "p1", 50, 10)

	assert.Error(t, err)
	var appErr *httputil.AppError
	assert.True(t, errors.As(err, &appErr))
	assert.Equal(t, "insufficient_resources", appErr.Code)
}

func TestSpend_InsufficientMaterials(t *testing.T) {
	repo := new(MockRepository)
	svc := NewResourceService(repo)
	ctx := context.Background()

	p := &Player{ID: "p1", Energy: 100, Materials: 5}
	repo.On("GetByID", ctx, "p1").Return(p, nil)

	_, err := svc.Spend(ctx, "p1", 10, 10)

	assert.Error(t, err)
	var appErr *httputil.AppError
	assert.True(t, errors.As(err, &appErr))
	assert.Equal(t, "insufficient_resources", appErr.Code)
}

func TestAdd_Success(t *testing.T) {
	repo := new(MockRepository)
	svc := NewResourceService(repo)
	ctx := context.Background()

	p := &Player{ID: "p1", Energy: 100, Materials: 50}
	repo.On("GetByID", ctx, "p1").Return(p, nil)
	repo.On("Update", ctx, p).Return(nil)

	result, err := svc.Add(ctx, "p1", 500, 0)

	assert.NoError(t, err)
	assert.Equal(t, 600, result.Energy)
	repo.AssertExpectations(t)
}

func TestEnergyMax_ScalesWithLevelAndCoversTopUpgrade(t *testing.T) {
	// C6 (anti-P2W): storage grows with level, so a level-15 F2P can bank the
	// 3200⚡ L5 beacon without buying storage.
	base := (&Player{Level: 1}).EnergyMax()
	assert.Equal(t, MaxEnergy, base)

	mid := (&Player{Level: 15}).EnergyMax()
	assert.Equal(t, MaxEnergy+14*EnergyCapPerLevel, mid)
	assert.GreaterOrEqual(t, mid, 3200, "max-level F2P must afford the L5 beacon")

	// Unset/zero level is treated as level 1 (no negative allowance).
	assert.Equal(t, MaxEnergy, (&Player{}).EnergyMax())

	// Shop bonus stacks on top of the level allowance.
	withShop := (&Player{Level: 10, StorageBonusEnergy: 500}).EnergyMax()
	assert.Equal(t, MaxEnergy+9*EnergyCapPerLevel+500, withShop)
}

func TestAdd_EnergyCapped(t *testing.T) {
	repo := new(MockRepository)
	svc := NewResourceService(repo)
	ctx := context.Background()

	p := &Player{ID: "p1", Energy: 1800, Materials: 50}
	repo.On("GetByID", ctx, "p1").Return(p, nil)
	repo.On("Update", ctx, p).Return(nil)

	result, err := svc.Add(ctx, "p1", 500, 0)

	assert.NoError(t, err)
	assert.Equal(t, MaxEnergy, result.Energy)
	repo.AssertExpectations(t)
}

func TestAdd_MaterialsCapped(t *testing.T) {
	repo := new(MockRepository)
	svc := NewResourceService(repo)
	ctx := context.Background()

	p := &Player{ID: "p1", Energy: 100, Materials: 400}
	repo.On("GetByID", ctx, "p1").Return(p, nil)
	repo.On("Update", ctx, p).Return(nil)

	result, err := svc.Add(ctx, "p1", 0, 200)

	assert.NoError(t, err)
	assert.Equal(t, MaxMaterials, result.Materials)
	repo.AssertExpectations(t)
}

// --- Service.AddXP Tests ---

func TestAddXP_NoLevelUp(t *testing.T) {
	repo := new(MockRepository)
	svc := NewService(repo)
	ctx := context.Background()

	// xpForNextLevel(1) = 100 * 1^1.6 = 100
	p := &Player{ID: "p1", Level: 1, XP: 50}
	repo.On("GetByID", ctx, "p1").Return(p, nil)
	repo.On("Update", ctx, p).Return(nil)

	result, err := svc.AddXP(ctx, "p1", 20)

	assert.NoError(t, err)
	assert.Equal(t, 70, result.XP)
	assert.Equal(t, 1, result.Level)
	repo.AssertExpectations(t)
}

func TestAddXP_LevelUp(t *testing.T) {
	repo := new(MockRepository)
	svc := NewService(repo)
	ctx := context.Background()

	// xpForNextLevel(1) = 100; 90+20=110 >= 100 → level 2, xp = 10
	p := &Player{ID: "p1", Level: 1, XP: 90}
	repo.On("GetByID", ctx, "p1").Return(p, nil)
	repo.On("Update", ctx, p).Return(nil)

	result, err := svc.AddXP(ctx, "p1", 20)

	assert.NoError(t, err)
	assert.Equal(t, 10, result.XP)
	assert.Equal(t, 2, result.Level)
	repo.AssertExpectations(t)
}

func TestAddXP_MultiLevelUp(t *testing.T) {
	repo := new(MockRepository)
	svc := NewService(repo)
	ctx := context.Background()

	p := &Player{ID: "p1", Level: 1, XP: 0}
	repo.On("GetByID", ctx, "p1").Return(p, nil)
	repo.On("Update", ctx, p).Return(nil)

	// Simulate the level-up loop to compute expected values.
	xp := 5000
	level := 1
	for xp >= int(100*math.Pow(float64(level), 1.6)) {
		xp -= int(100 * math.Pow(float64(level), 1.6))
		level++
	}

	result, err := svc.AddXP(ctx, "p1", 5000)

	assert.NoError(t, err)
	assert.Equal(t, level, result.Level)
	assert.Equal(t, xp, result.XP)
	assert.True(t, result.Level > 2, "should have leveled up multiple times")
	repo.AssertExpectations(t)
}

func TestAddXP_StopsAtMVPLevelCap(t *testing.T) {
	repo := new(MockRepository)
	svc := NewService(repo)
	ctx := context.Background()

	p := &Player{ID: "p1", Level: 15, XP: 700}
	repo.On("GetByID", ctx, "p1").Return(p, nil)
	repo.On("Update", ctx, p).Return(nil)

	result, err := svc.AddXP(ctx, "p1", 10000)

	assert.NoError(t, err)
	assert.Equal(t, 15, result.Level)
	assert.Equal(t, XPForNextLevel(15), result.XP)
	repo.AssertExpectations(t)
}

func TestUnlockSkill_SucceedsForNextTier(t *testing.T) {
	repo := new(MockRepository)
	svc := NewService(repo)
	ctx := context.Background()

	p := &Player{
		ID:    "p1",
		Level: 3,
		Skills: SkillPoints{
			Defender: 1,
		},
	}
	repo.On("GetByID", ctx, "p1").Return(p, nil)
	repo.On("Update", ctx, p).Return(nil)

	updated, skill, err := svc.UnlockSkill(ctx, "p1", "defender_2")

	assert.NoError(t, err)
	assert.Equal(t, "defender_2", skill.ID)
	assert.Equal(t, 2, updated.Skills.Defender)
	repo.AssertExpectations(t)
}

func TestUnlockSkill_RequiresAvailablePoint(t *testing.T) {
	repo := new(MockRepository)
	svc := NewService(repo)
	ctx := context.Background()

	p := &Player{
		ID:    "p1",
		Level: 2,
		Skills: SkillPoints{
			Defender: 1,
		},
	}
	repo.On("GetByID", ctx, "p1").Return(p, nil)

	_, _, err := svc.UnlockSkill(ctx, "p1", "commander_1")

	assert.Error(t, err)
	var appErr *httputil.AppError
	assert.True(t, errors.As(err, &appErr))
	assert.Equal(t, "no_skill_points", appErr.Code)
}

func TestUpdateUsername_SetsCustomFlag(t *testing.T) {
	repo := new(MockRepository)
	svc := NewService(repo)
	ctx := context.Background()

	p := &Player{ID: "p1", Username: "old_name", UsernameIsCustom: false}
	repo.On("GetByID", ctx, "p1").Return(p, nil)
	repo.On("Update", ctx, p).Return(nil)

	result, err := svc.UpdateUsername(ctx, "p1", "new_name")

	assert.NoError(t, err)
	assert.Equal(t, "new_name", result.Username)
	assert.True(t, result.UsernameIsCustom)
	repo.AssertExpectations(t)
}

// --- Symbiont onboarding step machine (docs/feature/symbiont_onboarding.md) ---

func TestAdvanceOnboarding_SymbiontTutorialChain(t *testing.T) {
	ctx := context.Background()
	cases := []struct{ from, action, want string }{
		{canon.OnboardingSymbiontIntro, "complete_symbiont_intro", canon.OnboardingSymbiontVerb},
		{canon.OnboardingSymbiontVerb, "complete_symbiont_verb", canon.OnboardingSymbiontEntity},
		{canon.OnboardingSymbiontEntity, "complete_symbiont_entity", canon.OnboardingFactionChoice},
	}
	for _, c := range cases {
		repo := new(MockRepository)
		svc := NewService(repo)
		p := &Player{ID: "p1", OnboardingStep: c.from}
		repo.On("GetByID", ctx, "p1").Return(p, nil)
		repo.On("Update", ctx, mock.MatchedBy(func(pp *Player) bool {
			return pp.OnboardingStep == c.want
		})).Return(nil)

		got, err := svc.AdvanceOnboarding(ctx, "p1", c.action)
		assert.NoError(t, err)
		assert.Equal(t, c.want, got.OnboardingStep, "from %s via %s", c.from, c.action)
		repo.AssertExpectations(t)
	}
}

func TestAdvanceOnboarding_SymbiontActionIgnoredOffStep(t *testing.T) {
	ctx := context.Background()
	repo := new(MockRepository)
	svc := NewService(repo)
	// Wrong step: action must not advance, but Update is still called (no-op write).
	p := &Player{ID: "p1", OnboardingStep: canon.OnboardingLoreStep}
	repo.On("GetByID", ctx, "p1").Return(p, nil)
	repo.On("Update", ctx, mock.Anything).Return(nil)

	got, err := svc.AdvanceOnboarding(ctx, "p1", "complete_symbiont_verb")
	assert.NoError(t, err)
	assert.Equal(t, canon.OnboardingLoreStep, got.OnboardingStep)
}

func TestFinishFactionChoice_CompletesOnlyFromChoiceStep(t *testing.T) {
	ctx := context.Background()

	// On the choice step → completes.
	repo := new(MockRepository)
	svc := NewService(repo)
	p := &Player{ID: "p1", OnboardingStep: canon.OnboardingFactionChoice}
	repo.On("GetByID", ctx, "p1").Return(p, nil)
	repo.On("Update", ctx, mock.MatchedBy(func(pp *Player) bool {
		return pp.OnboardingStep == canon.OnboardingCompleted
	})).Return(nil)
	assert.NoError(t, svc.FinishFactionChoice(ctx, "p1"))
	repo.AssertExpectations(t)

	// Already completed (later side-switch) → no Update, stays completed.
	repo2 := new(MockRepository)
	svc2 := NewService(repo2)
	p2 := &Player{ID: "p2", OnboardingStep: canon.OnboardingCompleted}
	repo2.On("GetByID", ctx, "p2").Return(p2, nil)
	assert.NoError(t, svc2.FinishFactionChoice(ctx, "p2"))
	repo2.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
}
