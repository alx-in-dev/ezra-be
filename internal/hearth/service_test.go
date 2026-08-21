package hearth

import (
	"context"
	"testing"
	"time"

	"github.com/ezra-game/server/pkg/httputil"
)

type fakeRepo struct {
	last       *EphemeralHearth
	lastOk     bool
	created    []*EphemeralHearth
	activeNear bool
}

func (f *fakeRepo) Create(_ context.Context, h *EphemeralHearth) error {
	h.ID = "hearth-1"
	h.CreatedAt = time.Now()
	f.created = append(f.created, h)
	return nil
}
func (f *fakeRepo) LastByOwner(context.Context, string) (*EphemeralHearth, bool, error) {
	return f.last, f.lastOk, nil
}
func (f *fakeRepo) ActiveNearby(context.Context, float64, float64, float64) (bool, error) {
	return f.activeNear, nil
}

type fakeFaction struct{ sym bool }

func (f fakeFaction) IsSymbiont(context.Context, string) (bool, error) { return f.sym, nil }

func TestSummon_FirstTimeSucceeds(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo).WithFaction(fakeFaction{sym: true})

	res, err := svc.Summon(context.Background(), "p1", 1, 1)

	if err != nil {
		t.Fatalf("summon: %v", err)
	}
	if res.ID != "hearth-1" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if len(repo.created) != 1 {
		t.Fatalf("want 1 hearth created, got %d", len(repo.created))
	}
}

func TestSummon_RejectsNonSymbiont(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo).WithFaction(fakeFaction{sym: false})

	_, err := svc.Summon(context.Background(), "p1", 1, 1)

	ae, ok := err.(*httputil.AppError)
	if !ok || ae.Code != "not_symbiont" {
		t.Fatalf("want not_symbiont, got %v", err)
	}
}

func TestSummon_RejectsWhileStillActive(t *testing.T) {
	now := time.Now()
	repo := &fakeRepo{lastOk: true, last: &EphemeralHearth{
		CreatedAt: now.Add(-1 * time.Minute),
		ExpiresAt: now.Add(TTLMin*time.Minute - time.Minute), // still active
	}}
	svc := NewService(repo).WithFaction(fakeFaction{sym: true})

	_, err := svc.Summon(context.Background(), "p1", 1, 1)

	ae, ok := err.(*httputil.AppError)
	if !ok || ae.Code != "hearth_active" {
		t.Fatalf("want hearth_active, got %v", err)
	}
	if len(repo.created) != 0 {
		t.Fatal("should not create a second active hearth (cap=1)")
	}
}

func TestSummon_RejectsDuringCooldownAfterExpiry(t *testing.T) {
	now := time.Now()
	repo := &fakeRepo{lastOk: true, last: &EphemeralHearth{
		CreatedAt: now.Add(-5 * time.Minute), // well within CooldownMin of the last summon
		ExpiresAt: now.Add(-1 * time.Minute), // already expired
	}}
	svc := NewService(repo).WithFaction(fakeFaction{sym: true})

	_, err := svc.Summon(context.Background(), "p1", 1, 1)

	ae, ok := err.(*httputil.AppError)
	if !ok || ae.Code != "hearth_cooldown" {
		t.Fatalf("want hearth_cooldown, got %v", err)
	}
}

func TestSummon_AllowedAfterCooldownElapsed(t *testing.T) {
	now := time.Now()
	repo := &fakeRepo{lastOk: true, last: &EphemeralHearth{
		CreatedAt: now.Add(-(CooldownMin + 1) * time.Minute),
		ExpiresAt: now.Add(-(CooldownMin - TTLMin) * time.Minute), // long expired
	}}
	svc := NewService(repo).WithFaction(fakeFaction{sym: true})

	_, err := svc.Summon(context.Background(), "p1", 1, 1)

	if err != nil {
		t.Fatalf("should be allowed once cooldown elapsed: %v", err)
	}
}

func TestBuffed_ReflectsRepository(t *testing.T) {
	repo := &fakeRepo{activeNear: true}
	svc := NewService(repo)

	buffed, err := svc.Buffed(context.Background(), 1, 1)

	if err != nil {
		t.Fatalf("buffed: %v", err)
	}
	if !buffed {
		t.Fatal("want buffed=true")
	}
}
