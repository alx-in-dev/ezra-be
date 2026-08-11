package cell

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	GetAll(ctx context.Context) ([]Cell, error)
	GetInRadius(ctx context.Context, lat, lng, radiusM float64) ([]Cell, error)
	GetByID(ctx context.Context, id string) (*Cell, error)
	GetNeighbors(ctx context.Context, cellID string) ([]Cell, error)
	Update(ctx context.Context, id string, infection float64) error
	SetTowerID(ctx context.Context, cellID string, towerID *string) error
	SetRiftID(ctx context.Context, cellID string, riftID *string) error
	StateSignature(ctx context.Context, lat, lng, radiusM float64) (string, error)
}

type PgRepository struct {
	db *pgxpool.Pool
}

func NewPgRepository(db *pgxpool.Pool) *PgRepository {
	return &PgRepository{db: db}
}

// StateSignature is a cheap fingerprint of everything the /map/cells response
// is built from: cells in radius (count + newest recalc) and towers (count +
// newest change). The infection job bumps last_calculated every tick, towers
// change on place/upgrade/remove — so between those events the signature is
// stable and the handler can answer "not modified" without building 500KB of
// JSON. Towers live in another package; the one-line subquery here is the
// pragmatic alternative to threading a second repository in.
func (r *PgRepository) StateSignature(ctx context.Context, lat, lng, radiusM float64) (string, error) {
	const q = `
		SELECT (SELECT count(*)::text || '-' || coalesce(extract(epoch from max(last_calculated))::text, '0')
		        FROM cells
		        WHERE ST_DWithin(geom::geography, ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography, $3))
		    || '|' ||
		       (SELECT count(*)::text || '-' || coalesce(extract(epoch from max(greatest(installed_at, upgraded_at)))::text, '0')
		        FROM towers)`
	var sig string
	if err := r.db.QueryRow(ctx, q, lng, lat, radiusM).Scan(&sig); err != nil {
		return "", fmt.Errorf("state signature: %w", err)
	}
	return sig, nil
}

func (r *PgRepository) GetAll(ctx context.Context) ([]Cell, error) {
	query := `SELECT id, lat, lng, infection, terrain, tower_id, rift_id, last_calculated, has_nearby_building, has_nearby_road, nearest_road_class FROM cells`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("get all cells: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[Cell])
}

func (r *PgRepository) GetInRadius(ctx context.Context, lat, lng, radiusM float64) ([]Cell, error) {
	query := `
		SELECT id, lat, lng, infection, terrain, tower_id, rift_id, last_calculated, has_nearby_building, has_nearby_road, nearest_road_class
		FROM cells
		WHERE ST_DWithin(geom, ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography, $3)
		ORDER BY infection DESC`

	rows, err := r.db.Query(ctx, query, lng, lat, radiusM)
	if err != nil {
		return nil, fmt.Errorf("get cells in radius: %w", err)
	}
	defer rows.Close()

	return pgx.CollectRows(rows, pgx.RowToStructByName[Cell])
}

func (r *PgRepository) GetByID(ctx context.Context, id string) (*Cell, error) {
	query := `SELECT id, lat, lng, infection, terrain, tower_id, rift_id, last_calculated, has_nearby_building, has_nearby_road, nearest_road_class FROM cells WHERE id = $1`
	rows, err := r.db.Query(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("get cell: %w", err)
	}
	defer rows.Close()

	cell, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[Cell])
	if err != nil {
		return nil, fmt.Errorf("get cell scan: %w", err)
	}
	return &cell, nil
}

func (r *PgRepository) GetNeighbors(ctx context.Context, cellID string) ([]Cell, error) {
	// Neighbors are cells within ~75m (diagonal of 50x50 grid)
	query := `
		SELECT c2.id, c2.lat, c2.lng, c2.infection, c2.terrain, c2.tower_id, c2.rift_id, c2.last_calculated
		FROM cells c1
		JOIN cells c2 ON ST_DWithin(c1.geom, c2.geom::geography, 75) AND c2.id != c1.id
		WHERE c1.id = $1`

	rows, err := r.db.Query(ctx, query, cellID)
	if err != nil {
		return nil, fmt.Errorf("get neighbors: %w", err)
	}
	defer rows.Close()

	return pgx.CollectRows(rows, pgx.RowToStructByName[Cell])
}

func (r *PgRepository) Update(ctx context.Context, id string, infection float64) error {
	query := `UPDATE cells SET infection = $1, last_calculated = NOW() WHERE id = $2`
	_, err := r.db.Exec(ctx, query, infection, id)
	if err != nil {
		return fmt.Errorf("update cell: %w", err)
	}
	return nil
}

// UnderForeignDome reports whether the cell nearest to (lat,lng) is covered by
// a dome owned by someone OTHER than playerID, plus that cell's infection (the
// Symbiont soft-drain hot input, #6 T-760). One KNN query; covered=false when
// no cell exists nearby (unseeded). Consumed via an optional interface.
func (r *PgRepository) UnderForeignDome(ctx context.Context, lat, lng float64, playerID string) (covered bool, infection float64, pierced bool, err error) {
	row := r.db.QueryRow(ctx, `
		SELECT c.infection,
		       EXISTS(SELECT 1 FROM domed_cells dc WHERE dc.cell_id = c.id AND dc.player_id <> $3),
		       (c.pierced_until IS NOT NULL AND c.pierced_until > now())
		FROM cells c
		ORDER BY c.geom <-> ST_SetSRID(ST_MakePoint($2, $1), 4326)
		LIMIT 1`, lat, lng, playerID)
	if scanErr := row.Scan(&infection, &covered, &pierced); scanErr != nil {
		if errors.Is(scanErr, pgx.ErrNoRows) {
			return false, 0, false, nil
		}
		return false, 0, false, fmt.Errorf("under foreign dome: %w", scanErr)
	}
	return covered, infection, pierced, nil
}

// RaiseInfectionNearest raises the nearest cell's infection by delta (capped at
// 100). If the new level reaches pierceThreshold the cell is pierced for ttlMin
// minutes (#6 T-762 — the dome stops suppressing it). Returns the new infection
// and whether it is now pierced.
func (r *PgRepository) RaiseInfectionNearest(ctx context.Context, lat, lng, delta, pierceThreshold float64, ttlMin int) (infection float64, pierced bool, err error) {
	err = r.db.QueryRow(ctx, `
		UPDATE cells SET
			infection = LEAST(100, infection + $3),
			pierced_until = CASE WHEN LEAST(100, infection + $3) >= $4
				THEN now() + make_interval(mins => $5) ELSE pierced_until END,
			last_calculated = now()
		WHERE id = (SELECT id FROM cells ORDER BY geom <-> ST_SetSRID(ST_MakePoint($2, $1), 4326) LIMIT 1)
		RETURNING infection, (pierced_until IS NOT NULL AND pierced_until > now())`,
		lat, lng, delta, pierceThreshold, ttlMin).Scan(&infection, &pierced)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, false, fmt.Errorf("raise infection: no cell near point")
		}
		return 0, false, fmt.Errorf("raise infection nearest: %w", err)
	}
	return infection, pierced, nil
}

// CountBuildingCellsInRadius counts cells flagged has_nearby_building within
// radiusM of (lat,lng) — a cheap OSM building-density proxy for the local
// electrical-grid capacity (energy pillar). Consumed via an optional interface
// (no Repository contract churn). Call only on rare paths (placement, tower
// detail), never per-tower on map polls.
func (r *PgRepository) CountBuildingCellsInRadius(ctx context.Context, lat, lng, radiusM float64) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `
		SELECT count(*) FROM cells
		WHERE has_nearby_building
		  AND ST_DWithin(geom, ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography, $3)`,
		lng, lat, radiusM).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count building cells in radius: %w", err)
	}
	return n, nil
}

// ClearInfectionInRadius knocks every cell within radiusM of (lat,lng) down to
// `target` infection (only cells currently above it) and returns how many rows
// changed. Used for the soft-start pocket under a new player's first Core.
// Consumed via an optional interface so it stays off the Repository contract
// (no mock churn). Mirrors GetInRadius's ST_DWithin shape/param order.
func (r *PgRepository) ClearInfectionInRadius(ctx context.Context, lat, lng, radiusM, target float64) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE cells SET infection = $4, last_calculated = NOW()
		WHERE ST_DWithin(geom, ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography, $3)
		  AND infection > $4`,
		lng, lat, radiusM, target)
	if err != nil {
		return 0, fmt.Errorf("clear infection in radius: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (r *PgRepository) SetTowerID(ctx context.Context, cellID string, towerID *string) error {
	_, err := r.db.Exec(ctx, `UPDATE cells SET tower_id = $1 WHERE id = $2`, towerID, cellID)
	return err
}

func (r *PgRepository) SetRiftID(ctx context.Context, cellID string, riftID *string) error {
	_, err := r.db.Exec(ctx, `UPDATE cells SET rift_id = $1 WHERE id = $2`, riftID, cellID)
	return err
}

// IsSuppressed reports whether a cell is under the dome or permanently
// stabilized (counter-wave) — both keep infection from growing. Mirrors the
// CASE branches in infection.PgRepository.BatchRecalculate so the per-cell
// recalculation path stays consistent with the set-based one. Not on the
// Repository interface (consumed via an optional interface) to avoid churning
// every mock for a path the production batch flow doesn't use.
func (r *PgRepository) IsSuppressed(ctx context.Context, cellID string) (bool, error) {
	var suppressed bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM domed_cells WHERE cell_id = $1)
		    OR EXISTS(SELECT 1 FROM stabilized_cells WHERE cell_id = $1)`,
		cellID).Scan(&suppressed)
	return suppressed, err
}
