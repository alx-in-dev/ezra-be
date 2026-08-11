package resonance

import (
	"context"
	"testing"

	"github.com/ezra-game/server/internal/item"
	"github.com/ezra-game/server/internal/player"
	"github.com/stretchr/testify/assert"
)

type fakeItems struct {
	owned    map[string]int
	granted  []string
}

func (f *fakeItems) ListByPlayer(ctx context.Context, playerID string) ([]item.Item, error) {
	var out []item.Item
	for k, q := range f.owned {
		out = append(out, item.Item{ItemType: item.TypeResonanceSignature, Variant: k, Qty: q})
	}
	return out, nil
}
func (f *fakeItems) Grant(ctx context.Context, playerID, itemType, variant string, delta int) (int, error) {
	f.granted = append(f.granted, variant)
	f.owned[variant] += delta
	return f.owned[variant], nil
}

type fakePlayers struct{}

func (fakePlayers) GetByID(ctx context.Context, id string) (*player.Player, error) {
	return &player.Player{ID: id, Position: player.Position{Lat: 54.7, Lng: 20.5}}, nil
}

type fakeRepo struct {
	stabilized int
	waves      int
}

func (r *fakeRepo) StabilizeRadius(ctx context.Context, playerID string, lat, lng float64, radiusM int) (int, error) {
	r.stabilized = 42
	return 42, nil
}
func (r *fakeRepo) RecordWave(ctx context.Context, playerID string, lat, lng float64, radiusM, cells int) error {
	r.waves++
	return nil
}
func (r *fakeRepo) WaveCount(ctx context.Context, playerID string) (int, error) { return r.waves, nil }

func fullSet() map[string]int {
	m := map[string]int{}
	for _, k := range item.ResonanceSpectra {
		m[k] = 1
	}
	return m
}

func TestStatus_IncompleteAndComplete(t *testing.T) {
	ctx := context.Background()
	// Missing one spectrum.
	partial := fullSet()
	delete(partial, item.ResonanceSpectra[0])
	svc := NewService(&fakeItems{owned: partial}, fakePlayers{}, &fakeRepo{})
	st, err := svc.Status(ctx, "p1")
	assert.NoError(t, err)
	assert.False(t, st.Complete)
	assert.Equal(t, len(item.ResonanceSpectra)-1, st.Collected)

	// Full set.
	svc2 := NewService(&fakeItems{owned: fullSet()}, fakePlayers{}, &fakeRepo{})
	st2, _ := svc2.Status(ctx, "p1")
	assert.True(t, st2.Complete)
	assert.Equal(t, st2.Total, st2.Collected)
}

func TestActivate_RequiresFullSet(t *testing.T) {
	ctx := context.Background()
	svc := NewService(&fakeItems{owned: map[string]int{"ember": 1}}, fakePlayers{}, &fakeRepo{})
	_, err := svc.Activate(ctx, "p1")
	assert.Error(t, err)
}

func TestActivate_ConsumesAndStabilizes(t *testing.T) {
	ctx := context.Background()
	items := &fakeItems{owned: fullSet()}
	repo := &fakeRepo{}
	svc := NewService(items, fakePlayers{}, repo)
	res, err := svc.Activate(ctx, "p1")
	assert.NoError(t, err)
	assert.Equal(t, 42, res.CellsStabilized)
	assert.Equal(t, 1, res.WaveNumber)
	// One signature of every spectrum consumed.
	assert.Len(t, items.granted, len(item.ResonanceSpectra))
	for _, k := range item.ResonanceSpectra {
		assert.Equal(t, 0, items.owned[k])
	}
}
