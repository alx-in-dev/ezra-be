package roster

import (
	"context"
	"testing"

	"github.com/ezra-game/server/internal/canon"
	"github.com/ezra-game/server/internal/faction"
	"github.com/ezra-game/server/internal/pet"
	"github.com/ezra-game/server/internal/tower"
	"github.com/ezra-game/server/internal/unit"
)

type fakeUnits struct{ list []unit.Unit }

func (f fakeUnits) GetByPlayer(context.Context, string) ([]unit.Unit, error) { return f.list, nil }

type fakeTowers struct{ list []tower.Tower }

func (f fakeTowers) GetByOwner(context.Context, string) ([]tower.Tower, error) { return f.list, nil }

type fakePets struct{ list []pet.Pet }

func (f fakePets) GetByPlayer(context.Context, string) ([]pet.Pet, error) { return f.list, nil }

type fakePool struct {
	seeded int
	called int
}

func (f *fakePool) SetResonancePool(_ context.Context, _ string, pool int) (int, error) {
	f.called++
	f.seeded = pool
	return pool, nil
}

func TestComputePool_SumsCanonWeights(t *testing.T) {
	svc := NewService(
		fakeUnits{list: []unit.Unit{{Type: "fighter"}, {Type: "engineer"}, {Type: "scout"}, {Type: "medic"}}}, // 1+2+1+1 = 5
		fakeTowers{list: []tower.Tower{{Level: 1}, {Level: 3}}},                                                // 1+2 = 3
		fakePets{list: []pet.Pet{{Level: 3}, {Level: 1}}},                                                      // 2+0 = 2
		&fakePool{},
	)
	got, err := svc.ComputePool(context.Background(), "p1")
	if err != nil {
		t.Fatal(err)
	}
	if got != 10 {
		t.Fatalf("ComputePool = %d, want 10", got)
	}
}

func TestOnFactionChosen_SeedsForSymbiontOnly(t *testing.T) {
	ctx := context.Background()

	// Human → no seeding.
	hp := &fakePool{}
	hs := NewService(fakeUnits{}, fakeTowers{}, fakePets{}, hp)
	if err := hs.OnFactionChosen(ctx, "p1", faction.Human); err != nil {
		t.Fatal(err)
	}
	if hp.called != 0 {
		t.Fatal("human side should not seed a pool")
	}

	// Symbiont with no assets → floored at PoolStarterMin.
	sp := &fakePool{}
	ss := NewService(fakeUnits{}, fakeTowers{}, fakePets{}, sp)
	if err := ss.OnFactionChosen(ctx, "p1", faction.Symbiont); err != nil {
		t.Fatal(err)
	}
	if sp.called != 1 || sp.seeded != canon.PoolStarterMin {
		t.Fatalf("symbiont seed: called=%d seeded=%d, want 1 / %d", sp.called, sp.seeded, canon.PoolStarterMin)
	}
}
