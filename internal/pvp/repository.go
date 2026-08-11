package pvp

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	NearbyEnemyDomes(ctx context.Context, attackerID string, lat, lng, radiusM float64) ([]Target, error)
	DefenderOf(ctx context.Context, cellID, attackerID string) (string, bool, error)
	RaiseInfection(ctx context.Context, cellID string, to float64) error
}

type PgRepository struct{ db *pgxpool.Pool }

func NewPgRepository(db *pgxpool.Pool) *PgRepository { return &PgRepository{db: db} }

// NearbyEnemyDomes lists other players' domed cells near a point that aren't
// already breached (no rift). One row per cell.
func (r *PgRepository) NearbyEnemyDomes(ctx context.Context, attackerID string, lat, lng, radiusM float64) ([]Target, error) {
	rows, err := r.db.Query(ctx, `
		SELECT dc.cell_id, c.lat, c.lng, dc.player_id, COALESCE(p.username, 'Игрок')
		FROM domed_cells dc
		JOIN cells c ON c.id = dc.cell_id
		JOIN players p ON p.id = dc.player_id
		WHERE dc.player_id <> $1
		  AND c.rift_id IS NULL
		  AND ST_DWithin(c.geom::geography, ST_SetSRID(ST_MakePoint($3, $2), 4326)::geography, $4)
		ORDER BY ST_Distance(c.geom::geography, ST_SetSRID(ST_MakePoint($3, $2), 4326)::geography)
		LIMIT 20`,
		attackerID, lat, lng, radiusM)
	if err != nil {
		return nil, fmt.Errorf("nearby enemy domes: %w", err)
	}
	defer rows.Close()
	var out []Target
	for rows.Next() {
		var t Target
		if err := rows.Scan(&t.CellID, &t.Lat, &t.Lng, &t.DefenderID, &t.DefenderName); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// CellCoords returns a cell's coordinates (breach range validation). Off the
// Repository contract — consumed via an optional interface.
func (r *PgRepository) CellCoords(ctx context.Context, cellID string) (lat, lng float64, err error) {
	err = r.db.QueryRow(ctx, `SELECT lat, lng FROM cells WHERE id = $1`, cellID).Scan(&lat, &lng)
	if err != nil {
		return 0, 0, fmt.Errorf("cell coords: %w", err)
	}
	return lat, lng, nil
}

// DefenderOf returns an enemy owner of the dome on a cell, explicitly excluding
// the attacker. Networks overlap geographically, so one cell can sit under two
// players' domes; a plain LIMIT 1 could return the attacker themselves and get
// the breach wrongly rejected as "own dome". ok=false if the cell is undomed or
// only the attacker domes it.
func (r *PgRepository) DefenderOf(ctx context.Context, cellID, attackerID string) (string, bool, error) {
	var playerID string
	err := r.db.QueryRow(ctx,
		`SELECT player_id FROM domed_cells WHERE cell_id = $1 AND player_id <> $2 LIMIT 1`,
		cellID, attackerID).Scan(&playerID)
	if err == pgx.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("defender of cell: %w", err)
	}
	return playerID, true, nil
}

func (r *PgRepository) RaiseInfection(ctx context.Context, cellID string, to float64) error {
	_, err := r.db.Exec(ctx,
		`UPDATE cells SET infection = GREATEST(infection, $2), last_calculated = NOW() WHERE id = $1`, cellID, to)
	return err
}
