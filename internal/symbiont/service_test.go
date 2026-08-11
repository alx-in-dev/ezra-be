package symbiont

import (
	"context"
	"testing"

	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/internal/roster"
	"github.com/ezra-game/server/internal/tower"
	"github.com/ezra-game/server/pkg/httputil"
)

type fakeFaction struct{ sym bool }

func (f fakeFaction) IsSymbiont(context.Context, string) (bool, error) { return f.sym, nil }

type fakeCells struct {
	covered   bool
	pierced   bool
	infection float64
	raisedTo  float64
	raiseN    int
}

func (f *fakeCells) UnderForeignDome(context.Context, float64, float64, string) (bool, float64, bool, error) {
	return f.covered, f.infection, f.pierced, nil
}
func (f *fakeCells) RaiseInfectionNearest(_ context.Context, _, _, delta, threshold float64, _ int) (float64, bool, error) {
	f.raiseN++
	f.raisedTo = f.infection + delta
	return f.raisedTo, f.raisedTo >= threshold, nil
}

type fakeSpender struct{ spent int }

func (f *fakeSpender) Spend(_ context.Context, _ string, energy, _ int) (*player.Player, error) {
	f.spent += energy
	return &player.Player{}, nil
}

type fakeResonance struct{ total int }

func (f *fakeResonance) AddSymbiontResonance(_ context.Context, _ string, delta int) (int, error) {
	f.total += delta
	return f.total, nil
}

func TestDrainAmount_ScalesWithInfection(t *testing.T) {
	if got := drainAmount(0); got != DrainBaseEnergy {
		t.Fatalf("fresh dome: want full drain %d, got %d", DrainBaseEnergy, got)
	}
	if got := drainAmount(PocketInfection); got != 0 {
		t.Fatalf("carved pocket: want 0 drain, got %d", got)
	}
	mid := drainAmount(PocketInfection / 2)
	if mid <= 0 || mid >= DrainBaseEnergy {
		t.Fatalf("mid infection drain %d should be between 0 and base", mid)
	}
}

func TestRaiseInfection_GatedToUnderDome(t *testing.T) {
	ctx := context.Background()
	cells := &fakeCells{covered: false, infection: 20}
	spender := &fakeSpender{}
	svc := NewService(fakeFaction{sym: true}, cells, nil, spender, &fakeResonance{})

	// Not under a dome → rejected, no energy spent, no raise.
	if _, err := svc.RaiseInfection(ctx, "p1", 1, 1); err == nil {
		t.Fatal("expected rejection when not under a foreign dome")
	} else if ae, ok := err.(*httputil.AppError); !ok || ae.Code != "not_under_dome" {
		t.Fatalf("want not_under_dome AppError, got %v", err)
	}
	if spender.spent != 0 || cells.raiseN != 0 {
		t.Fatalf("nothing should be charged/raised when gated out (spent=%d raised=%d)", spender.spent, cells.raiseN)
	}

	// Under a dome → succeeds, charges energy, raises infection, feeds resonance.
	cells.covered = true
	res := &fakeResonance{}
	svc = NewService(fakeFaction{sym: true}, cells, nil, spender, res)
	out, err := svc.RaiseInfection(ctx, "p1", 1, 1)
	if err != nil {
		t.Fatalf("under dome should succeed: %v", err)
	}
	if spender.spent != RaiseEnergyCost {
		t.Fatalf("want energy charged %d, got %d", RaiseEnergyCost, spender.spent)
	}
	if out.CellInfection != 20+RaiseInfectionStep {
		t.Fatalf("want infection %d, got %v", 20+RaiseInfectionStep, out.CellInfection)
	}
	if res.total != ResonancePerRaise {
		t.Fatalf("want resonance +%d, got %d", ResonancePerRaise, res.total)
	}
}

func TestRaiseInfection_RejectsNonSymbiont(t *testing.T) {
	svc := NewService(fakeFaction{sym: false}, &fakeCells{covered: true}, nil, &fakeSpender{}, &fakeResonance{})
	if _, err := svc.RaiseInfection(context.Background(), "p1", 1, 1); err == nil {
		t.Fatal("non-symbiont should be rejected")
	}
}

type fakeCorroder struct {
	calls      int
	found      bool
	destroyed  bool
	lastDamage int
	lastRadius int
}

func (f *fakeCorroder) Corrode(_ context.Context, _, _ float64, _ string, damage, _ int) (*tower.CorrodeResult, error) {
	f.calls++
	f.lastDamage = damage
	return &tower.CorrodeResult{Found: f.found, Destroyed: f.destroyed, RemainingHP: 10}, nil
}

func (f *fakeCorroder) WeakestHostile(_ context.Context, _, _ float64, _ string, radiusM int) (*tower.WeakestResult, error) {
	f.lastRadius = radiusM
	return &tower.WeakestResult{Found: f.found, DistanceM: 42, HP: 30, HPMax: 100}, nil
}

// fakeRoster is a minimal EntityManager that only reports verb bonuses.
type fakeRoster struct{ bonus roster.VerbBonus }

func (f fakeRoster) EnsureStarterRoster(context.Context, string) error { return nil }
func (f fakeRoster) ListEntities(context.Context, string) ([]roster.EntityView, error) {
	return nil, nil
}
func (f fakeRoster) ControlUsage(context.Context, string) (int, int, int, error) { return 0, 0, 1, nil }
func (f fakeRoster) AssignEntityAuto(context.Context, string, string, float64, float64) (*roster.EntityView, error) {
	return nil, nil
}
func (f fakeRoster) RecallEntity(context.Context, string, string) (*roster.EntityView, error) {
	return nil, nil
}
func (f fakeRoster) VerbBonuses(context.Context, string) (roster.VerbBonus, error) {
	return f.bonus, nil
}

func TestVerbBonus_BoostsOverloadAndRecon(t *testing.T) {
	ctx := context.Background()
	// Overload: distortion bonus adds to corrosion damage.
	cor := &fakeCorroder{found: true}
	svc := NewService(fakeFaction{sym: true}, &fakeCells{covered: true}, nil, &fakeSpender{}, &fakeResonance{}).
		WithCorroder(cor).WithRoster(fakeRoster{bonus: roster.VerbBonus{OverloadDamage: 10, ReconRangeM: 75}})
	if _, err := svc.Overload(ctx, "p1", 1, 1); err != nil {
		t.Fatal(err)
	}
	if cor.lastDamage != OverloadDamage+10 {
		t.Fatalf("overload damage = %d, want %d", cor.lastDamage, OverloadDamage+10)
	}
	// Recon: scout bonus widens the scan radius.
	if _, err := svc.Recon(ctx, "p1", 1, 1); err != nil {
		t.Fatal(err)
	}
	if cor.lastRadius != ReconRangeM+75 {
		t.Fatalf("recon radius = %d, want %d", cor.lastRadius, ReconRangeM+75)
	}
}

func TestOverload_GatedToUnderDome(t *testing.T) {
	ctx := context.Background()
	cells := &fakeCells{covered: false}
	spender := &fakeSpender{}
	cor := &fakeCorroder{found: true}
	svc := NewService(fakeFaction{sym: true}, cells, nil, spender, &fakeResonance{}).WithCorroder(cor)

	if _, err := svc.Overload(ctx, "p1", 1, 1); err == nil {
		t.Fatal("expected rejection when not under a foreign dome")
	} else if ae, ok := err.(*httputil.AppError); !ok || ae.Code != "not_under_dome" {
		t.Fatalf("want not_under_dome AppError, got %v", err)
	}
	if spender.spent != 0 || cor.calls != 0 {
		t.Fatalf("nothing should be charged/corroded when gated out (spent=%d calls=%d)", spender.spent, cor.calls)
	}
}

func TestOverload_ChargesAndFeedsResonanceOnHit(t *testing.T) {
	ctx := context.Background()
	cells := &fakeCells{covered: true}
	spender := &fakeSpender{}
	res := &fakeResonance{}
	cor := &fakeCorroder{found: true, destroyed: true}
	svc := NewService(fakeFaction{sym: true}, cells, nil, spender, res).WithCorroder(cor)

	out, err := svc.Overload(ctx, "p1", 1, 1)
	if err != nil {
		t.Fatalf("under dome should succeed: %v", err)
	}
	if spender.spent != OverloadEnergyCost {
		t.Fatalf("want energy charged %d, got %d", OverloadEnergyCost, spender.spent)
	}
	if !out.Found || !out.Destroyed {
		t.Fatalf("want found+destroyed, got %+v", out)
	}
	if res.total != ResonancePerOverload {
		t.Fatalf("want resonance +%d, got %d", ResonancePerOverload, res.total)
	}
}

func TestRecon_SymbiontOnly_ReturnsWeakest(t *testing.T) {
	ctx := context.Background()
	cor := &fakeCorroder{found: true}
	svc := NewService(fakeFaction{sym: true}, &fakeCells{}, nil, &fakeSpender{}, &fakeResonance{}).WithCorroder(cor)

	out, err := svc.Recon(ctx, "p1", 1, 1)
	if err != nil {
		t.Fatalf("symbiont recon should succeed: %v", err)
	}
	if !out.Found || out.HP != 30 || out.HPMax != 100 || out.DistanceM != 42 {
		t.Fatalf("unexpected recon result %+v", out)
	}

	// Non-symbiont rejected.
	hum := NewService(fakeFaction{sym: false}, &fakeCells{}, nil, &fakeSpender{}, &fakeResonance{}).WithCorroder(cor)
	if _, err := hum.Recon(ctx, "p1", 1, 1); err == nil {
		t.Fatal("non-symbiont recon should be rejected")
	}
}

func TestOverload_NoResonanceWhenNoTarget(t *testing.T) {
	ctx := context.Background()
	res := &fakeResonance{}
	cor := &fakeCorroder{found: false}
	svc := NewService(fakeFaction{sym: true}, &fakeCells{covered: true}, nil, &fakeSpender{}, res).WithCorroder(cor)

	out, err := svc.Overload(ctx, "p1", 1, 1)
	if err != nil {
		t.Fatalf("should not error when no beacon in range: %v", err)
	}
	if out.Found {
		t.Fatal("want Found=false when no hostile beacon in range")
	}
	if res.total != 0 {
		t.Fatalf("no resonance on a miss, got %d", res.total)
	}
}
