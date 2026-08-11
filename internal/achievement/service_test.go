package achievement

import (
	"context"
	"testing"
)

type fakeRepo struct {
	metrics  Metrics
	unlocked map[string]bool
	marked   []string
}

func (f *fakeRepo) Metrics(context.Context, string) (Metrics, error) { return f.metrics, nil }
func (f *fakeRepo) UnlockedIDs(context.Context, string) (map[string]bool, error) {
	cp := map[string]bool{}
	for k, v := range f.unlocked {
		cp[k] = v
	}
	return cp, nil
}
func (f *fakeRepo) MarkUnlocked(_ context.Context, _, id string) error {
	f.marked = append(f.marked, id)
	if f.unlocked == nil {
		f.unlocked = map[string]bool{}
	}
	f.unlocked[id] = true
	return nil
}

type fakeGranter struct{ total int }

func (g *fakeGranter) IncrementCrystals(_ context.Context, _ string, delta int) error {
	g.total += delta
	return nil
}

func statusByID(list []Status, id string) (Status, bool) {
	for _, s := range list {
		if s.ID == id {
			return s, true
		}
	}
	return Status{}, false
}

func TestEvaluate_UnlocksMetThresholdsOnceWithReward(t *testing.T) {
	// 12 beacons, core L3, 1 win, 0 spectra, level 6 → unlocks:
	// first_beacon, network_10, core_3, battle_1, level_5.
	repo := &fakeRepo{metrics: Metrics{Beacons: 12, CoreLevel: 3, BattlesWon: 1, Spectra: 0, PlayerLevel: 6}}
	granter := &fakeGranter{}
	svc := NewService(repo, granter)

	res, err := svc.Evaluate(context.Background(), "p1")
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	wantUnlocked := map[string]bool{
		"first_beacon": true, "network_10": true, "core_3": true,
		"battle_1": true, "level_5": true,
	}
	if len(res.NewlyUnlocked) != len(wantUnlocked) {
		t.Fatalf("want %d newly unlocked, got %d", len(wantUnlocked), len(res.NewlyUnlocked))
	}
	for _, s := range res.NewlyUnlocked {
		if !wantUnlocked[s.ID] {
			t.Fatalf("unexpected unlock %q", s.ID)
		}
	}
	// network_25 not met (only 12 beacons) → still locked, progress reported.
	if st, ok := statusByID(res.Achievements, "network_25"); !ok || st.Unlocked || st.Progress != 12 {
		t.Fatalf("network_25 should be locked at progress 12, got %+v", st)
	}
	// Reward = 10+15+15+10+15 = 65.
	if granter.total != 65 {
		t.Fatalf("want 65 crystals granted, got %d", granter.total)
	}
	if res.RewardCrystals != 65 {
		t.Fatalf("want result reward 65, got %d", res.RewardCrystals)
	}
}

func TestEvaluate_Idempotent(t *testing.T) {
	repo := &fakeRepo{metrics: Metrics{Beacons: 1}}
	granter := &fakeGranter{}
	svc := NewService(repo, granter)

	first, _ := svc.Evaluate(context.Background(), "p1")
	if len(first.NewlyUnlocked) != 1 || first.NewlyUnlocked[0].ID != "first_beacon" {
		t.Fatalf("first eval should unlock first_beacon, got %+v", first.NewlyUnlocked)
	}

	second, _ := svc.Evaluate(context.Background(), "p1")
	if len(second.NewlyUnlocked) != 0 {
		t.Fatalf("second eval must not re-unlock, got %d", len(second.NewlyUnlocked))
	}
	if granter.total != 10 {
		t.Fatalf("reward must be granted once (10), got %d", granter.total)
	}
	// Still reported as unlocked on the steady-state list.
	if st, _ := statusByID(second.Achievements, "first_beacon"); !st.Unlocked {
		t.Fatalf("first_beacon should stay unlocked")
	}
}
