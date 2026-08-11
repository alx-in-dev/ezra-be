package resonance

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository persists the world effects of the counter-wave.
type Repository interface {
	// StabilizeRadius marks every cell within radiusM of (lat,lng) as
	// permanently stabilized and zeroes their infection. Returns cells affected.
	StabilizeRadius(ctx context.Context, playerID string, lat, lng float64, radiusM int) (int, error)
	RecordWave(ctx context.Context, playerID string, lat, lng float64, radiusM, cells int) error
	WaveCount(ctx context.Context, playerID string) (int, error)
}

type PgRepository struct {
	db *pgxpool.Pool
}

func NewPgRepository(db *pgxpool.Pool) *PgRepository {
	return &PgRepository{db: db}
}

func (r *PgRepository) StabilizeRadius(ctx context.Context, playerID string, lat, lng float64, radiusM int) (int, error) {
	// Insert all in-radius cells into stabilized_cells (idempotent) and zero
	// their infection now; the infection recalc keeps them at 0 thereafter.
	tag, err := r.db.Exec(ctx, `
		WITH target AS (
			SELECT c.id FROM cells c
			WHERE ST_DWithin(
				c.geom::geography,
				ST_SetSRID(ST_MakePoint($2::float8, $3::float8), 4326)::geography,
				$4::float8)
		), ins AS (
			INSERT INTO stabilized_cells (cell_id, player_id)
			SELECT id, $1 FROM target
			ON CONFLICT (cell_id) DO NOTHING
		)
		UPDATE cells SET infection = 0, last_calculated = NOW()
		WHERE id IN (SELECT id FROM target)`,
		playerID, lng, lat, float64(radiusM),
	)
	if err != nil {
		return 0, fmt.Errorf("stabilize radius: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

func (r *PgRepository) RecordWave(ctx context.Context, playerID string, lat, lng float64, radiusM, cells int) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO resonance_waves (player_id, lat, lng, radius_m, cells_stabilized) VALUES ($1,$2,$3,$4,$5)`,
		playerID, lat, lng, radiusM, cells)
	if err != nil {
		return fmt.Errorf("record wave: %w", err)
	}
	return nil
}

func (r *PgRepository) WaveCount(ctx context.Context, playerID string) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM resonance_waves WHERE player_id = $1`, playerID).Scan(&n)
	return n, err
}
