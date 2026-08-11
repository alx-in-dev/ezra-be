package survivor

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// BuildingCell is the centre of a domed building cell — where survivors are
// found (lore: people shelter in buildings under an active dome).
type BuildingCell struct {
	Lat float64
	Lng float64
}

// DomeReader returns the player's domed building cells near a point. Survivors
// are discovered only inside an active dome (docs/02_core_gameplay.md §2.6), so
// this gates where they may appear.
type DomeReader interface {
	DomedBuildingCells(ctx context.Context, playerID string, lat, lng, radiusM float64) ([]BuildingCell, error)
}

// PgDomeReader reads domed building cells from PostGIS.
type PgDomeReader struct {
	db *pgxpool.Pool
}

// NewPgDomeReader constructs a DomeReader over the given pool.
func NewPgDomeReader(db *pgxpool.Pool) *PgDomeReader {
	return &PgDomeReader{db: db}
}

// DomedBuildingCells returns the centres of the player's domed cells with
// building terrain within radiusM of (lat,lng), capped for sanity.
func (r *PgDomeReader) DomedBuildingCells(ctx context.Context, playerID string, lat, lng, radiusM float64) ([]BuildingCell, error) {
	const q = `
		SELECT c.lat, c.lng
		FROM cells c
		JOIN domed_cells dc ON dc.cell_id = c.id
		WHERE dc.player_id = $1::uuid
		  AND c.terrain = 'building'
		  AND ST_DWithin(c.geom::geography, ST_SetSRID(ST_MakePoint($3, $2), 4326)::geography, $4)
		LIMIT 32`
	rows, err := r.db.Query(ctx, q, playerID, lat, lng, radiusM)
	if err != nil {
		return nil, fmt.Errorf("domed building cells: %w", err)
	}
	defer rows.Close()
	var cells []BuildingCell
	for rows.Next() {
		var c BuildingCell
		if err := rows.Scan(&c.Lat, &c.Lng); err != nil {
			return nil, err
		}
		cells = append(cells, c)
	}
	return cells, rows.Err()
}
