package item

import (
	"context"
	"testing"
)

// fakeRepo is an in-memory item.Repository for pity tests.
type fakeRepo struct {
	qty map[string]int // key: itemType|variant
}

func newFakeRepo() *fakeRepo { return &fakeRepo{qty: map[string]int{}} }

func (f *fakeRepo) Add(_ context.Context, _, itemType, variant string, delta int) (int, error) {
	k := itemType + "|" + variant
	f.qty[k] += delta
	return f.qty[k], nil
}

func (f *fakeRepo) Consume(context.Context, string, string, int) (bool, error) { return true, nil }
func (f *fakeRepo) ListByPlayer(context.Context, string) ([]Item, error)       { return nil, nil }

func TestEffectivePityChance(t *testing.T) {
	const base = 0.02
	if got := effectivePityChance(softPityBlueprint, base); got != base {
		t.Fatalf("at soft pity want base %v, got %v", base, got)
	}
	if got := effectivePityChance(softPityBlueprint-3, base); got != base {
		t.Fatalf("below soft pity want base %v, got %v", base, got)
	}
	if got := effectivePityChance(hardPityBlueprint, base); got != 1.0 {
		t.Fatalf("at hard pity want guaranteed 1.0, got %v", got)
	}
	mid := effectivePityChance((softPityBlueprint+hardPityBlueprint)/2, base)
	if mid <= base || mid >= 1.0 {
		t.Fatalf("mid-ramp chance %v should sit strictly between base and 1.0", mid)
	}
}

func TestRollBlueprintFragment_HardPityGuarantees(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	ctx := context.Background()

	// roll=0.999 never beats a base of 0.02 — every roll is a miss until the
	// hard-pity guarantee forces a drop on the hardPityBlueprint-th attempt.
	dropAt := 0
	for i := 1; i <= hardPityBlueprint; i++ {
		dropped, err := svc.RollBlueprintFragment(ctx, "p1", 0.02, 0.999)
		if err != nil {
			t.Fatalf("roll %d: %v", i, err)
		}
		if dropped {
			dropAt = i
			break
		}
	}
	if dropAt != hardPityBlueprint {
		t.Fatalf("expected guaranteed drop at attempt %d, got %d", hardPityBlueprint, dropAt)
	}
	if frags := repo.qty[TypeBlueprintFragment+"|"]; frags != 1 {
		t.Fatalf("want exactly 1 fragment granted, got %d", frags)
	}
	if streak := repo.qty[TypePityBlueprint+"|"]; streak != 0 {
		t.Fatalf("want streak reset to 0 after drop, got %d", streak)
	}
}

func TestRollBlueprintFragment_DropResetsStreak(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	ctx := context.Background()

	// Two guaranteed misses (roll above base), then a guaranteed hit (roll 0).
	_, _ = svc.RollBlueprintFragment(ctx, "p1", 0.1, 0.999)
	_, _ = svc.RollBlueprintFragment(ctx, "p1", 0.1, 0.999)
	if streak := repo.qty[TypePityBlueprint+"|"]; streak != 2 {
		t.Fatalf("want streak 2 after two misses, got %d", streak)
	}
	dropped, err := svc.RollBlueprintFragment(ctx, "p1", 0.1, 0.0)
	if err != nil || !dropped {
		t.Fatalf("roll=0 must drop: dropped=%v err=%v", dropped, err)
	}
	if streak := repo.qty[TypePityBlueprint+"|"]; streak != 0 {
		t.Fatalf("want streak reset after drop, got %d", streak)
	}
}
