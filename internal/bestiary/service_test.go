package bestiary

import (
	"context"
	"testing"
)

type fakeRepo struct {
	discovered map[string]bool
	rewarded   bool
}

func (f *fakeRepo) DiscoveredKeys(context.Context, string) (map[string]bool, error) {
	cp := map[string]bool{}
	for k, v := range f.discovered {
		cp[k] = v
	}
	return cp, nil
}
func (f *fakeRepo) MarkDiscovered(_ context.Context, _ string, keys []string) error {
	if f.discovered == nil {
		f.discovered = map[string]bool{}
	}
	for _, k := range keys {
		f.discovered[k] = true
	}
	return nil
}
func (f *fakeRepo) HasCompletionReward(context.Context, string) (bool, error) { return f.rewarded, nil }
func (f *fakeRepo) SetCompletionReward(context.Context, string) error         { f.rewarded = true; return nil }

type fakeGranter struct{ total int }

func (g *fakeGranter) IncrementCrystals(_ context.Context, _ string, d int) error {
	g.total += d
	return nil
}

func TestStatus_ProgressAndPartial(t *testing.T) {
	repo := &fakeRepo{discovered: map[string]bool{"spirit:small": true, "rift:minor": true}}
	g := &fakeGranter{}
	res, err := NewService(repo, g).Status(context.Background(), "p1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != len(Catalog) {
		t.Fatalf("total %d want %d", res.Total, len(Catalog))
	}
	if res.Discovered != 2 {
		t.Fatalf("discovered %d want 2", res.Discovered)
	}
	if res.Complete {
		t.Fatal("should not be complete")
	}
	if g.total != 0 {
		t.Fatalf("no reward for partial, got %d", g.total)
	}
}

func TestStatus_CompletionRewardOnce(t *testing.T) {
	all := map[string]bool{}
	for _, e := range Catalog {
		all[e.Key] = true
	}
	repo := &fakeRepo{discovered: all}
	g := &fakeGranter{}
	svc := NewService(repo, g)

	first, _ := svc.Status(context.Background(), "p1")
	if !first.Complete || first.RewardCrystals != CompletionRewardCrystals {
		t.Fatalf("first complete should reward %d, got complete=%v reward=%d",
			CompletionRewardCrystals, first.Complete, first.RewardCrystals)
	}
	second, _ := svc.Status(context.Background(), "p1")
	if second.RewardCrystals != 0 {
		t.Fatalf("completion reward must be once, got %d", second.RewardCrystals)
	}
	if g.total != CompletionRewardCrystals {
		t.Fatalf("granter total %d want %d", g.total, CompletionRewardCrystals)
	}
}

func TestDiscover_Dedups(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo, nil)
	_ = svc.Discover(context.Background(), "p1", []string{"spirit:shard", "rift:critical"})
	_ = svc.Discover(context.Background(), "p1", []string{"spirit:shard"})
	if len(repo.discovered) != 2 {
		t.Fatalf("want 2 distinct discoveries, got %d", len(repo.discovered))
	}
}
