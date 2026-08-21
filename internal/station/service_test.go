package station

import (
	"context"
	"testing"

	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/pkg/httputil"
)

type fakeRepo struct {
	count       int
	nearestDist float64
	nearestOk   bool
	created     int
	listed      []PowerPlant
	degraded    []string // IDs passed to DegradeOneStep, in order
}

func (f *fakeRepo) Create(_ context.Context, p *PowerPlant) error {
	f.created++
	p.ID = "plant"
	p.State = "active"
	return nil
}
func (f *fakeRepo) CountInRadius(context.Context, float64, float64, float64) (int, error) {
	return f.count, nil
}
func (f *fakeRepo) NearestDistanceM(context.Context, float64, float64) (float64, bool, error) {
	return f.nearestDist, f.nearestOk, nil
}
func (f *fakeRepo) StationBonusInRadius(context.Context, float64, float64, float64) (int, error) {
	return 0, nil
}
func (f *fakeRepo) ListInRadius(context.Context, float64, float64, float64) ([]PowerPlant, error) {
	return f.listed, nil
}
func (f *fakeRepo) AdvanceLifecycle(context.Context, int, int) error { return nil }
func (f *fakeRepo) Upkeep(context.Context, string, float64, float64, float64) (bool, error) {
	return true, nil
}
func (f *fakeRepo) DegradeOneStep(_ context.Context, id string) (string, error) {
	f.degraded = append(f.degraded, id)
	for i := range f.listed {
		if f.listed[i].ID != id {
			continue
		}
		if f.listed[i].State == "active" {
			f.listed[i].State = "degrading"
		} else {
			f.listed[i].State = "ruins"
		}
		return f.listed[i].State, nil
	}
	return "degrading", nil
}

type fakeSpender struct{ energy, materials int }

func (f *fakeSpender) Spend(_ context.Context, _ string, e, m int) (*player.Player, error) {
	f.energy += e
	f.materials += m
	return &player.Player{}, nil
}

func TestBuild_OK(t *testing.T) {
	repo := &fakeRepo{count: 0, nearestOk: false}
	sp := &fakeSpender{}
	p, err := NewService(repo, sp).Build(context.Background(), "p1", 54.7, 20.5)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if repo.created != 1 || p.CapacityBonus != CapacityBonus {
		t.Fatalf("plant not created right: created=%d bonus=%d", repo.created, p.CapacityBonus)
	}
	if sp.energy != CostEnergy || sp.materials != CostMaterials {
		t.Fatalf("wrong cost charged: e=%d m=%d", sp.energy, sp.materials)
	}
}

func TestBuild_RejectsTooClose(t *testing.T) {
	repo := &fakeRepo{nearestOk: true, nearestDist: MinSpacingM - 1}
	sp := &fakeSpender{}
	_, err := NewService(repo, sp).Build(context.Background(), "p1", 54.7, 20.5)
	if ae, ok := err.(*httputil.AppError); !ok || ae.Code != "too_close" {
		t.Fatalf("want too_close, got %v", err)
	}
	if repo.created != 0 || sp.energy != 0 {
		t.Fatal("nothing should be built/charged when too close")
	}
}

func TestBuild_RejectsAreaFull(t *testing.T) {
	repo := &fakeRepo{count: MaxInRadius}
	sp := &fakeSpender{}
	_, err := NewService(repo, sp).Build(context.Background(), "p1", 54.7, 20.5)
	if ae, ok := err.(*httputil.AppError); !ok || ae.Code != "area_full" {
		t.Fatalf("want area_full, got %v", err)
	}
	if repo.created != 0 || sp.energy != 0 {
		t.Fatal("nothing should be built/charged when area full")
	}
}

// fakeFaction is a stub FactionChecker (T-800).
type fakeFaction struct{ symbionts map[string]bool }

func (f fakeFaction) IsSymbiont(_ context.Context, playerID string) (bool, error) {
	return f.symbionts[playerID], nil
}

func TestBuild_RejectsSymbiont(t *testing.T) {
	repo := &fakeRepo{count: 0, nearestOk: false}
	sp := &fakeSpender{}
	svc := NewService(repo, sp).WithFaction(fakeFaction{symbionts: map[string]bool{"sym-1": true}})

	_, err := svc.Build(context.Background(), "sym-1", 54.7, 20.5)

	ae, ok := err.(*httputil.AppError)
	if !ok || ae.Code != "symbiont_no_human_toolkit" {
		t.Fatalf("want symbiont_no_human_toolkit, got %v", err)
	}
	if repo.created != 0 || sp.energy != 0 {
		t.Fatal("nothing should be built/charged for a symbiont")
	}
}

func TestSabotage_DegradesNearestHostileActivePlant(t *testing.T) {
	repo := &fakeRepo{listed: []PowerPlant{
		{ID: "p1", OwnerID: "human-1", Lat: 54.7, Lng: 20.5, State: "active"},
	}}
	svc := NewService(repo, &fakeSpender{})

	res, err := svc.Sabotage(context.Background(), 54.7, 20.5, "sym-1", 100)

	if err != nil {
		t.Fatalf("sabotage: %v", err)
	}
	if !res.Found || res.StationID != "p1" || res.State != "degrading" || res.Ruined {
		t.Fatalf("unexpected result: %+v", res)
	}
	if len(repo.degraded) != 1 || repo.degraded[0] != "p1" {
		t.Fatalf("want DegradeOneStep(p1), got %v", repo.degraded)
	}
}

func TestSabotage_DegradingGoesToRuins(t *testing.T) {
	repo := &fakeRepo{listed: []PowerPlant{
		{ID: "p1", OwnerID: "human-1", Lat: 54.7, Lng: 20.5, State: "degrading"},
	}}
	svc := NewService(repo, &fakeSpender{})

	res, err := svc.Sabotage(context.Background(), 54.7, 20.5, "sym-1", 100)

	if err != nil {
		t.Fatalf("sabotage: %v", err)
	}
	if !res.Found || res.State != "ruins" || !res.Ruined {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestSabotage_SkipsOwnPlant(t *testing.T) {
	repo := &fakeRepo{listed: []PowerPlant{
		{ID: "p1", OwnerID: "sym-1", Lat: 54.7, Lng: 20.5, State: "active"},
	}}
	svc := NewService(repo, &fakeSpender{})

	res, err := svc.Sabotage(context.Background(), 54.7, 20.5, "sym-1", 100)

	if err != nil {
		t.Fatalf("sabotage: %v", err)
	}
	if res.Found {
		t.Fatalf("should not target attacker's own plant, got %+v", res)
	}
}

func TestSabotage_SkipsAlreadyRuinedPlant(t *testing.T) {
	repo := &fakeRepo{listed: []PowerPlant{
		{ID: "p1", OwnerID: "human-1", Lat: 54.7, Lng: 20.5, State: "ruins"},
	}}
	svc := NewService(repo, &fakeSpender{})

	res, err := svc.Sabotage(context.Background(), 54.7, 20.5, "sym-1", 100)

	if err != nil {
		t.Fatalf("sabotage: %v", err)
	}
	if res.Found {
		t.Fatalf("should skip an already-ruined plant, got %+v", res)
	}
}

func TestNearestHostile_ReportsWithoutMutating(t *testing.T) {
	repo := &fakeRepo{listed: []PowerPlant{
		{ID: "p1", OwnerID: "human-1", Lat: 54.7, Lng: 20.5, State: "degrading"},
	}}
	svc := NewService(repo, &fakeSpender{})

	res, err := svc.NearestHostile(context.Background(), 54.7, 20.5, "sym-1", 100)

	if err != nil {
		t.Fatalf("nearest hostile: %v", err)
	}
	if !res.Found || res.StationID != "p1" || res.State != "degrading" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if len(repo.degraded) != 0 {
		t.Fatal("NearestHostile must not degrade the target")
	}
}

func TestNearestHostile_SkipsOwnAndRuinedAndFellowSymbiont(t *testing.T) {
	repo := &fakeRepo{listed: []PowerPlant{
		{ID: "own", OwnerID: "sym-1", Lat: 54.7, Lng: 20.5, State: "active"},
		{ID: "ruined", OwnerID: "human-1", Lat: 54.7, Lng: 20.5, State: "ruins"},
		{ID: "fellow", OwnerID: "sym-2", Lat: 54.7, Lng: 20.5, State: "active"},
	}}
	svc := NewService(repo, &fakeSpender{}).
		WithFaction(fakeFaction{symbionts: map[string]bool{"sym-1": true, "sym-2": true}})

	res, err := svc.NearestHostile(context.Background(), 54.7, 20.5, "sym-1", 100)

	if err != nil {
		t.Fatalf("nearest hostile: %v", err)
	}
	if res.Found {
		t.Fatalf("no valid target should remain, got %+v", res)
	}
}

func TestSabotage_SkipsFellowSymbiontPlant(t *testing.T) {
	repo := &fakeRepo{listed: []PowerPlant{
		{ID: "p1", OwnerID: "sym-2", Lat: 54.7, Lng: 20.5, State: "active"},
	}}
	svc := NewService(repo, &fakeSpender{}).
		WithFaction(fakeFaction{symbionts: map[string]bool{"sym-1": true, "sym-2": true}})

	res, err := svc.Sabotage(context.Background(), 54.7, 20.5, "sym-1", 100)

	if err != nil {
		t.Fatalf("sabotage: %v", err)
	}
	if res.Found {
		t.Fatalf("should not target a fellow Symbiont's plant, got %+v", res)
	}
}
