package roster

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Entity is a bound Symbiont entity (R4-2 roster L2). It is materialized from the
// Resonance Pool and acts autonomously on its assigned target.
type Entity struct {
	ID             string     `db:"id"`
	PlayerID       string     `db:"player_id"`
	Archetype      string     `db:"archetype"`
	Attunement     int        `db:"attunement"`
	AttunementUses int        `db:"attunement_uses"`
	Status         string     `db:"status"`
	TargetKind     *string    `db:"target_kind"`
	TargetID       *string    `db:"target_id"`
	ManifestAt     *time.Time `db:"manifest_at"`
	RecoveryAt     *time.Time `db:"recovery_at"`
	CreatedAt      time.Time  `db:"created_at"`
}

// EntityRepository persists the Symbiont entity roster.
type EntityRepository interface {
	Create(ctx context.Context, e *Entity) error
	GetByID(ctx context.Context, id string) (*Entity, error)
	ListByPlayer(ctx context.Context, playerID string) ([]Entity, error)
	CountByPlayer(ctx context.Context, playerID string) (int, error)
	Update(ctx context.Context, e *Entity) error
	// ListEngaged returns every entity not in the available state (across all
	// players) — the working set for the autonomous-pressure tick.
	ListEngaged(ctx context.Context) ([]Entity, error)
}

const entityColumns = `id, player_id, archetype, attunement, attunement_uses, status, target_kind, target_id, manifest_at, recovery_at, created_at`

// PgEntityRepository is the Postgres-backed EntityRepository.
type PgEntityRepository struct {
	db *pgxpool.Pool
}

func NewPgEntityRepository(db *pgxpool.Pool) *PgEntityRepository {
	return &PgEntityRepository{db: db}
}

var _ EntityRepository = (*PgEntityRepository)(nil)

func (r *PgEntityRepository) Create(ctx context.Context, e *Entity) error {
	query := `INSERT INTO symbiont_entities (player_id, archetype, attunement, status)
		VALUES ($1, $2, $3, $4) RETURNING id, created_at`
	return r.db.QueryRow(ctx, query, e.PlayerID, e.Archetype, e.Attunement, e.Status).
		Scan(&e.ID, &e.CreatedAt)
}

func (r *PgEntityRepository) GetByID(ctx context.Context, id string) (*Entity, error) {
	rows, err := r.db.Query(ctx, `SELECT `+entityColumns+` FROM symbiont_entities WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	e, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[Entity])
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *PgEntityRepository) ListByPlayer(ctx context.Context, playerID string) ([]Entity, error) {
	rows, err := r.db.Query(ctx, `SELECT `+entityColumns+` FROM symbiont_entities WHERE player_id = $1 ORDER BY created_at`, playerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[Entity])
}

func (r *PgEntityRepository) CountByPlayer(ctx context.Context, playerID string) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `SELECT count(*) FROM symbiont_entities WHERE player_id = $1`, playerID).Scan(&n)
	return n, err
}

func (r *PgEntityRepository) Update(ctx context.Context, e *Entity) error {
	query := `UPDATE symbiont_entities
		SET archetype=$1, attunement=$2, attunement_uses=$3, status=$4, target_kind=$5, target_id=$6, manifest_at=$7, recovery_at=$8
		WHERE id=$9`
	_, err := r.db.Exec(ctx, query,
		e.Archetype, e.Attunement, e.AttunementUses, e.Status, e.TargetKind, e.TargetID, e.ManifestAt, e.RecoveryAt, e.ID)
	return err
}

func (r *PgEntityRepository) ListEngaged(ctx context.Context) ([]Entity, error) {
	rows, err := r.db.Query(ctx, `SELECT `+entityColumns+` FROM symbiont_entities WHERE status <> 'available'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[Entity])
}
