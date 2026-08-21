package worldmigrate

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// fakeStore records call order and lets each step be scripted.
type fakeStore struct {
	snapshotRows int64
	snapshotErr  error
	quenchRows   int64
	quenchErr    error
	calls        []string
}

func (f *fakeStore) SnapshotInfection(_ context.Context, _ time.Time) (int64, error) {
	f.calls = append(f.calls, "snapshot")
	return f.snapshotRows, f.snapshotErr
}

func (f *fakeStore) QuenchOutsideSources(_ context.Context) (int64, error) {
	f.calls = append(f.calls, "quench")
	return f.quenchRows, f.quenchErr
}

type fakeGate struct {
	open bool
	err  error
}

func (f fakeGate) MigrationWindowOpen(_ context.Context) (bool, error) { return f.open, f.err }

func fixedNow() time.Time { return time.Date(2026, 8, 21, 3, 0, 0, 0, time.UTC) }

func TestRun_HappyPath(t *testing.T) {
	store := &fakeStore{snapshotRows: 10201, quenchRows: 9000}
	res, err := Run(context.Background(), store, nil, fixedNow)
	assert.NoError(t, err)
	assert.Equal(t, int64(10201), res.Snapshotted)
	assert.Equal(t, int64(9000), res.Quenched)
	// Snapshot MUST precede quench (backup before destroy).
	assert.Equal(t, []string{"snapshot", "quench"}, store.calls)
}

func TestRun_RefusesWhenSnapshotEmpty(t *testing.T) {
	store := &fakeStore{snapshotRows: 0}
	_, err := Run(context.Background(), store, nil, fixedNow)
	assert.ErrorIs(t, err, ErrNoSnapshot)
	// Quench never ran without a backup.
	assert.Equal(t, []string{"snapshot"}, store.calls)
}

func TestRun_RefusesOnSnapshotError(t *testing.T) {
	store := &fakeStore{snapshotErr: errors.New("db down")}
	_, err := Run(context.Background(), store, nil, fixedNow)
	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrNoSnapshot)
	assert.Equal(t, []string{"snapshot"}, store.calls)
}

func TestRun_QuenchErrorReportsSnapshotCount(t *testing.T) {
	store := &fakeStore{snapshotRows: 500, quenchErr: errors.New("boom")}
	res, err := Run(context.Background(), store, nil, fixedNow)
	assert.Error(t, err)
	assert.Equal(t, int64(500), res.Snapshotted)
	assert.Equal(t, []string{"snapshot", "quench"}, store.calls)
}

func TestRun_SeasonGateClosedRefusesBeforeSnapshot(t *testing.T) {
	store := &fakeStore{snapshotRows: 100}
	_, err := Run(context.Background(), store, fakeGate{open: false}, fixedNow)
	assert.ErrorIs(t, err, ErrSeasonWindowClosed)
	// Nothing touched — the gate is checked first.
	assert.Empty(t, store.calls)
}

func TestRun_SeasonGateOpenProceeds(t *testing.T) {
	store := &fakeStore{snapshotRows: 100, quenchRows: 40}
	res, err := Run(context.Background(), store, fakeGate{open: true}, fixedNow)
	assert.NoError(t, err)
	assert.Equal(t, int64(40), res.Quenched)
}

func TestRun_SeasonGateErrorAborts(t *testing.T) {
	store := &fakeStore{snapshotRows: 100}
	_, err := Run(context.Background(), store, fakeGate{err: errors.New("gate down")}, fixedNow)
	assert.Error(t, err)
	assert.Empty(t, store.calls)
}

// A second run is safe: the snapshot re-runs (extra backup rows are harmless)
// and the quench finds nothing left hot outside sources (0 rows), so re-running
// is a no-op on the world.
func TestRun_IdempotentSecondRun(t *testing.T) {
	ctx := context.Background()
	first := &fakeStore{snapshotRows: 10201, quenchRows: 9000}
	_, err := Run(ctx, first, nil, fixedNow)
	assert.NoError(t, err)

	second := &fakeStore{snapshotRows: 10201, quenchRows: 0} // nothing left to quench
	res, err := Run(ctx, second, nil, fixedNow)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), res.Quenched)
}
