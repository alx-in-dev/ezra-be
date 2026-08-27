package faction

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

type fakeRepo struct {
	faction   string
	contacted bool
	chosen    bool
}

func (f *fakeRepo) Get(ctx context.Context, p string) (string, bool, bool, error) {
	fac := f.faction
	if fac == "" {
		fac = Human
	}
	return fac, f.contacted, f.chosen, nil
}
func (f *fakeRepo) SetContacted(ctx context.Context, p string) error { f.contacted = true; return nil }
func (f *fakeRepo) Choose(ctx context.Context, p, side string) error {
	f.faction = side
	f.chosen = true
	return nil
}
func (f *fakeRepo) SetFaction(ctx context.Context, p, side string, chosen bool) error {
	f.faction = side
	f.contacted = true
	f.chosen = chosen
	return nil
}

func TestStatus_DefaultHumanUncontacted(t *testing.T) {
	svc := NewService(&fakeRepo{})
	st, err := svc.Status(context.Background(), "p1")
	assert.NoError(t, err)
	assert.Equal(t, Human, st.Faction)
	assert.False(t, st.Contacted)
	assert.False(t, st.CanChoose)
	assert.Equal(t, "Люди", st.FactionRu)
}

func TestChoose_RequiresContact(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{}
	svc := NewService(repo)

	// Not contacted → choice refused.
	_, err := svc.Choose(ctx, "p1", Symbiont)
	assert.Error(t, err)

	// Invalid side → refused even after contact.
	repo.contacted = true
	_, err = svc.Choose(ctx, "p1", "alien")
	assert.Error(t, err)

	// Contacted + valid → chosen.
	st, err := svc.Choose(ctx, "p1", Symbiont)
	assert.NoError(t, err)
	assert.Equal(t, Symbiont, st.Faction)
	assert.True(t, st.Chosen)
	assert.Equal(t, "Симбионты", st.FactionRu)
}

func TestMarkContacted_UnlocksChoice(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{}
	svc := NewService(repo)
	assert.NoError(t, svc.MarkContacted(ctx, "p1"))
	st, _ := svc.Status(ctx, "p1")
	assert.True(t, st.Contacted)
	assert.True(t, st.CanChoose)
}

// --- Symbiont onboarding bridge (docs/feature/symbiont_onboarding.md) ---

type fakeRoster struct{ seededSide string }

func (f *fakeRoster) OnFactionChosen(ctx context.Context, playerID, side string) error {
	f.seededSide = side
	return nil
}

type fakeFinisher struct{ called bool }

func (f *fakeFinisher) FinishFactionChoice(ctx context.Context, playerID string) error {
	f.called = true
	return nil
}

func TestEnterTemporarySymbiont(t *testing.T) {
	repo := &fakeRepo{}
	roster := &fakeRoster{}
	svc := NewService(repo)
	svc.SetRosterInitializer(roster)

	err := svc.EnterTemporarySymbiont(context.Background(), "p1")
	assert.NoError(t, err)
	assert.Equal(t, Symbiont, repo.faction)
	assert.True(t, repo.contacted)
	assert.False(t, repo.chosen, "temporary symbiont is not a final choice")
	// The one-shot Pool conversion must NOT fire during the tutorial (the
	// player has no progress yet — seeding here froze every account at the
	// floor value forever). It fires on the first real Choose(symbiont).
	assert.Equal(t, "", roster.seededSide, "tutorial must not seed the pool")
}

func TestChoose_FinishesOnboarding(t *testing.T) {
	repo := &fakeRepo{contacted: true}
	fin := &fakeFinisher{}
	svc := NewService(repo)
	svc.SetOnboardingFinisher(fin)

	_, err := svc.Choose(context.Background(), "p1", Human)
	assert.NoError(t, err)
	assert.Equal(t, Human, repo.faction, "returning to human reverts the temporary symbiont")
	assert.True(t, repo.chosen)
	assert.True(t, fin.called, "onboarding finisher invoked on choice")
}

func TestChooseInstant_SkipsContactAndCooldown(t *testing.T) {
	// No Contact, no prior faction row — ChooseInstant must not require either
	// (docs/feature/onboarding_quick_start.md: it runs before any exists).
	repo := &fakeRepo{}
	svc := NewService(repo)

	st, err := svc.ChooseInstant(context.Background(), "p1")
	assert.NoError(t, err)
	assert.Equal(t, Symbiont, repo.faction)
	assert.True(t, repo.chosen)
	assert.Equal(t, Symbiont, st.Faction)
	assert.True(t, st.Chosen)
}

func TestChooseInstant_SeedsRoster(t *testing.T) {
	repo := &fakeRepo{}
	roster := &fakeRoster{}
	svc := NewService(repo)
	svc.SetRosterInitializer(roster)

	_, err := svc.ChooseInstant(context.Background(), "p1")
	assert.NoError(t, err)
	assert.Equal(t, Symbiont, roster.seededSide)
}
