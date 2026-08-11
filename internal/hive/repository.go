package hive

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository persists hives and applies their world effects.
type Repository interface {
	Create(ctx context.Context, h *Hive) error
	GetByID(ctx context.Context, id string) (*Hive, error)
	GetOpen(ctx context.Context) ([]Hive, error)
	GetNearby(ctx context.Context, lat, lng, radiusM float64) ([]Hive, error)
	Close(ctx context.Context, id string) error
	// ReduceIntensity lowers a hive's vitality by delta (floored at 0) and
	// returns the new value.
	ReduceIntensity(ctx context.Context, id string, delta int) (int, error)
	// CleanseRegion knocks infection down within the hive's radius (a partial
	// cleanse, not the permanent resonance stabilization) and returns cells hit.
	CleanseRegion(ctx context.Context, h *Hive, toLevel float64) (int, error)
	// PulseInfection raises infection on non-dome, non-stabilized cells within
	// the hive's radius and returns how many cells were affected.
	PulseInfection(ctx context.Context, h *Hive, amount float64) (int, error)
	// Empower raises a hive's intensity by delta, leveling it up (to a cap) when
	// intensity tops out — the Symbiont's "разливать" verb. Returns new state.
	Empower(ctx context.Context, hiveID string, delta int) (intensity, level int, err error)
	// HottestCellInZone returns the most-infected open cell within the hive's
	// radius that has no rift yet (a candidate to seed a rift on).
	HottestCellInZone(ctx context.Context, h *Hive, minInfection float64) (cellID string, lat, lng, infection float64, ok bool, err error)
	// SeedCandidate finds the most-infected cell with no open hive within
	// excludeM and no dome (a candidate to grow a new hive). ok=false if none.
	SeedCandidate(ctx context.Context, minInfection, excludeM float64) (cellID string, lat, lng float64, ok bool, err error)
}

type PgRepository struct {
	db *pgxpool.Pool
}

func NewPgRepository(db *pgxpool.Pool) *PgRepository {
	return &PgRepository{db: db}
}

func (r *PgRepository) Create(ctx context.Context, h *Hive) error {
	return r.db.QueryRow(ctx, `
		INSERT INTO hives (cell_id, lat, lng, geom, level, intensity)
		VALUES ($1, $2, $3, ST_SetSRID(ST_MakePoint($3, $2), 4326), $4, $5)
		RETURNING id, created_at`,
		h.CellID, h.Lat, h.Lng, h.Level, h.Intensity,
	).Scan(&h.ID, &h.CreatedAt)
}

func (r *PgRepository) GetByID(ctx context.Context, id string) (*Hive, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, cell_id, lat, lng, level, intensity, created_at, closed_at FROM hives WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	h, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[Hive])
	if err != nil {
		return nil, fmt.Errorf("hive not found: %w", err)
	}
	h.decorate()
	return &h, nil
}

func (r *PgRepository) ReduceIntensity(ctx context.Context, id string, delta int) (int, error) {
	var intensity int
	err := r.db.QueryRow(ctx,
		`UPDATE hives SET intensity = GREATEST(0, intensity - $2) WHERE id = $1 RETURNING intensity`,
		id, delta).Scan(&intensity)
	if err != nil {
		return 0, fmt.Errorf("reduce intensity: %w", err)
	}
	return intensity, nil
}

func (r *PgRepository) CleanseRegion(ctx context.Context, h *Hive, toLevel float64) (int, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE cells AS c SET infection = LEAST(c.infection, $1::float8), last_calculated = NOW()
		WHERE ST_DWithin(c.geom::geography, ST_SetSRID(ST_MakePoint($3, $2), 4326)::geography, $4::float8)
		  AND c.infection > $1::float8`,
		toLevel, h.Lat, h.Lng, Config(h.Level).RadiusM)
	if err != nil {
		return 0, fmt.Errorf("cleanse region: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

func (r *PgRepository) GetOpen(ctx context.Context) ([]Hive, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, cell_id, lat, lng, level, intensity, created_at, closed_at FROM hives WHERE closed_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	hives, err := pgx.CollectRows(rows, pgx.RowToStructByName[Hive])
	if err != nil {
		return nil, fmt.Errorf("scan hives: %w", err)
	}
	for i := range hives {
		hives[i].decorate()
	}
	return hives, nil
}

func (r *PgRepository) GetNearby(ctx context.Context, lat, lng, radiusM float64) ([]Hive, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, cell_id, lat, lng, level, intensity, created_at, closed_at
		FROM hives
		WHERE closed_at IS NULL
		  AND ST_DWithin(geom::geography, ST_SetSRID(ST_MakePoint($2, $1), 4326)::geography, $3)`,
		lat, lng, radiusM)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	hives, err := pgx.CollectRows(rows, pgx.RowToStructByName[Hive])
	if err != nil {
		return nil, fmt.Errorf("scan nearby hives: %w", err)
	}
	for i := range hives {
		hives[i].decorate()
	}
	return hives, nil
}

func (r *PgRepository) Close(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `UPDATE hives SET closed_at = now() WHERE id = $1 AND closed_at IS NULL`, id)
	return err
}

func (r *PgRepository) PulseInfection(ctx context.Context, h *Hive, amount float64) (int, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE cells AS c SET infection = LEAST(100.0, c.infection + $1::float8), last_calculated = NOW()
		WHERE ST_DWithin(c.geom::geography, ST_SetSRID(ST_MakePoint($3, $2), 4326)::geography, $4::float8)
		  AND NOT EXISTS (SELECT 1 FROM stabilized_cells sc WHERE sc.cell_id = c.id)
		  AND NOT EXISTS (SELECT 1 FROM domed_cells dc WHERE dc.cell_id = c.id)`,
		amount, h.Lat, h.Lng, Config(h.Level).RadiusM)
	if err != nil {
		return 0, fmt.Errorf("pulse infection: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

func (r *PgRepository) Empower(ctx context.Context, hiveID string, delta int) (int, int, error) {
	const maxLevel = 3
	var intensity, level int
	err := r.db.QueryRow(ctx, `
		UPDATE hives SET
			level = CASE WHEN intensity + $2 >= 100 AND level < $3 THEN level + 1 ELSE level END,
			intensity = CASE WHEN intensity + $2 >= 100 AND level < $3 THEN 50
			                 ELSE LEAST(100, intensity + $2) END
		WHERE id = $1 AND closed_at IS NULL
		RETURNING intensity, level`,
		hiveID, delta, maxLevel).Scan(&intensity, &level)
	if err != nil {
		return 0, 0, fmt.Errorf("empower hive: %w", err)
	}
	return intensity, level, nil
}

func (r *PgRepository) HottestCellInZone(ctx context.Context, h *Hive, minInfection float64) (string, float64, float64, float64, bool, error) {
	var id string
	var lat, lng, infection float64
	err := r.db.QueryRow(ctx, `
		SELECT c.id, c.lat, c.lng, c.infection FROM cells c
		WHERE c.rift_id IS NULL
		  AND c.infection >= $1::float8
		  AND ST_DWithin(c.geom::geography, ST_SetSRID(ST_MakePoint($3, $2), 4326)::geography, $4::float8)
		ORDER BY c.infection DESC LIMIT 1`,
		minInfection, h.Lat, h.Lng, Config(h.Level).RadiusM).Scan(&id, &lat, &lng, &infection)
	if err == pgx.ErrNoRows {
		return "", 0, 0, 0, false, nil
	}
	if err != nil {
		return "", 0, 0, 0, false, fmt.Errorf("hottest cell: %w", err)
	}
	return id, lat, lng, infection, true, nil
}

func (r *PgRepository) SeedCandidate(ctx context.Context, minInfection, excludeM float64) (string, float64, float64, bool, error) {
	var id string
	var lat, lng float64
	err := r.db.QueryRow(ctx, `
		SELECT c.id, c.lat, c.lng FROM cells c
		WHERE c.infection >= $1::float8
		  AND NOT EXISTS (SELECT 1 FROM domed_cells dc WHERE dc.cell_id = c.id)
		  AND NOT EXISTS (SELECT 1 FROM stabilized_cells sc WHERE sc.cell_id = c.id)
		  AND NOT EXISTS (
			SELECT 1 FROM hives hv
			WHERE hv.closed_at IS NULL
			  AND ST_DWithin(c.geom::geography, hv.geom::geography, $2::float8)
		  )
		ORDER BY c.infection DESC LIMIT 1`,
		minInfection, excludeM).Scan(&id, &lat, &lng)
	if err == pgx.ErrNoRows {
		return "", 0, 0, false, nil
	}
	if err != nil {
		return "", 0, 0, false, fmt.Errorf("seed candidate: %w", err)
	}
	return id, lat, lng, true, nil
}
