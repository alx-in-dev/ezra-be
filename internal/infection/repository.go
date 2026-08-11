package infection

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ezra-game/server/internal/canon"
)

// Repository performs set-based infection recalculation in the database.
type Repository interface {
	BatchRecalculate(ctx context.Context) (int64, error)
	AdvanceTide(ctx context.Context) (int64, error)
}

// domeSuppressionPerHour returns the dome suppression rate, overridable via
// DOME_SUPPRESSION_PER_HOUR so dev environments can clear infected zones in
// minutes instead of overnight (same pattern as MAX_SPEED_KMH).
func domeSuppressionPerHour() float64 {
	if v := os.Getenv("DOME_SUPPRESSION_PER_HOUR"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			return f
		}
	}
	return canon.DomeSuppressionPerHour
}

// PgRepository implements Repository using a single PostGIS UPDATE.
type PgRepository struct {
	db *pgxpool.Pool
}

func NewPgRepository(db *pgxpool.Pool) *PgRepository {
	return &PgRepository{db: db}
}

// BatchRecalculate applies one infection tick to every cell in a single
// statement, mirroring Service.RecalculateCell semantics (growth + neighbor
// bonus + rift effect - tower effect, scaled to the 5-minute interval and
// clamped to [0, 100]).
//
// Unlike the legacy per-cell loop, all neighbor reads come from the statement
// snapshot, so the result does not depend on cell processing order.
func (r *PgRepository) BatchRecalculate(ctx context.Context) (int64, error) {
	const query = `
		UPDATE cells AS c SET
			infection = CASE
			WHEN EXISTS (SELECT 1 FROM stabilized_cells sc WHERE sc.cell_id = c.id)
				THEN 0.0
			-- Dome suppresses domed cells UNLESS a Symbiont has pierced them (#6
			-- T-762): a pierced cell falls through to growth until its TTL lapses.
			WHEN EXISTS (SELECT 1 FROM domed_cells dc WHERE dc.cell_id = c.id)
				AND (c.pierced_until IS NULL OR c.pierced_until <= now())
				THEN GREATEST(0.0, c.infection - $11::float8)
			ELSE GREATEST(0.0, LEAST(100.0,
				c.infection
				+ (
					$1::float8
					+ $2::float8 * (
						SELECT COUNT(*)::float8 FROM cells n
						WHERE n.id <> c.id
						  AND n.infection > $3::float8
						  AND ST_DWithin(c.geom::geography, n.geom::geography, $4::float8)
					)
					-- Rift pressure scales with intensity (×1.0 at 1 → ×1.5 at 100):
				-- aged / amplified / empowered rifts push harder.
				+ COALESCE((
						SELECT SUM(
							CASE r.type
								WHEN 'minor'    THEN $5::float8
								WHEN 'medium'   THEN $6::float8
								WHEN 'critical' THEN $7::float8
								ELSE 0.0
							END
							* (1.0 + (LEAST(GREATEST(r.intensity, 1), 100) - 1)::float8 / 198.0))
						FROM rifts r
						JOIN cells rc ON rc.id = r.cell_id
						WHERE r.closed_at IS NULL
						  AND ST_DWithin(c.geom::geography, rc.geom::geography, $8::float8)
					), 0.0)
				) * $9::float8
				-- Brownout beacons suppress at half strength (beacon_network_dome.md §5).
				- COALESCE((
					SELECT SUM(t.effect_per_hour
						* (CASE WHEN t.brownout THEN 0.5 ELSE 1.0 END)
						* (1.0 - $10::float8 * (ST_Distance(c.geom::geography, t.geom::geography) / t.radius_m)))
					FROM towers t
					WHERE ST_DWithin(c.geom::geography, t.geom::geography, t.radius_m)
				), 0.0) * $9::float8
				-- Civic Spire: a megastructure suppresses infection region-wide
				-- for everyone in its radius (megastructure_spire.md). Active =
				-- full bonus; degrading = half; ruins = none. MAX so overlapping
				-- Spires take the strongest, not the sum.
				- COALESCE((
						SELECT MAX(CASE sp.state
							WHEN 'active'    THEN $12::float8
							WHEN 'degrading' THEN $12::float8 * 0.5
							ELSE 0.0 END)
						FROM spires sp
						WHERE sp.state IN ('active', 'degrading')
						  AND ST_DWithin(c.geom::geography, sp.geom::geography, sp.radius_m)
					), 0.0) * $9::float8
			)) END,
			last_calculated = NOW()`

	tag, err := r.db.Exec(ctx, query,
		baseGrowthPerHour,
		neighborBonus,
		neighborThreshold,
		neighborRadiusM,
		riftBonusPerHour["minor"],
		riftBonusPerHour["medium"],
		riftBonusPerHour["critical"],
		maxRiftSearchM,
		recalcIntervalMin/minutesPerHour,
		towerFalloff,
		domeSuppressionPerHour()*(recalcIntervalMin/minutesPerHour),
		spireSuppressionPerHour,
	)
	if err != nil {
		return 0, fmt.Errorf("batch recalculate infection: %w", err)
	}
	return tag.RowsAffected(), nil
}

// AdvanceTide is the D2 active antagonist: instead of growing uniformly, the
// infection channels toward player networks. A cell is pushed when it sits
// within tideRange of a beacon AND has an infected neighbour that is FARTHER
// from that beacon than the cell itself — so the infected frontier creeps
// inward toward the beacon, corridor by corridor (mirror of the dome reaching
// out, §7.4). Stabilized/domed cells are immune. Returns cells advanced.
func (r *PgRepository) AdvanceTide(ctx context.Context) (int64, error) {
	const query = `
		UPDATE cells AS c SET
			infection = LEAST(100.0, c.infection + $1::float8),
			last_calculated = NOW()
		WHERE c.infection < $2::float8
		  AND NOT EXISTS (SELECT 1 FROM stabilized_cells sc WHERE sc.cell_id = c.id)
		  AND NOT EXISTS (SELECT 1 FROM domed_cells dc WHERE dc.cell_id = c.id)
		  AND EXISTS (
			SELECT 1 FROM towers t
			WHERE ST_DWithin(c.geom::geography, t.geom::geography, $3::float8)
			  AND EXISTS (
				SELECT 1 FROM cells n
				WHERE n.id <> c.id
				  AND n.infection > $4::float8
				  AND ST_DWithin(c.geom::geography, n.geom::geography, $5::float8)
				  AND ST_Distance(n.geom::geography, t.geom::geography)
				      > ST_Distance(c.geom::geography, t.geom::geography)
			  )
		  )`
	tag, err := r.db.Exec(ctx, query,
		tideStepPerTick,
		tideAdvanceCeiling,
		tideRangeM,
		tideFrontierThreshold,
		neighborRadiusM,
	)
	if err != nil {
		return 0, fmt.Errorf("advance tide: %w", err)
	}
	return tag.RowsAffected(), nil
}
