package tower

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository defines tower persistence operations.
type Repository interface {
	Create(ctx context.Context, t *Tower) error
	GetAll(ctx context.Context) ([]Tower, error)
	GetByID(ctx context.Context, id string) (*Tower, error)
	GetByIDs(ctx context.Context, ids []string) ([]Tower, error)
	GetByOwner(ctx context.Context, playerID string) ([]Tower, error)
	GetInRadius(ctx context.Context, lat, lng, radiusM float64) ([]Tower, error)
	Update(ctx context.Context, t *Tower) error
	UpdateOwnership(ctx context.Context, id, newOwnerID string, hp, hpMax int) error
	Delete(ctx context.Context, id string) error
	CountInRadius(ctx context.Context, lat, lng, radiusM float64, ownerID string) (int, error)
	CountAllInRadius(ctx context.Context, lat, lng, radiusM float64) (int, error)
}

// PgRepository implements Repository using PostgreSQL.
type PgRepository struct {
	db *pgxpool.Pool
}

func NewPgRepository(db *pgxpool.Pool) *PgRepository {
	return &PgRepository{db: db}
}

func (r *PgRepository) Create(ctx context.Context, t *Tower) error {
	query := `INSERT INTO towers (cell_id, lat, lng, geom, owner_id, level, type, hp, hp_max, radius_m, effect_per_hour)
		VALUES ($1, $2, $3, ST_SetSRID(ST_MakePoint($3, $2), 4326), $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, installed_at, upgraded_at`
	return r.db.QueryRow(ctx, query, t.CellID, t.Lat, t.Lng, t.OwnerID, t.Level, t.Type, t.HP, t.HPMax, t.RadiusM, t.EffectPerHour).
		Scan(&t.ID, &t.InstalledAt, &t.UpgradedAt)
}

func (r *PgRepository) GetAll(ctx context.Context) ([]Tower, error) {
	query := `SELECT id, cell_id, lat, lng, owner_id, level, type, hp, hp_max, radius_m, effect_per_hour, installed_at, upgraded_at
		FROM towers`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[Tower])
}

// GetIncomeEligible is GetAll minus legacy beacons: once a tower is stamped
// legacy_since (owner went Symbiont), it pays the ex-owner nothing (canon
// legacy_human_network.md "no income to the ex-owner"). Also carries the
// brownout flag (halved income). Off the Repository contract — the income
// worker consumes it via an optional interface.
func (r *PgRepository) GetIncomeEligible(ctx context.Context) ([]Tower, error) {
	query := `SELECT id, owner_id, level, brownout FROM towers WHERE legacy_since IS NULL`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Tower
	for rows.Next() {
		var t Tower
		if err := rows.Scan(&t.ID, &t.OwnerID, &t.Level, &t.Brownout); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *PgRepository) GetByID(ctx context.Context, id string) (*Tower, error) {
	query := `SELECT id, cell_id, lat, lng, owner_id, level, type, hp, hp_max, radius_m, effect_per_hour, installed_at, upgraded_at
		FROM towers WHERE id = $1`
	rows, err := r.db.Query(ctx, query, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	t, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[Tower])
	if err != nil {
		return nil, fmt.Errorf("tower not found: %w", err)
	}
	return &t, nil
}

func (r *PgRepository) GetByIDs(ctx context.Context, ids []string) ([]Tower, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	query := `SELECT id, cell_id, lat, lng, owner_id, level, type, hp, hp_max, radius_m, effect_per_hour, installed_at, upgraded_at
		FROM towers WHERE id = ANY($1)`
	rows, err := r.db.Query(ctx, query, ids)
	if err != nil {
		return nil, fmt.Errorf("get towers by ids: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[Tower])
}

func (r *PgRepository) GetByOwner(ctx context.Context, playerID string) ([]Tower, error) {
	query := `SELECT id, cell_id, lat, lng, owner_id, level, type, hp, hp_max, radius_m, effect_per_hour, installed_at, upgraded_at
		FROM towers WHERE owner_id = $1`
	rows, err := r.db.Query(ctx, query, playerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[Tower])
}

func (r *PgRepository) GetInRadius(ctx context.Context, lat, lng, radiusM float64) ([]Tower, error) {
	query := `SELECT t.id, t.cell_id, t.lat, t.lng, t.owner_id, t.level, t.type, t.hp, t.hp_max, t.radius_m, t.effect_per_hour, t.installed_at, t.upgraded_at
		FROM towers t
		WHERE ST_DWithin(t.geom::geography, ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography, $3)`
	rows, err := r.db.Query(ctx, query, lng, lat, radiusM)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[Tower])
}

func (r *PgRepository) Update(ctx context.Context, t *Tower) error {
	query := `UPDATE towers SET level=$1, hp=$2, hp_max=$3, radius_m=$4, effect_per_hour=$5, upgraded_at=NOW() WHERE id=$6`
	_, err := r.db.Exec(ctx, query, t.Level, t.HP, t.HPMax, t.RadiusM, t.EffectPerHour, t.ID)
	return err
}

func (r *PgRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM towers WHERE id = $1`, id)
	return err
}

func (r *PgRepository) CountInRadius(ctx context.Context, lat, lng, radiusM float64, ownerID string) (int, error) {
	query := `SELECT COUNT(*) FROM towers t
		WHERE ST_DWithin(t.geom::geography, ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography, $3) AND t.owner_id = $4`
	var count int
	err := r.db.QueryRow(ctx, query, lng, lat, radiusM, ownerID).Scan(&count)
	return count, err
}

func (r *PgRepository) UpdateOwnership(ctx context.Context, id, newOwnerID string, hp, hpMax int) error {
	query := `UPDATE towers SET owner_id=$1, hp=$2, hp_max=$3, upgraded_at=NOW() WHERE id=$4`
	_, err := r.db.Exec(ctx, query, newOwnerID, hp, hpMax, id)
	return err
}

func (r *PgRepository) CountAllInRadius(ctx context.Context, lat, lng, radiusM float64) (int, error) {
	query := `SELECT COUNT(*) FROM towers t
		WHERE ST_DWithin(t.geom::geography, ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography, $3)`
	var count int
	err := r.db.QueryRow(ctx, query, lng, lat, radiusM).Scan(&count)
	return count, err
}

// OwnerOf returns the owner player id of a beacon (N2 spirit wave targeting).
func (r *PgRepository) OwnerOf(ctx context.Context, towerID string) (string, error) {
	var owner string
	err := r.db.QueryRow(ctx, `SELECT owner_id::text FROM towers WHERE id = $1`, towerID).Scan(&owner)
	if err != nil {
		return "", fmt.Errorf("tower owner: %w", err)
	}
	return owner, nil
}

// SetSpiritPressure marks a beacon under N2 spirit pressure until `until` — a
// time-boxed brownout cause that shrinks the dome and halves suppression (T-864).
func (r *PgRepository) SetSpiritPressure(ctx context.Context, towerID string, until time.Time) error {
	_, err := r.db.Exec(ctx,
		`UPDATE towers SET spirit_pressure_until = $2 WHERE id = $1`, towerID, until)
	if err != nil {
		return fmt.Errorf("set spirit pressure: %w", err)
	}
	return nil
}
