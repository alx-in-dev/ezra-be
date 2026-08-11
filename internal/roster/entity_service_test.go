package roster

import (
	"context"
	"testing"

	"github.com/ezra-game/server/internal/canon"
)

// fakeEntities is an in-memory EntityRepository.
type fakeEntities struct {
	rows   map[string]*Entity
	nextID int
}

func newFakeEntities() *fakeEntities { return &fakeEntities{rows: map[string]*Entity{}} }

func (f *fakeEntities) Create(_ context.Context, e *Entity) error {
	f.nextID++
	e.ID = string(rune('a' + f.nextID))
	cp := *e
	f.rows[e.ID] = &cp
	return nil
}
func (f *fakeEntities) GetByID(_ context.Context, id string) (*Entity, error) {
	e, ok := f.rows[id]
	if !ok {
		return nil, context.Canceled
	}
	cp := *e
	return &cp, nil
}
func (f *fakeEntities) ListByPlayer(_ context.Context, playerID string) ([]Entity, error) {
	var out []Entity
	for _, e := range f.rows {
		if e.PlayerID == playerID {
			out = append(out, *e)
		}
	}
	return out, nil
}
func (f *fakeEntities) CountByPlayer(_ context.Context, playerID string) (int, error) {
	n := 0
	for _, e := range f.rows {
		if e.PlayerID == playerID {
			n++
		}
	}
	return n, nil
}
func (f *fakeEntities) Update(_ context.Context, e *Entity) error {
	cp := *e
	f.rows[e.ID] = &cp
	return nil
}
func (f *fakeEntities) ListEngaged(_ context.Context) ([]Entity, error) {
	var out []Entity
	for _, e := range f.rows {
		if e.Status != canon.EntityAvailable {
			out = append(out, *e)
		}
	}
	return out, nil
}

// fakeResonance reports a fixed pool/level.
type fakeResonance struct{ pool, level, xp int }

func (f fakeResonance) ResonanceProgressOf(context.Context, string) (int, int, int, error) {
	return f.pool, f.level, f.xp, nil
}

func newEntitySvc(ents EntityRepository, res ResonanceReader) *Service {
	s := NewService(fakeUnits{}, fakeTowers{}, fakePets{}, &fakePool{})
	return s.WithEntities(ents, res)
}

func TestEnsureStarterRoster_MaterializesOnceFromPool(t *testing.T) {
	ctx := context.Background()
	ents := newFakeEntities()
	svc := newEntitySvc(ents, fakeResonance{pool: 14, level: 1})

	if err := svc.EnsureStarterRoster(ctx, "p1"); err != nil {
		t.Fatal(err)
	}
	// pool 14 → assault, scout, distortion.
	got, _ := ents.ListByPlayer(ctx, "p1")
	if len(got) != 3 {
		t.Fatalf("materialized %d entities, want 3", len(got))
	}

	// Idempotent: second call adds nothing.
	if err := svc.EnsureStarterRoster(ctx, "p1"); err != nil {
		t.Fatal(err)
	}
	got, _ = ents.ListByPlayer(ctx, "p1")
	if len(got) != 3 {
		t.Fatalf("after re-ensure %d entities, want 3", len(got))
	}
}

func TestListEntities_LocksAboveLevel(t *testing.T) {
	ctx := context.Background()
	ents := newFakeEntities()
	// RL1, but pool seeds an absorber (unlock RL3) → it should report locked.
	svc := newEntitySvc(ents, fakeResonance{pool: 18, level: 1})

	views, err := svc.ListEntities(ctx, "p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 4 {
		t.Fatalf("got %d views, want 4", len(views))
	}
	for _, v := range views {
		wantLocked := v.Archetype == canon.EntityDistortion || v.Archetype == canon.EntityAbsorber
		if v.Locked != wantLocked {
			t.Fatalf("%s locked=%v, want %v (RL1)", v.Archetype, v.Locked, wantLocked)
		}
	}
}

func TestControlUsage_SumsEngagedWeights(t *testing.T) {
	ctx := context.Background()
	ents := newFakeEntities()
	svc := newEntitySvc(ents, fakeResonance{pool: 0, level: 3}) // RL3 → cap 4

	// one assigned absorber (weight 2) + one manifesting assault (weight 1) +
	// one available scout (not counted) + one recovering assault (not counted).
	kind := canon.EntityTargetRift
	tid := "r1"
	_ = ents.row("p1", canon.EntityAbsorber, canon.EntityAssigned, &kind, &tid)
	_ = ents.row("p1", canon.EntityAssault, canon.EntityManifesting, nil, nil)
	_ = ents.row("p1", canon.EntityScout, canon.EntityAvailable, nil, nil)
	_ = ents.row("p1", canon.EntityAssault, canon.EntityRecovering, nil, nil)

	used, capacity, level, err := svc.ControlUsage(ctx, "p1")
	if err != nil {
		t.Fatal(err)
	}
	if used != 3 {
		t.Fatalf("used = %d, want 3 (absorber 2 + assault 1)", used)
	}
	if capacity != 4 {
		t.Fatalf("cap = %d, want 4 (RL3)", capacity)
	}
	if level != 3 {
		t.Fatalf("level = %d, want 3", level)
	}
}

// row is a test helper to insert an entity in a given state.
func (f *fakeEntities) row(playerID, arch, status string, kind, tid *string) *Entity {
	e := &Entity{PlayerID: playerID, Archetype: arch, Attunement: 1, Status: status, TargetKind: kind, TargetID: tid}
	_ = f.Create(context.Background(), e)
	stored := f.rows[e.ID]
	stored.TargetKind = kind
	stored.TargetID = tid
	stored.Status = status
	return stored
}
