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
	return nil, nil
}
func (f *fakeRepo) AdvanceLifecycle(context.Context, int, int) error { return nil }
func (f *fakeRepo) Upkeep(context.Context, string, float64, float64, float64) (bool, error) {
	return true, nil
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
