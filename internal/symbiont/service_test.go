package symbiont

import (
	"context"
	"testing"

	"github.com/ezra-game/server/internal/hearth"
	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/internal/roster"
	"github.com/ezra-game/server/internal/station"
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

// fakeStations is a stub StationSaboteur (T-804).
type fakeStations struct {
	calls        int
	result       station.SabotageResult
	nearestCalls int
	nearest      station.NearestResult
}

func (f *fakeStations) Sabotage(_ context.Context, _, _ float64, _ string, _ int) (*station.SabotageResult, error) {
	f.calls++
	r := f.result
	return &r, nil
}

func (f *fakeStations) NearestHostile(_ context.Context, _, _ float64, _ string, _ int) (*station.NearestResult, error) {
	f.nearestCalls++
	r := f.nearest
	return &r, nil
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
	level      int
	lastDamage int
	lastRadius int
}

func (f *fakeCorroder) Corrode(_ context.Context, _, _ float64, _ string, damage, _ int) (*tower.CorrodeResult, error) {
	f.calls++
	f.lastDamage = damage
	return &tower.CorrodeResult{Found: f.found, Destroyed: f.destroyed, RemainingHP: 10, Level: f.level}, nil
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
	// Level 0 (unset on this fake) is untiered → falls back to the floor.
	if res.total != ResonancePerOverloadFloor {
		t.Fatalf("want resonance +%d, got %d", ResonancePerOverloadFloor, res.total)
	}
	if out.EnergySiphoned != ResonancePerOverloadFloor {
		t.Fatalf("want energy_siphoned %d, got %d", ResonancePerOverloadFloor, out.EnergySiphoned)
	}
}

// T-803: Overload's Resonance reward scales with the struck beacon's level
// (its real income rate), not a flat amount — a fat L5 beacon pays out more
// than a fresh L1 one.
func TestOverload_ResonanceScalesWithBeaconLevel(t *testing.T) {
	ctx := context.Background()
	cells := &fakeCells{covered: true}

	l1 := &fakeCorroder{found: true, destroyed: true, level: 1}
	svc1 := NewService(fakeFaction{sym: true}, cells, nil, &fakeSpender{}, &fakeResonance{}).WithCorroder(l1)
	out1, err := svc1.Overload(ctx, "p1", 1, 1)
	if err != nil {
		t.Fatalf("l1 overload: %v", err)
	}

	l5 := &fakeCorroder{found: true, destroyed: true, level: 5}
	svc5 := NewService(fakeFaction{sym: true}, cells, nil, &fakeSpender{}, &fakeResonance{}).WithCorroder(l5)
	out5, err := svc5.Overload(ctx, "p1", 1, 1)
	if err != nil {
		t.Fatalf("l5 overload: %v", err)
	}

	if out5.EnergySiphoned <= out1.EnergySiphoned {
		t.Fatalf("want L5 to siphon more than L1, got L1=%d L5=%d", out1.EnergySiphoned, out5.EnergySiphoned)
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

// T-804: Recon falls back to scouting a station when no beacon is found.
func TestRecon_FallsBackToStationWhenNoBeaconFound(t *testing.T) {
	ctx := context.Background()
	cor := &fakeCorroder{found: false}
	stations := &fakeStations{nearest: station.NearestResult{Found: true, StationID: "p1", DistanceM: 77, State: "active"}}
	svc := NewService(fakeFaction{sym: true}, &fakeCells{}, nil, &fakeSpender{}, &fakeResonance{}).
		WithCorroder(cor).WithStations(stations)

	out, err := svc.Recon(ctx, "p1", 1, 1)

	if err != nil {
		t.Fatalf("recon: %v", err)
	}
	if !out.Found || out.TargetKind != "station" || out.State != "active" || out.DistanceM != 77 {
		t.Fatalf("unexpected recon result %+v", out)
	}
	if stations.nearestCalls != 1 {
		t.Fatalf("want station scout attempted once, got %d", stations.nearestCalls)
	}
}

// T-804: a scouted beacon takes priority over the station fallback.
func TestRecon_BeaconTakesPriorityOverStation(t *testing.T) {
	ctx := context.Background()
	cor := &fakeCorroder{found: true}
	stations := &fakeStations{nearest: station.NearestResult{Found: true}}
	svc := NewService(fakeFaction{sym: true}, &fakeCells{}, nil, &fakeSpender{}, &fakeResonance{}).
		WithCorroder(cor).WithStations(stations)

	out, err := svc.Recon(ctx, "p1", 1, 1)

	if err != nil {
		t.Fatalf("recon: %v", err)
	}
	if out.TargetKind != "beacon" {
		t.Fatalf("want beacon to win, got target_kind=%q", out.TargetKind)
	}
	if stations.nearestCalls != 0 {
		t.Fatalf("station scout should not run when a beacon was found, got %d calls", stations.nearestCalls)
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

// T-804: Overload falls back to a station strike when no beacon is found
// under a foreign dome.
func TestOverload_FallsBackToStationWhenNoBeaconFound(t *testing.T) {
	ctx := context.Background()
	cor := &fakeCorroder{found: false}
	stations := &fakeStations{result: station.SabotageResult{Found: true, StationID: "p1", State: "degrading"}}
	res := &fakeResonance{}
	svc := NewService(fakeFaction{sym: true}, &fakeCells{covered: true}, nil, &fakeSpender{}, res).
		WithCorroder(cor).WithStations(stations)

	out, err := svc.Overload(ctx, "p1", 1, 1)

	if err != nil {
		t.Fatalf("overload: %v", err)
	}
	if !out.Found || out.TargetKind != "station" || out.Destroyed {
		t.Fatalf("want a station hit (not ruined), got %+v", out)
	}
	if stations.calls != 1 {
		t.Fatalf("want station sabotage attempted once, got %d", stations.calls)
	}
	if res.total != ResonancePerStationSabotage {
		t.Fatalf("want resonance +%d, got %d", ResonancePerStationSabotage, res.total)
	}
}

// T-804: a beacon hit takes priority over the station fallback — the station
// dep must not even be called when the beacon strike lands.
func TestOverload_BeaconTakesPriorityOverStation(t *testing.T) {
	ctx := context.Background()
	cor := &fakeCorroder{found: true, destroyed: true}
	stations := &fakeStations{result: station.SabotageResult{Found: true}}
	svc := NewService(fakeFaction{sym: true}, &fakeCells{covered: true}, nil, &fakeSpender{}, &fakeResonance{}).
		WithCorroder(cor).WithStations(stations)

	out, err := svc.Overload(ctx, "p1", 1, 1)

	if err != nil {
		t.Fatalf("overload: %v", err)
	}
	if out.TargetKind != "beacon" {
		t.Fatalf("want beacon to win, got target_kind=%q", out.TargetKind)
	}
	if stations.calls != 0 {
		t.Fatalf("station fallback should not run when the beacon strike landed, got %d calls", stations.calls)
	}
}

// T-804: the station fallback works even with no foreign dome overhead at
// all — Resonance growth shouldn't require human density nearby.
func TestOverload_StationWorksWithoutAnyDome(t *testing.T) {
	ctx := context.Background()
	stations := &fakeStations{result: station.SabotageResult{Found: true, Ruined: true}}
	svc := NewService(fakeFaction{sym: true}, &fakeCells{covered: false}, nil, &fakeSpender{}, &fakeResonance{}).
		WithStations(stations)

	out, err := svc.Overload(ctx, "p1", 1, 1)

	if err != nil {
		t.Fatalf("overload without any dome should succeed via station fallback: %v", err)
	}
	if !out.Found || out.TargetKind != "station" || !out.Destroyed {
		t.Fatalf("want a ruined station hit, got %+v", out)
	}
}

// Still gated when nothing at all is wired to catch it: no dome, no station dep.
func TestOverload_StillGatedWithoutDomeOrStations(t *testing.T) {
	ctx := context.Background()
	cor := &fakeCorroder{found: true}
	svc := NewService(fakeFaction{sym: true}, &fakeCells{covered: false}, nil, &fakeSpender{}, &fakeResonance{}).
		WithCorroder(cor)

	_, err := svc.Overload(ctx, "p1", 1, 1)

	ae, ok := err.(*httputil.AppError)
	if !ok || ae.Code != "not_under_dome" {
		t.Fatalf("want not_under_dome, got %v", err)
	}
}

// fakeHearth is a stub HearthBuffChecker (T-805).
type fakeHearth struct{ buffed bool }

func (f fakeHearth) Buffed(context.Context, float64, float64) (bool, error) { return f.buffed, nil }

// T-805: an active Ephemeral Hearth nearby boosts Overload damage against a
// beacon and is reported back for client copy.
func TestOverload_HearthBuffBoostsDamage(t *testing.T) {
	ctx := context.Background()
	cor := &fakeCorroder{found: true}
	svc := NewService(fakeFaction{sym: true}, &fakeCells{covered: true}, nil, &fakeSpender{}, &fakeResonance{}).
		WithCorroder(cor).WithHearth(fakeHearth{buffed: true})

	out, err := svc.Overload(ctx, "p1", 1, 1)

	if err != nil {
		t.Fatalf("overload: %v", err)
	}
	if !out.HearthBuffed {
		t.Fatal("want HearthBuffed=true")
	}
	if cor.lastDamage != OverloadDamage+hearth.DamageBonus {
		t.Fatalf("want damage %d+%d, got %d", OverloadDamage, hearth.DamageBonus, cor.lastDamage)
	}
}

func TestOverload_NoHearthNoBonus(t *testing.T) {
	ctx := context.Background()
	cor := &fakeCorroder{found: true}
	svc := NewService(fakeFaction{sym: true}, &fakeCells{covered: true}, nil, &fakeSpender{}, &fakeResonance{}).
		WithCorroder(cor).WithHearth(fakeHearth{buffed: false})

	out, err := svc.Overload(ctx, "p1", 1, 1)

	if err != nil {
		t.Fatalf("overload: %v", err)
	}
	if out.HearthBuffed {
		t.Fatal("want HearthBuffed=false")
	}
	if cor.lastDamage != OverloadDamage {
		t.Fatalf("want plain damage %d, got %d", OverloadDamage, cor.lastDamage)
	}
}

// fakePressure is a stub PressureReader (T-806).
type fakePressure struct{ n int }

func (f fakePressure) PiercedCellCount(context.Context) (int, error) { return f.n, nil }

func TestPressure_ReportsPiercedCellCount(t *testing.T) {
	svc := NewService(fakeFaction{sym: true}, &fakeCells{}, nil, &fakeSpender{}, &fakeResonance{}).
		WithPressure(fakePressure{n: 42})

	p, err := svc.Pressure(context.Background())

	if err != nil {
		t.Fatalf("pressure: %v", err)
	}
	if p.PiercedCells != 42 {
		t.Fatalf("want 42, got %d", p.PiercedCells)
	}
}

func TestPressure_ZeroWhenNotWired(t *testing.T) {
	svc := NewService(fakeFaction{sym: true}, &fakeCells{}, nil, &fakeSpender{}, &fakeResonance{})

	p, err := svc.Pressure(context.Background())

	if err != nil {
		t.Fatalf("pressure: %v", err)
	}
	if p.PiercedCells != 0 {
		t.Fatalf("want 0, got %d", p.PiercedCells)
	}
}
