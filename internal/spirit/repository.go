package spirit

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ezra-game/server/internal/canon"
)

const spiritColumns = `id, class, origin_lat, origin_lng, dest_lat, dest_lng,
	spawn_ts, speed_mps, dist_m, arrive_at, state, weakened_pct, tamed_by,
	target_tower_id, region_key, created_at`

// liveProjection is the SQL expression for a spirit's interpolated CURRENT point
// (the same lerp as Spirit.progress/decorate, done in-DB for spatial WHEREs).
const liveProjection = `
	ST_SetSRID(ST_MakePoint(
		origin_lng + (dest_lng - origin_lng) * LEAST(1.0, GREATEST(0.0,
			(EXTRACT(epoch FROM (now() - spawn_ts)) * speed_mps) / NULLIF(dist_m,0))),
		origin_lat + (dest_lat - origin_lat) * LEAST(1.0, GREATEST(0.0,
			(EXTRACT(epoch FROM (now() - spawn_ts)) * speed_mps) / NULLIF(dist_m,0)))
	), 4326)`

type Repository interface {
	// SpawnWave inserts new spirits and RETURNS them, so the caller can telegraph
	// each inbound wave's ETA to the target beacon's owner.
	SpawnWave(ctx context.Context, routeMaxM float64, regionBudget int, speeds [canon.SpiritMaxClass + 1]float64) ([]Spirit, error)
	GetNearby(ctx context.Context, lat, lng, radiusM float64) ([]Spirit, error)
	GetByID(ctx context.Context, id string) (*Spirit, error)
	// TakeArrivals returns spirits that have arrived at a human beacon and are
	// still wandering, marking them expired (they discharge their wave). The
	// caller applies the wave-drain/brownout to each target.
	TakeArrivals(ctx context.Context) ([]Spirit, error)
	ExpireOld(ctx context.Context, olderThan time.Duration) (int64, error)
	Weaken(ctx context.Context, id string, delta float64) (*Spirit, error)
	MarkTamed(ctx context.Context, id, playerID string) error
	// TouchedPlayers is not stored here — proximity is resolved by the field
	// service against player positions; see TouchCandidates.
	TouchCandidates(ctx context.Context, lat, lng, radiusM float64) ([]Spirit, error)
	// SeedGeyser plants at most one geyser at a random live-hive location that has
	// no geyser within minGapM (the rare class-V source, T-872). Returns 1 if a
	// geyser was created, 0 otherwise. Sparse by construction; the caller gates the
	// call behind a low per-tick probability.
	SeedGeyser(ctx context.Context, minGapM float64) (int64, error)
	// RepelWithin expires every live spirit whose CURRENT position is within
	// radiusM of a point (the Ionized Charge repellent, T-881). Returns how many
	// were repelled. Clearing the local area IS the player's protection window.
	RepelWithin(ctx context.Context, lat, lng, radiusM float64) (int64, error)
}

type PgRepository struct{ db *pgxpool.Pool }

func NewPgRepository(db *pgxpool.Pool) *PgRepository { return &PgRepository{db: db} }

// SpawnWave inserts at most one spirit per live source (hive ∪ open rift ∪ nest)
// toward the nearest human beacon within routeMaxM, capped by the per-region
// budget. Set-based, one statement (ADR-N2 / N0 anti-per-row rule). region_key
// is a coarse 0.05° grid bucket. Class comes from the source type/level.
func (r *PgRepository) SpawnWave(ctx context.Context, routeMaxM float64, regionBudget int, speeds [canon.SpiritMaxClass + 1]float64) ([]Spirit, error) {
	rows, err := r.db.Query(ctx, `
		WITH sources AS (
			-- Hives: deep Symbiont infrastructure. A max-level hive is the class-IV
			-- "wild zone" proxy until Фаза 5.3 lands (T-872, see impl_notes).
			SELECT h.geom AS geom, LEAST(4, h.level + 1) AS class
			FROM hives h WHERE h.closed_at IS NULL
			UNION ALL
			SELECT rc.geom, CASE r.type WHEN 'critical' THEN 3 WHEN 'medium' THEN 2 ELSE 1 END
			FROM rifts r JOIN cells rc ON rc.id = r.cell_id WHERE r.closed_at IS NULL
			UNION ALL
			SELECT n.geom, LEAST(2, n.level)
			FROM nests n WHERE n.collapsed_at IS NULL
			UNION ALL
			-- Geysers: the rare class-V source (T-872).
			SELECT g.geom, 5 FROM geysers g
		),
		targeted AS (
			SELECT s.geom AS origin_geom, s.class,
			       t.id AS target_tower_id, t.geom AS dest_geom,
			       ST_Distance(s.geom::geography, t.geom::geography) AS dist_m,
			       (round(ST_Y(s.geom)/0.05)::text || ':' || round(ST_X(s.geom)/0.05)::text) AS region_key
			FROM sources s
			JOIN LATERAL (
				SELECT tw.id, tw.geom FROM towers tw
				WHERE ST_DWithin(tw.geom::geography, s.geom::geography, $1::float8)
				ORDER BY tw.geom::geography <-> s.geom::geography
				LIMIT 1
			) t ON true
		)
		INSERT INTO wild_spirits
			(class, origin_lat, origin_lng, origin_geom, dest_lat, dest_lng, dest_geom,
			 spawn_ts, speed_mps, dist_m, arrive_at, region_key, target_tower_id)
		SELECT class, ST_Y(origin_geom), ST_X(origin_geom), origin_geom,
		       ST_Y(dest_geom), ST_X(dest_geom), dest_geom,
		       now(),
		       (CASE class WHEN 1 THEN $3::float8 WHEN 2 THEN $4::float8 WHEN 3 THEN $5::float8 WHEN 4 THEN $6::float8 ELSE $7::float8 END),
		       dist_m,
		       now() + make_interval(secs => dist_m / (CASE class WHEN 1 THEN $3::float8 WHEN 2 THEN $4::float8 WHEN 3 THEN $5::float8 WHEN 4 THEN $6::float8 ELSE $7::float8 END)),
		       region_key, target_tower_id
		FROM targeted
		WHERE dist_m > 0
		  AND (SELECT count(*) FROM wild_spirits w
		       WHERE w.region_key = targeted.region_key
		         AND w.state IN ('wandering','weakened')) < $2::int
		RETURNING `+spiritColumns,
		routeMaxM, regionBudget, speeds[1], speeds[2], speeds[3], speeds[4], speeds[5])
	if err != nil {
		return nil, fmt.Errorf("spawn spirit wave: %w", err)
	}
	defer rows.Close()
	list, err := pgx.CollectRows(rows, pgx.RowToStructByName[Spirit])
	if err != nil {
		return nil, fmt.Errorf("scan spawned spirits: %w", err)
	}
	return list, nil
}

// GetNearby returns live (wandering/weakened) spirits whose interpolated CURRENT
// position is within radiusM of the point, decorated with their live position.
func (r *PgRepository) GetNearby(ctx context.Context, lat, lng, radiusM float64) ([]Spirit, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+spiritColumns+` FROM wild_spirits
		WHERE state IN ('wandering','weakened')
		  AND ST_DWithin(`+liveProjection+`::geography,
		                 ST_SetSRID(ST_MakePoint($2, $1), 4326)::geography, $3)`,
		lat, lng, radiusM)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list, err := pgx.CollectRows(rows, pgx.RowToStructByName[Spirit])
	if err != nil {
		return nil, fmt.Errorf("scan nearby spirits: %w", err)
	}
	now := time.Now()
	for i := range list {
		list[i].decorate(now)
	}
	return list, nil
}

// TouchCandidates returns live spirits whose CURRENT position is within radiusM
// of a point AND within their own danger radius (the field-touch check). The
// caller filters by novice-inertness.
func (r *PgRepository) TouchCandidates(ctx context.Context, lat, lng, radiusM float64) ([]Spirit, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+spiritColumns+` FROM wild_spirits
		WHERE state IN ('wandering','weakened')
		  AND ST_DWithin(`+liveProjection+`::geography,
		                 ST_SetSRID(ST_MakePoint($2, $1), 4326)::geography, $3)`,
		lat, lng, radiusM)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list, err := pgx.CollectRows(rows, pgx.RowToStructByName[Spirit])
	if err != nil {
		return nil, fmt.Errorf("scan touch candidates: %w", err)
	}
	now := time.Now()
	for i := range list {
		list[i].decorate(now)
	}
	return list, nil
}

func (r *PgRepository) GetByID(ctx context.Context, id string) (*Spirit, error) {
	rows, err := r.db.Query(ctx, `SELECT `+spiritColumns+` FROM wild_spirits WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	s, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[Spirit])
	if err != nil {
		return nil, fmt.Errorf("spirit not found: %w", err)
	}
	s.decorate(time.Now())
	return &s, nil
}

// TakeArrivals atomically marks arrived wandering spirits expired and returns
// them (with their target beacon) so the caller applies the wave-drain. A
// weakened spirit that arrives is NOT discharged — a Symbiont is taming it.
func (r *PgRepository) TakeArrivals(ctx context.Context) ([]Spirit, error) {
	rows, err := r.db.Query(ctx, `
		UPDATE wild_spirits SET state = 'expired'
		WHERE state = 'wandering' AND arrive_at <= now()
		RETURNING `+spiritColumns)
	if err != nil {
		return nil, fmt.Errorf("take spirit arrivals: %w", err)
	}
	defer rows.Close()
	list, err := pgx.CollectRows(rows, pgx.RowToStructByName[Spirit])
	if err != nil {
		return nil, fmt.Errorf("scan arrivals: %w", err)
	}
	return list, nil
}

// ExpireOld discharges spirits long past their arrival (housekeeping).
func (r *PgRepository) ExpireOld(ctx context.Context, olderThan time.Duration) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE wild_spirits SET state = 'expired'
		WHERE state IN ('wandering','weakened')
		  AND arrive_at < now() - make_interval(secs => $1)`,
		olderThan.Seconds())
	if err != nil {
		return 0, fmt.Errorf("expire old spirits: %w", err)
	}
	return tag.RowsAffected(), nil
}

// SeedGeyser plants one geyser at a random live-hive location that has no geyser
// within minGapM (T-872 — the rare class-V source, grown slowly in deep Symbiont
// territory). Set-based single statement; the NOT EXISTS spacing guard keeps
// geysers sparse. No-op (0 rows) when every hive already has one nearby.
func (r *PgRepository) SeedGeyser(ctx context.Context, minGapM float64) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		INSERT INTO geysers (geom)
		SELECT h.geom FROM hives h
		WHERE h.closed_at IS NULL
		  AND NOT EXISTS (
		      SELECT 1 FROM geysers g
		      WHERE ST_DWithin(g.geom::geography, h.geom::geography, $1::float8))
		ORDER BY random()
		LIMIT 1`, minGapM)
	if err != nil {
		return 0, fmt.Errorf("seed geyser: %w", err)
	}
	return tag.RowsAffected(), nil
}

// RepelWithin expires every live (wandering/weakened) spirit whose interpolated
// CURRENT position is within radiusM of the point (T-881). Set-based single
// statement; mirrors the TouchCandidates geo filter.
func (r *PgRepository) RepelWithin(ctx context.Context, lat, lng, radiusM float64) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE wild_spirits SET state = 'expired'
		WHERE state IN ('wandering','weakened')
		  AND ST_DWithin(`+liveProjection+`::geography,
		                 ST_SetSRID(ST_MakePoint($2, $1), 4326)::geography, $3)`,
		lat, lng, radiusM)
	if err != nil {
		return 0, fmt.Errorf("repel spirits: %w", err)
	}
	return tag.RowsAffected(), nil
}

// Weaken adds to a spirit's weakened_pct (toward tameable) and flips it to the
// weakened state once it has taken any softening.
func (r *PgRepository) Weaken(ctx context.Context, id string, delta float64) (*Spirit, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE wild_spirits
		SET weakened_pct = LEAST(100.0, weakened_pct + $2),
		    state = 'weakened'
		WHERE id = $1 AND state IN ('wandering','weakened')`,
		id, delta)
	if err != nil {
		return nil, fmt.Errorf("weaken spirit: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, fmt.Errorf("weaken spirit: %s not in a weakenable state", id)
	}
	return r.GetByID(ctx, id)
}

// MarkTamed flips a spirit to tamed (the roster owns it thereafter).
func (r *PgRepository) MarkTamed(ctx context.Context, id, playerID string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE wild_spirits SET state = 'tamed', tamed_by = $2
		WHERE id = $1 AND state = 'weakened'`,
		id, playerID)
	if err != nil {
		return fmt.Errorf("mark spirit tamed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("mark spirit tamed: %s not weakened enough", id)
	}
	return nil
}
