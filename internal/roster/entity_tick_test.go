package roster

import (
	"context"
	"testing"
	"time"

	"github.com/ezra-game/server/internal/canon"
	"github.com/ezra-game/server/internal/rift"
	"github.com/ezra-game/server/internal/tower"
	"github.com/ezra-game/server/pkg/httputil"
)

// fakeCorroder records CorrodeTower calls and returns a scripted result.
type fakeCorroder struct {
	res   *tower.CorrodeResult
	calls int
}

func (f *fakeCorroder) CorrodeTower(_ context.Context, _, _ string, _ int) (*tower.CorrodeResult, error) {
	f.calls++
	return f.res, nil
}

func (f *fakeCorroder) WeakestHostile(_ context.Context, _, _ float64, _ string, _ int) (*tower.WeakestResult, error) {
	return &tower.WeakestResult{Found: true, TowerID: "t1"}, nil
}

// fakeAmplifier records AmplifyRift calls and returns a scripted open flag.
type fakeAmplifier struct {
	open  bool
	calls int
}

func (f *fakeAmplifier) AmplifyRift(_ context.Context, _ string, _ int) (bool, int, error) {
	f.calls++
	return f.open, 50, nil
}

func (f *fakeAmplifier) NearestOpenRift(_ context.Context, _, _, _ float64) (*rift.Rift, error) {
	return &rift.Rift{ID: "r1"}, nil
}

func codeOf(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if ae, ok := err.(*httputil.AppError); ok {
		return ae.Code
	}
	t.Fatalf("not an AppError: %v", err)
	return ""
}

func TestAssignEntity_GatesAndCap(t *testing.T) {
	ctx := context.Background()
	ents := newFakeEntities()
	svc := newEntitySvc(ents, fakeResonance{pool: 0, level: 1}) // RL1 → cap 2, distortion/absorber locked

	assault1 := ents.row("p1", canon.EntityAssault, canon.EntityAvailable, nil, nil)
	assault2 := ents.row("p1", canon.EntityAssault, canon.EntityAvailable, nil, nil)
	assault3 := ents.row("p1", canon.EntityAssault, canon.EntityAvailable, nil, nil)
	distortion := ents.row("p1", canon.EntityDistortion, canon.EntityAvailable, nil, nil)
	scout := ents.row("p1", canon.EntityScout, canon.EntityAvailable, nil, nil)

	// Wrong target kind for an assault (it's a tower archetype).
	if c := codeOf(t, mustErr(svc.AssignEntity(ctx, "p1", assault1.ID, canon.EntityTargetRift, "r1"))); c != "wrong_target" {
		t.Fatalf("wrong-target code = %s", c)
	}
	// Locked archetype at RL1.
	if c := codeOf(t, mustErr(svc.AssignEntity(ctx, "p1", distortion.ID, canon.EntityTargetTower, "t1"))); c != "locked" {
		t.Fatalf("locked code = %s", c)
	}
	// Not your entity.
	if c := codeOf(t, mustErr(svc.AssignEntity(ctx, "intruder", assault1.ID, canon.EntityTargetTower, "t1"))); c != "not_owner" {
		t.Fatalf("not-owner code = %s", c)
	}

	// Two assaults fill the RL1 cap of 2.
	if _, err := svc.AssignEntity(ctx, "p1", assault1.ID, canon.EntityTargetTower, "t1"); err != nil {
		t.Fatalf("assign 1: %v", err)
	}
	if _, err := svc.AssignEntity(ctx, "p1", assault2.ID, canon.EntityTargetTower, "t2"); err != nil {
		t.Fatalf("assign 2: %v", err)
	}
	// Third exceeds the cap.
	if c := codeOf(t, mustErr(svc.AssignEntity(ctx, "p1", assault3.ID, canon.EntityTargetTower, "t3"))); c != "no_slots" {
		t.Fatalf("no-slots code = %s", c)
	}
	// Scout (rift kind) also blocked — cap is full regardless of kind.
	if c := codeOf(t, mustErr(svc.AssignEntity(ctx, "p1", scout.ID, canon.EntityTargetRift, "r1"))); c != "no_slots" {
		t.Fatalf("scout no-slots code = %s", c)
	}

	// Assigning an already-busy entity fails.
	if c := codeOf(t, mustErr(svc.AssignEntity(ctx, "p1", assault1.ID, canon.EntityTargetTower, "t9"))); c != "busy" {
		t.Fatalf("busy code = %s", c)
	}
}

func TestRecallEntity(t *testing.T) {
	ctx := context.Background()
	ents := newFakeEntities()
	svc := newEntitySvc(ents, fakeResonance{pool: 0, level: 1})

	idle := ents.row("p1", canon.EntityAssault, canon.EntityAvailable, nil, nil)
	if c := codeOf(t, mustErr(svc.RecallEntity(ctx, "p1", idle.ID))); c != "not_engaged" {
		t.Fatalf("idle recall code = %s", c)
	}

	tk, tid := canon.EntityTargetTower, "t1"
	busy := ents.row("p1", canon.EntityAssault, canon.EntityAssigned, &tk, &tid)
	v, err := svc.RecallEntity(ctx, "p1", busy.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != canon.EntityRecovering {
		t.Fatalf("recalled status = %s, want recovering", v.Status)
	}
	stored, _ := ents.GetByID(ctx, busy.ID)
	if stored.TargetID != nil {
		t.Fatal("recall should clear target")
	}
}

func TestRunTick_Lifecycle(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)

	ents := newFakeEntities()
	cor := &fakeCorroder{res: &tower.CorrodeResult{Found: true, Destroyed: false}}
	amp := &fakeAmplifier{open: true}
	svc := newEntitySvc(ents, fakeResonance{level: 5})
	svc.WithEffectors(cor, amp)
	svc.now = func() time.Time { return base }

	tk, tid := canon.EntityTargetTower, "t1"
	rk, rid := canon.EntityTargetRift, "r1"

	// manifesting, due (manifest_at in the past) → promote to assigned.
	past := base.Add(-time.Minute)
	man := &Entity{PlayerID: "p1", Archetype: canon.EntityAssault, Status: canon.EntityManifesting, TargetKind: &tk, TargetID: &tid, ManifestAt: &past}
	_ = ents.Create(ctx, man)
	// assigned tower → corrode (target alive, stays assigned).
	_ = ents.row("p1", canon.EntityAssault, canon.EntityAssigned, &tk, &tid)
	// assigned rift → amplify.
	_ = ents.row("p1", canon.EntityScout, canon.EntityAssigned, &rk, &rid)
	// recovering, due → wake to available.
	rec := &Entity{PlayerID: "p1", Archetype: canon.EntityAssault, Status: canon.EntityRecovering, RecoveryAt: &past}
	_ = ents.Create(ctx, rec)

	applied, disengaged, err := svc.RunTick(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if disengaged != 0 {
		t.Fatalf("disengaged = %d, want 0 (targets alive)", disengaged)
	}
	if applied != 2 {
		t.Fatalf("applied = %d, want 2 (one corrode + one amplify)", applied)
	}
	if cor.calls != 1 || amp.calls != 1 {
		t.Fatalf("effector calls: corrode=%d amplify=%d, want 1/1", cor.calls, amp.calls)
	}
	if e, _ := ents.GetByID(ctx, man.ID); e.Status != canon.EntityAssigned {
		t.Fatalf("manifesting not promoted: %s", e.Status)
	}
	if e, _ := ents.GetByID(ctx, rec.ID); e.Status != canon.EntityAvailable {
		t.Fatalf("recovering not woken: %s", e.Status)
	}
}

func TestRunTick_AttunementGrowsWithUse(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	ents := newFakeEntities()
	cor := &fakeCorroder{res: &tower.CorrodeResult{Found: true, Destroyed: false}}
	svc := newEntitySvc(ents, fakeResonance{level: 5})
	svc.WithEffectors(cor, &fakeAmplifier{open: true})
	svc.now = func() time.Time { return base }

	tk, tid := canon.EntityTargetTower, "t1"
	e := ents.row("p1", canon.EntityAssault, canon.EntityAssigned, &tk, &tid)

	// 10 successful ticks → rank I→II (threshold 10).
	for i := 0; i < 10; i++ {
		if _, _, err := svc.RunTick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	stored, _ := ents.GetByID(ctx, e.ID)
	if stored.AttunementUses != 10 {
		t.Fatalf("uses = %d, want 10", stored.AttunementUses)
	}
	if stored.Attunement != 2 {
		t.Fatalf("rank = %d, want 2 after 10 uses", stored.Attunement)
	}
	// The corrode damage should have scaled up once rank II hit (×1.12 of 20 = 22).
	// fakeCorroder ignores the damage arg, so assert via the canon helper instead.
	if canon.AttunementEffectMultiplier(stored.Attunement) <= 1.0 {
		t.Fatal("rank II should scale effect above base")
	}
}

func TestRunTick_DisengagesOnTargetGone(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	ents := newFakeEntities()
	cor := &fakeCorroder{res: &tower.CorrodeResult{Found: false}} // target gone
	svc := newEntitySvc(ents, fakeResonance{level: 5})
	svc.WithEffectors(cor, &fakeAmplifier{open: false})
	svc.now = func() time.Time { return base }

	tk, tid := canon.EntityTargetTower, "t1"
	e := ents.row("p1", canon.EntityAssault, canon.EntityAssigned, &tk, &tid)

	_, disengaged, err := svc.RunTick(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if disengaged != 1 {
		t.Fatalf("disengaged = %d, want 1", disengaged)
	}
	stored, _ := ents.GetByID(ctx, e.ID)
	if stored.Status != canon.EntityRecovering || stored.TargetID != nil {
		t.Fatalf("entity not disengaged: status=%s target=%v", stored.Status, stored.TargetID)
	}
}

func TestAssignEntityAuto_AcquiresTarget(t *testing.T) {
	ctx := context.Background()
	ents := newFakeEntities()
	svc := newEntitySvc(ents, fakeResonance{level: 5}) // cap 6, all unlocked
	svc.WithEffectors(
		&fakeCorroder{res: &tower.CorrodeResult{Found: true}}, // WeakestHostile → t1
		&fakeAmplifier{open: true},                            // NearestOpenRift → r1
	)

	assault := ents.row("p1", canon.EntityAssault, canon.EntityAvailable, nil, nil)
	scout := ents.row("p1", canon.EntityScout, canon.EntityAvailable, nil, nil)

	v, err := svc.AssignEntityAuto(ctx, "p1", assault.ID, 50.0, 30.0)
	if err != nil {
		t.Fatalf("assault auto-assign: %v", err)
	}
	if v.Status != canon.EntityManifesting || v.TargetKind != canon.EntityTargetTower || v.TargetID != "t1" {
		t.Fatalf("assault assigned wrong: status=%s kind=%s id=%s", v.Status, v.TargetKind, v.TargetID)
	}

	v, err = svc.AssignEntityAuto(ctx, "p1", scout.ID, 50.0, 30.0)
	if err != nil {
		t.Fatalf("scout auto-assign: %v", err)
	}
	if v.TargetKind != canon.EntityTargetRift || v.TargetID != "r1" {
		t.Fatalf("scout assigned wrong: kind=%s id=%s", v.TargetKind, v.TargetID)
	}
}

func TestAssignEntityAuto_NoTarget(t *testing.T) {
	ctx := context.Background()
	ents := newFakeEntities()
	svc := newEntitySvc(ents, fakeResonance{level: 5})
	// corroder reports no hostile beacon found.
	svc.WithEffectors(&fakeCorroder{res: &tower.CorrodeResult{Found: false}}, &fakeAmplifier{open: true})
	// override WeakestHostile via a not-found corroder:
	svc.corroder = noTargetCorroder{}

	assault := ents.row("p1", canon.EntityAssault, canon.EntityAvailable, nil, nil)
	if c := codeOf(t, mustErr(svc.AssignEntityAuto(ctx, "p1", assault.ID, 0, 0))); c != "no_target" {
		t.Fatalf("no-target code = %s", c)
	}
}

// noTargetCorroder reports no weakest hostile beacon.
type noTargetCorroder struct{}

func (noTargetCorroder) CorrodeTower(context.Context, string, string, int) (*tower.CorrodeResult, error) {
	return &tower.CorrodeResult{Found: false}, nil
}
func (noTargetCorroder) WeakestHostile(context.Context, float64, float64, string, int) (*tower.WeakestResult, error) {
	return &tower.WeakestResult{Found: false}, nil
}

func TestVerbBonuses_SumsAssignedByArchetype(t *testing.T) {
	ctx := context.Background()
	ents := newFakeEntities()
	svc := newEntitySvc(ents, fakeResonance{level: 5})

	tk, tid := canon.EntityTargetTower, "t1"
	rk, rid := canon.EntityTargetRift, "r1"
	// Two assigned distortions (rank I each → +8 overload each = 16).
	ents.row("p1", canon.EntityDistortion, canon.EntityAssigned, &tk, &tid)
	ents.row("p1", canon.EntityDistortion, canon.EntityAssigned, &tk, &tid)
	// One assigned scout (rank I → +60 recon range).
	ents.row("p1", canon.EntityScout, canon.EntityAssigned, &rk, &rid)
	// One assigned absorber (rank I → −3 drain).
	ents.row("p1", canon.EntityAbsorber, canon.EntityAssigned, &rk, &rid)
	// Non-contributing: manifesting distortion + available scout.
	ents.row("p1", canon.EntityDistortion, canon.EntityManifesting, &tk, &tid)
	ents.row("p1", canon.EntityScout, canon.EntityAvailable, nil, nil)

	b, err := svc.VerbBonuses(ctx, "p1")
	if err != nil {
		t.Fatal(err)
	}
	if b.OverloadDamage != 16 {
		t.Fatalf("overload bonus = %d, want 16", b.OverloadDamage)
	}
	if b.ReconRangeM != 60 {
		t.Fatalf("recon range bonus = %d, want 60", b.ReconRangeM)
	}
	if b.DrainReduction != 3 {
		t.Fatalf("drain reduction = %d, want 3", b.DrainReduction)
	}
}

// mustErr discards an EntityView result, returning only the error (test helper).
func mustErr(_ *EntityView, err error) error { return err }
