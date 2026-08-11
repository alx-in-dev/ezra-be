package spire

import (
	"context"
	"testing"
	"time"

	"github.com/ezra-game/server/internal/item"
	"github.com/ezra-game/server/internal/player"
	"github.com/stretchr/testify/assert"
)

func TestRegionKeyAndCenter(t *testing.T) {
	k1 := RegionKeyFor(54.7354, 20.5521)
	k2 := RegionKeyFor(54.73, 20.55) // same 0.1° bucket
	assert.Equal(t, k1, k2)
	clat, clng := RegionCenter(54.7354, 20.5521)
	assert.InDelta(t, 54.75, clat, 0.0001)
	assert.InDelta(t, 20.55, clng, 0.0001)
}

func TestTargetForPopulation(t *testing.T) {
	assert.Equal(t, MinBuildTarget, TargetForPopulation(0)) // floor
	assert.Equal(t, MinBuildTarget, TargetForPopulation(4)) // 4*300=1200 < floor
	assert.Equal(t, 6000, TargetForPopulation(20))          // 20*300
}

func TestTierFor(t *testing.T) {
	// target 6000, P_zone 20 → fair share = 300.
	assert.Equal(t, "", TierFor(0, 6000, 20))
	assert.Equal(t, TierBronze, TierFor(100, 6000, 20)) // < fair
	assert.Equal(t, TierSilver, TierFor(300, 6000, 20)) // = fair
	assert.Equal(t, TierGold, TierFor(700, 6000, 20))   // >= 2*fair
}

type fakeRepo struct {
	sp         *Spire
	maintained int
	contribs   map[string]*Contribution
	expired    []Spire
	reset      bool
	pZoneCount int
}

func newFakeRepo(sp *Spire) *fakeRepo {
	return &fakeRepo{sp: sp, contribs: map[string]*Contribution{}}
}

func (f *fakeRepo) GetByRegion(ctx context.Context, key string) (*Spire, error) { return f.sp, nil }
func (f *fakeRepo) Create(ctx context.Context, s *Spire) error {
	s.ID = "sp1"
	f.sp = s
	return nil
}
func (f *fakeRepo) AddFragment(ctx context.Context, id, p string) (int, error) {
	f.sp.Fragments++
	return f.sp.Fragments, nil
}
func (f *fakeRepo) SetState(ctx context.Context, id, st string) error { f.sp.State = st; return nil }
func (f *fakeRepo) OpenBuild(ctx context.Context, id string, target, pZone int, deadline time.Time) error {
	f.sp.State = "building"
	f.sp.BuildTarget = target
	f.sp.PZone = pZone
	f.sp.BuildDeadline = &deadline
	return nil
}
func (f *fakeRepo) CountActivePlayers(ctx context.Context, lat, lng float64, radiusM, days int) (int, error) {
	return f.pZoneCount, nil
}
func (f *fakeRepo) AddResources(ctx context.Context, id, p string, amt int) (int, error) {
	f.sp.BuildProgress += amt
	c := f.contribs[p]
	if c == nil {
		c = &Contribution{PlayerID: p}
		f.contribs[p] = c
	}
	c.Resources += amt
	return f.sp.BuildProgress, nil
}
func (f *fakeRepo) Activate(ctx context.Context, id string) error { f.sp.State = "active"; return nil }
func (f *fakeRepo) Contribution(ctx context.Context, id, p string) (int, int, string, error) {
	if c := f.contribs[p]; c != nil {
		return c.Fragments, c.Resources, c.Tier, nil
	}
	return 0, 0, "", nil
}
func (f *fakeRepo) ListContributions(ctx context.Context, id string) ([]Contribution, error) {
	out := make([]Contribution, 0, len(f.contribs))
	for _, c := range f.contribs {
		out = append(out, *c)
	}
	return out, nil
}
func (f *fakeRepo) SetTier(ctx context.Context, id, p, tier string) error {
	if c := f.contribs[p]; c != nil {
		c.Tier = tier
	}
	return nil
}
func (f *fakeRepo) Maintain(ctx context.Context, id, p string, amt int) error {
	if f.sp.State == "degrading" {
		f.sp.State = "active"
	}
	f.maintained++
	return nil
}
func (f *fakeRepo) TransitionStale(ctx context.Context, deg, ru time.Time) (int64, error) {
	return 0, nil
}
func (f *fakeRepo) ExpiredBuilds(ctx context.Context, now time.Time) ([]Spire, error) {
	return f.expired, nil
}
func (f *fakeRepo) ResetForRefund(ctx context.Context, id string) error {
	f.reset = true
	f.contribs = map[string]*Contribution{}
	if f.sp != nil {
		f.sp.State = "assembling"
	}
	return nil
}
func (f *fakeRepo) Rebuild(ctx context.Context, id string) error {
	f.sp.State = "assembling"
	f.sp.Fragments = 0
	return nil
}

type fakeItems struct {
	frags   int
	granted map[string]int // itemType+variant → qty
}

func (f *fakeItems) ListByPlayer(ctx context.Context, p string) ([]item.Item, error) {
	return []item.Item{{ItemType: item.TypeBlueprintFragment, Qty: f.frags}}, nil
}
func (f *fakeItems) Grant(ctx context.Context, p, t, v string, d int) (int, error) {
	if t == item.TypeBlueprintFragment {
		f.frags += d
		return f.frags, nil
	}
	if f.granted == nil {
		f.granted = map[string]int{}
	}
	f.granted[t+":"+v] += d
	return f.granted[t+":"+v], nil
}

type fakeRes struct{ refunded map[string]int }

func (fakeRes) Spend(ctx context.Context, p string, e, m int) (*player.Player, error) {
	return &player.Player{}, nil
}
func (f *fakeRes) Add(ctx context.Context, p string, e, m int) (*player.Player, error) {
	if f.refunded == nil {
		f.refunded = map[string]int{}
	}
	f.refunded[p] += e
	return &player.Player{}, nil
}

func TestFragmentsOpenBuild_SizesTargetFromPopulation(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo(nil)
	repo.pZoneCount = 20 // → target 6000
	items := &fakeItems{frags: 5}
	svc := NewService(repo, items, &fakeRes{})

	for i := 0; i < 2; i++ {
		st, err := svc.ContributeFragment(ctx, "p1", 54.7, 20.5)
		assert.NoError(t, err)
		assert.Equal(t, "assembling", st.Spire.State)
	}
	st, err := svc.ContributeFragment(ctx, "p1", 54.7, 20.5)
	assert.NoError(t, err)
	assert.Equal(t, "building", st.Spire.State)
	assert.Equal(t, 3, st.Spire.Fragments)
	assert.Equal(t, 20, st.Spire.PZone)
	assert.Equal(t, 6000, st.Spire.BuildTarget) // sized from P_zone
	assert.NotNil(t, st.Spire.BuildDeadline)    // escrow window opened
}

func TestResourcesActivate_AwardsTiers(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo(&Spire{ID: "sp1", RegionKey: "k", State: "building", BuildTarget: 500, PZone: 1})
	items := &fakeItems{}
	svc := NewService(repo, items, &fakeRes{})
	// 250 + 250 = 500 >= target → active.
	_, _ = svc.ContributeResources(ctx, "p1", 54.7, 20.5)
	st, err := svc.ContributeResources(ctx, "p1", 54.7, 20.5)
	assert.NoError(t, err)
	assert.Equal(t, "active", st.Spire.State)
	// p1 contributed full target with P_zone=1 (fair=500) → gold (>=2*fair? 500>=1000 no → silver).
	assert.Equal(t, TierSilver, st.YourTier)
	assert.Equal(t, 1, items.granted["season_title:spire_silver"])
}

func TestEscrowExpiry_RefundsAndResets(t *testing.T) {
	ctx := context.Background()
	past := time.Now().Add(-time.Hour)
	sp := Spire{ID: "sp1", RegionKey: "k", State: "building", BuildTarget: 6000, BuildProgress: 500, BuildDeadline: &past}
	repo := newFakeRepo(&sp)
	repo.expired = []Spire{sp}
	repo.contribs["p1"] = &Contribution{PlayerID: "p1", Resources: 500, Fragments: 2}
	repo.contribs["p2"] = &Contribution{PlayerID: "p2", Fragments: 1}
	items := &fakeItems{}
	res := &fakeRes{}
	svc := NewService(repo, items, res)

	err := svc.LifecycleTick(ctx)
	assert.NoError(t, err)
	assert.True(t, repo.reset)                  // spire reset to assembling
	assert.Equal(t, 500, res.refunded["p1"])    // energy returned
	assert.Equal(t, 3, items.frags)             // 2 + 1 fragments returned
}

func TestMaintain_RevivesDegrading(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo(&Spire{ID: "sp1", RegionKey: "k", State: "degrading", BuildTarget: 500})
	svc := NewService(repo, &fakeItems{}, &fakeRes{})
	st, err := svc.ContributeResources(ctx, "p1", 54.7, 20.5)
	assert.NoError(t, err)
	assert.Equal(t, "active", st.Spire.State) // upkeep revived it
	assert.Equal(t, 1, repo.maintained)
}

func TestContributeFragment_RebuildsFromRuins(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo(&Spire{ID: "sp1", RegionKey: "k", State: "ruins"})
	svc := NewService(repo, &fakeItems{frags: 1}, &fakeRes{})
	st, err := svc.ContributeFragment(ctx, "p1", 54.7, 20.5)
	assert.NoError(t, err)
	// ruins → rebuild → assembling, then this fragment counts.
	assert.Equal(t, "assembling", st.Spire.State)
	assert.Equal(t, 1, st.Spire.Fragments)
}
