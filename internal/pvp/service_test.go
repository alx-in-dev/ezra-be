package pvp

import (
	"context"
	"testing"

	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/internal/push"
	"github.com/ezra-game/server/internal/rift"
	"github.com/stretchr/testify/assert"
)

type fakeRepo struct {
	defender   string
	defenderOK bool
	raised     bool
}

func (f *fakeRepo) NearbyEnemyDomes(ctx context.Context, a string, lat, lng, r float64) ([]Target, error) {
	return []Target{{CellID: "c1", DefenderID: "d1", DefenderName: "bob"}}, nil
}
func (f *fakeRepo) DefenderOf(ctx context.Context, cellID, attackerID string) (string, bool, error) {
	return f.defender, f.defenderOK, nil
}
func (f *fakeRepo) RaiseInfection(ctx context.Context, cellID string, to float64) error {
	f.raised = true
	return nil
}

type fakeFaction struct{ sym bool }

func (f fakeFaction) IsSymbiont(ctx context.Context, p string) (bool, error) { return f.sym, nil }

type fakeRift struct{ spawned int }

func (f *fakeRift) ForceSpawn(ctx context.Context, cellID, t string) (*rift.Rift, error) {
	f.spawned++
	return &rift.Rift{ID: "r1", CellID: cellID, Type: t}, nil
}

type fakeRes struct{}

func (fakeRes) Spend(ctx context.Context, p string, e, m int) (*player.Player, error) {
	return &player.Player{}, nil
}

type fakePush struct{ sent int }

func (f *fakePush) Send(ctx context.Context, msg push.PushMessage) error { f.sent++; return nil }

type fakePlayers struct{}

func (fakePlayers) GetByID(ctx context.Context, id string) (*player.Player, error) {
	return &player.Player{ID: id, Username: "u_" + id}, nil
}

func TestTargets_GatedToSymbiont(t *testing.T) {
	ctx := context.Background()
	svc := NewService(&fakeRepo{}, fakeFaction{sym: false}, &fakeRift{}, fakeRes{}, &fakePush{}, fakePlayers{})
	_, err := svc.Targets(ctx, "a1", 54.7, 20.5)
	assert.Error(t, err)

	svc2 := NewService(&fakeRepo{}, fakeFaction{sym: true}, &fakeRift{}, fakeRes{}, &fakePush{}, fakePlayers{})
	ts, err := svc2.Targets(ctx, "a1", 54.7, 20.5)
	assert.NoError(t, err)
	assert.Len(t, ts, 1)
}

func TestBreach_SpawnsRiftAndPushes(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{defender: "d1", defenderOK: true}
	pushSvc := &fakePush{}
	rf := &fakeRift{}
	svc := NewService(repo, fakeFaction{sym: true}, rf, fakeRes{}, pushSvc, fakePlayers{})

	res, err := svc.Breach(ctx, "a1", "c1")
	assert.NoError(t, err)
	assert.Equal(t, "r1", res.RiftID)
	assert.Equal(t, 1, rf.spawned)
	assert.True(t, repo.raised)
	assert.Equal(t, 1, pushSvc.sent) // defender alerted
}

func TestBreach_RejectsNonSymbiontAndOwnDome(t *testing.T) {
	ctx := context.Background()
	// non-symbiont
	svc := NewService(&fakeRepo{defender: "d1", defenderOK: true}, fakeFaction{sym: false}, &fakeRift{}, fakeRes{}, &fakePush{}, fakePlayers{})
	_, err := svc.Breach(ctx, "a1", "c1")
	assert.Error(t, err)

	// own dome / undomed: DefenderOf excludes the attacker, so it returns ok=false
	// when the cell is only the attacker's (or undomed) — breach must be rejected.
	svc2 := NewService(&fakeRepo{defenderOK: false}, fakeFaction{sym: true}, &fakeRift{}, fakeRes{}, &fakePush{}, fakePlayers{})
	_, err = svc2.Breach(ctx, "a1", "c1")
	assert.Error(t, err)
}
