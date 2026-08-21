// Package worldmigrate holds the one-shot N1 localization migration: it snapshots
// the world's infection into infection_snapshots (the T-824 backup table) and
// then quenches infection everywhere outside the reach of a live source (open
// hives ∪ open rifts). Distant rifts are grandfathered — they stay open sources,
// so their (possibly expanded) footprint is spared, not cleansed.
//
// It is deliberately NOT an asynq task: an enqueued task can fire by accident,
// but this is destructive and must be run once, on purpose, from the cmd binary
// with the season window open (T-826) and the snapshot guard satisfied.
package worldmigrate

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// ErrNoSnapshot aborts the migration when the guarded first step wrote no backup
// rows — never run the destructive quench without a fresh snapshot to roll back
// to (T-824/T-825).
var ErrNoSnapshot = errors.New("worldmigrate: refusing to quench — no infection snapshot was taken")

// ErrSeasonWindowClosed aborts when a season gate is wired and reports that a
// Faction-War season is still active (T-826): the localization must land
// strictly between seasons so the first post-migration scoring snapshot is
// post-wipe.
var ErrSeasonWindowClosed = errors.New("worldmigrate: refusing to run — Faction-War season window is not open")

// Store is the persistence the migration drives. Both operations must run inside
// ONE transaction (the caller wraps them) so a failed quench rolls back the
// snapshot too — the run is all-or-nothing.
type Store interface {
	// SnapshotInfection copies every cell's current infection into
	// infection_snapshots at ts and returns rows written.
	SnapshotInfection(ctx context.Context, ts time.Time) (int64, error)
	// QuenchOutsideSources zeroes infection on every cell that has infection > 0
	// and sits outside every live source's reach, returning rows quenched. The
	// source set is resolved inside the same statement, so rifts open at the
	// migration moment are included; re-running only touches cells still hot, so
	// it is idempotent.
	QuenchOutsideSources(ctx context.Context) (int64, error)
}

// SeasonGate reports whether the Faction-War season window is open for the
// migration (T-826). Optional: a nil gate skips the check (the snapshot guard
// still applies).
type SeasonGate interface {
	MigrationWindowOpen(ctx context.Context) (bool, error)
}

// Result reports what the run did.
type Result struct {
	Snapshotted int64
	Quenched    int64
}

// Run executes the localization: (optional) season gate → snapshot → snapshot
// guard → quench. Atomicity is the caller's job (wrap in a tx, commit only on a
// nil error). The order is fixed and every refusal returns before the
// destructive quench, so nothing is cleansed without a backup.
func Run(ctx context.Context, store Store, gate SeasonGate, now func() time.Time) (Result, error) {
	if gate != nil {
		open, err := gate.MigrationWindowOpen(ctx)
		if err != nil {
			return Result{}, fmt.Errorf("worldmigrate: season gate: %w", err)
		}
		if !open {
			return Result{}, ErrSeasonWindowClosed
		}
	}

	ts := now()
	snapshotted, err := store.SnapshotInfection(ctx, ts)
	if err != nil {
		return Result{}, fmt.Errorf("worldmigrate: snapshot: %w", err)
	}
	if snapshotted == 0 {
		return Result{}, ErrNoSnapshot
	}

	quenched, err := store.QuenchOutsideSources(ctx)
	if err != nil {
		return Result{Snapshotted: snapshotted}, fmt.Errorf("worldmigrate: quench: %w", err)
	}
	return Result{Snapshotted: snapshotted, Quenched: quenched}, nil
}

// querier is the subset of pgx used here; both *pgxpool.Pool and pgx.Tx satisfy
// it, so the same PgStore works standalone or inside a transaction.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// PgStore is the PostgreSQL-backed Store.
type PgStore struct {
	q querier
}

// NewPgStore binds a Store to a querier (pass the transaction so snapshot +
// quench commit atomically).
func NewPgStore(q querier) PgStore { return PgStore{q: q} }

func (s PgStore) SnapshotInfection(ctx context.Context, ts time.Time) (int64, error) {
	tag, err := s.q.Exec(ctx, `
		INSERT INTO infection_snapshots (cell_id, infection, snapshot_at)
		SELECT id, infection, $1 FROM cells`, ts)
	if err != nil {
		return 0, fmt.Errorf("snapshot infection: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (s PgStore) QuenchOutsideSources(ctx context.Context) (int64, error) {
	// Source reach mirrors infection's localization: a hive by its level radius
	// (200/350/500), an open rift by GREATEST(200, its expanded footprint) so a
	// rift that already grew past the 200m growth reach keeps its whole zone.
	// active_sources is resolved inside this one statement ⇒ rifts open at the
	// migration moment are included, and distant rifts stay sources (spared),
	// which is exactly the grandfathering the migration requires.
	tag, err := s.q.Exec(ctx, `
		WITH active_sources AS MATERIALIZED (
			SELECT (h.geom::geography) AS geog,
				CASE h.level WHEN 1 THEN 200.0 WHEN 2 THEN 350.0 ELSE 500.0 END AS radius_m
			FROM hives h WHERE h.closed_at IS NULL
			UNION ALL
			SELECT (rc.geom::geography) AS geog,
				GREATEST(200.0, r.radius_cells::float8 * 50.0 * 1.4142) AS radius_m
			FROM rifts r JOIN cells rc ON rc.id = r.cell_id
			WHERE r.closed_at IS NULL
		)
		UPDATE cells c SET infection = 0.0, last_calculated = now()
		WHERE c.infection > 0.0
		  AND NOT EXISTS (
			SELECT 1 FROM active_sources s
			WHERE ST_DWithin(c.geom::geography, s.geog, s.radius_m)
		  )`)
	if err != nil {
		return 0, fmt.Errorf("quench outside sources: %w", err)
	}
	return tag.RowsAffected(), nil
}
