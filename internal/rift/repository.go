package rift

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	Create(ctx context.Context, r *Rift) error
	GetByID(ctx context.Context, id string) (*Rift, error)
	GetOpen(ctx context.Context) ([]Rift, error)
	GetNearby(ctx context.Context, lat, lng, radiusM float64) ([]Rift, error)
	Update(ctx context.Context, r *Rift) error
	Close(ctx context.Context, id string) error
}

type PgRepository struct {
	db *pgxpool.Pool
}

func NewPgRepository(db *pgxpool.Pool) *PgRepository {
	return &PgRepository{db: db}
}

func (r *PgRepository) Create(ctx context.Context, rift *Rift) error {
	query := `INSERT INTO rifts (cell_id, type, intensity, radius_cells, spirits_count)
		VALUES ($1, $2, $3, $4, $5) RETURNING id, opened_at`
	return r.db.QueryRow(ctx, query, rift.CellID, rift.Type, rift.Intensity, rift.RadiusCells, rift.SpiritsCount).
		Scan(&rift.ID, &rift.OpenedAt)
}

func (r *PgRepository) GetByID(ctx context.Context, id string) (*Rift, error) {
	query := `SELECT id, cell_id, type, intensity, radius_cells, spirits_count, opened_at, closed_at FROM rifts WHERE id = $1`
	rows, err := r.db.Query(ctx, query, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rift, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[Rift])
	if err != nil {
		return nil, fmt.Errorf("rift not found: %w", err)
	}
	return &rift, nil
}

func (r *PgRepository) GetOpen(ctx context.Context) ([]Rift, error) {
	query := `SELECT id, cell_id, type, intensity, radius_cells, spirits_count, opened_at, closed_at FROM rifts WHERE closed_at IS NULL`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[Rift])
}

func (r *PgRepository) GetNearby(ctx context.Context, lat, lng, radiusM float64) ([]Rift, error) {
	query := `SELECT r.id, r.cell_id, r.type, r.intensity, r.radius_cells, r.spirits_count, r.opened_at, r.closed_at
		FROM rifts r JOIN cells c ON r.cell_id = c.id
		WHERE r.closed_at IS NULL AND ST_DWithin(c.geom, ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography, $3)`
	rows, err := r.db.Query(ctx, query, lng, lat, radiusM)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[Rift])
}

func (r *PgRepository) Update(ctx context.Context, rift *Rift) error {
	query := `UPDATE rifts SET radius_cells=$1, intensity=$2, spirits_count=$3 WHERE id=$4`
	_, err := r.db.Exec(ctx, query, rift.RadiusCells, rift.Intensity, rift.SpiritsCount, rift.ID)
	return err
}

func (r *PgRepository) Close(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `UPDATE rifts SET closed_at=NOW() WHERE id=$1`, id)
	return err
}

// SpawnCandidate is a cell eligible for an organic rift spawn.
type SpawnCandidate struct {
	CellID    string  `db:"id"`
	Infection float64 `db:"infection"`
	Lat       float64 `db:"lat"`
	Lng       float64 `db:"lng"`
}

// OrganicCandidates samples random cells hot enough to open a rift
// (infection ≥ minInfection), excluding cells that already host a rift or
// tower, domed cells, and anything within spacingM of an existing open rift
// (density cap). Random pre-sample keeps the spatial NOT EXISTS cheap on
// large worlds. Kept off the Repository contract — consumed via an optional
// interface (test fakes don't need it).
func (r *PgRepository) OrganicCandidates(ctx context.Context, minInfection, spacingM float64, limit int) ([]SpawnCandidate, error) {
	query := `
		SELECT id, infection, lat, lng FROM (
			SELECT c.id, c.infection, c.lat, c.lng, c.geom
			FROM cells c
			WHERE c.infection >= $1 AND c.rift_id IS NULL AND c.tower_id IS NULL
			  AND NOT EXISTS (SELECT 1 FROM domed_cells dc WHERE dc.cell_id = c.id)
			ORDER BY random() LIMIT 200
		) s
		WHERE NOT EXISTS (
			SELECT 1 FROM rifts r JOIN cells rc ON r.cell_id = rc.id
			WHERE r.closed_at IS NULL AND ST_DWithin(rc.geom, s.geom::geography, $2)
		)
		LIMIT $3`
	rows, err := r.db.Query(ctx, query, minInfection, spacingM, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[SpawnCandidate])
}
