package station

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository persists power plants.
type Repository interface {
	Create(ctx context.Context, p *PowerPlant) error
	CountInRadius(ctx context.Context, lat, lng, radiusM float64) (int, error)
	NearestDistanceM(ctx context.Context, lat, lng float64) (float64, bool, error)
	// StationBonusInRadius sums the capacity bonus of ACTIVE plants within
	// radiusM of (lat,lng). Drives beacon-area capacity. Satisfies the optional
	// interface the tower service consumes.
	StationBonusInRadius(ctx context.Context, lat, lng, radiusM float64) (int, error)
	ListInRadius(ctx context.Context, lat, lng, radiusM float64) ([]PowerPlant, error)
	// AdvanceLifecycle moves plants active→degrading→ruins by upkeep age.
	AdvanceLifecycle(ctx context.Context, activeDays, degradeDays int) error
	// DegradeOneStep forces a single plant one step down its lifecycle
	// (active→degrading, degrading→ruins) regardless of upkeep age — the
	// Symbiont Sabotage strike (T-804). Returns the resulting state.
	DegradeOneStep(ctx context.Context, id string) (string, error)
	// Upkeep resets the nearest non-ruins plant the player owns within radiusM
	// to active. Returns false if none is in range.
	Upkeep(ctx context.Context, playerID string, lat, lng, radiusM float64) (bool, error)
}

type PgRepository struct {
	db *pgxpool.Pool
}

func NewPgRepository(db *pgxpool.Pool) *PgRepository {
	return &PgRepository{db: db}
}

func (r *PgRepository) Create(ctx context.Context, p *PowerPlant) error {
	const q = `INSERT INTO power_plants (owner_id, lat, lng, geom, capacity_bonus)
		VALUES ($1, $2, $3, ST_SetSRID(ST_MakePoint($3, $2), 4326), $4)
		RETURNING id, created_at, state`
	return r.db.QueryRow(ctx, q, p.OwnerID, p.Lat, p.Lng, p.CapacityBonus).
		Scan(&p.ID, &p.CreatedAt, &p.State)
}

func (r *PgRepository) CountInRadius(ctx context.Context, lat, lng, radiusM float64) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `
		SELECT count(*) FROM power_plants
		WHERE state <> 'ruins'
		  AND ST_DWithin(geom::geography, ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography, $3)`,
		lng, lat, radiusM).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count plants in radius: %w", err)
	}
	return n, nil
}

func (r *PgRepository) NearestDistanceM(ctx context.Context, lat, lng float64) (float64, bool, error) {
	var d float64
	err := r.db.QueryRow(ctx, `
		SELECT ST_Distance(geom::geography, ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography) AS dist
		FROM power_plants WHERE state <> 'ruins'
		ORDER BY geom <-> ST_SetSRID(ST_MakePoint($1, $2), 4326) LIMIT 1`, lng, lat).Scan(&d)
	if err != nil {
		if err == pgx.ErrNoRows {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("nearest plant: %w", err)
	}
	return d, true, nil
}

func (r *PgRepository) StationBonusInRadius(ctx context.Context, lat, lng, radiusM float64) (int, error) {
	var sum int
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(capacity_bonus), 0) FROM power_plants
		WHERE state = 'active'
		  AND ST_DWithin(geom::geography, ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography, $3)`,
		lng, lat, radiusM).Scan(&sum)
	if err != nil {
		return 0, fmt.Errorf("station bonus in radius: %w", err)
	}
	return sum, nil
}

func (r *PgRepository) AdvanceLifecycle(ctx context.Context, activeDays, degradeDays int) error {
	// active → degrading once upkeep is older than activeDays.
	if _, err := r.db.Exec(ctx, `
		UPDATE power_plants SET state = 'degrading'
		WHERE state = 'active' AND last_upkeep_at < now() - make_interval(days => $1)`, activeDays); err != nil {
		return fmt.Errorf("station degrade: %w", err)
	}
	// degrading → ruins once upkeep is older than activeDays+degradeDays.
	if _, err := r.db.Exec(ctx, `
		UPDATE power_plants SET state = 'ruins'
		WHERE state = 'degrading' AND last_upkeep_at < now() - make_interval(days => $1)`,
		activeDays+degradeDays); err != nil {
		return fmt.Errorf("station ruin: %w", err)
	}
	return nil
}

func (r *PgRepository) Upkeep(ctx context.Context, playerID string, lat, lng, radiusM float64) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE power_plants SET state = 'active', last_upkeep_at = now()
		WHERE id = (
			SELECT id FROM power_plants
			WHERE owner_id = $1 AND state <> 'ruins'
			  AND ST_DWithin(geom::geography, ST_SetSRID(ST_MakePoint($3, $2), 4326)::geography, $4)
			ORDER BY geom <-> ST_SetSRID(ST_MakePoint($3, $2), 4326) LIMIT 1)`,
		playerID, lat, lng, radiusM)
	if err != nil {
		return false, fmt.Errorf("station upkeep: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (r *PgRepository) DegradeOneStep(ctx context.Context, id string) (string, error) {
	var state string
	err := r.db.QueryRow(ctx, `
		UPDATE power_plants
		SET state = CASE state WHEN 'active' THEN 'degrading' ELSE 'ruins' END
		WHERE id = $1
		RETURNING state`, id).Scan(&state)
	if err != nil {
		return "", fmt.Errorf("degrade station: %w", err)
	}
	return state, nil
}

func (r *PgRepository) ListInRadius(ctx context.Context, lat, lng, radiusM float64) ([]PowerPlant, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, owner_id::text, lat, lng, capacity_bonus, state, created_at
		FROM power_plants
		WHERE ST_DWithin(geom::geography, ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography, $3)`,
		lng, lat, radiusM)
	if err != nil {
		return nil, fmt.Errorf("list plants: %w", err)
	}
	defer rows.Close()
	var out []PowerPlant
	for rows.Next() {
		var p PowerPlant
		if err := rows.Scan(&p.ID, &p.OwnerID, &p.Lat, &p.Lng, &p.CapacityBonus, &p.State, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
