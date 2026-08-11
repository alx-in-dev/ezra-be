package battle

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository defines battle persistence operations.
type Repository interface {
	Create(ctx context.Context, b *Battle) error
	GetByID(ctx context.Context, id string) (*Battle, error)
	Update(ctx context.Context, b *Battle) error
}

// PgRepository implements Repository using PostgreSQL.
type PgRepository struct {
	db *pgxpool.Pool
}

// NewPgRepository creates a new battle repository.
func NewPgRepository(db *pgxpool.Pool) *PgRepository {
	return &PgRepository{db: db}
}

func (r *PgRepository) Create(ctx context.Context, b *Battle) error {
	roundsJSON, err := json.Marshal(b.Rounds)
	if err != nil {
		return fmt.Errorf("marshal rounds: %w", err)
	}
	query := `INSERT INTO battles (attacker_id, squad_id, target_type, target_id, rounds, status)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id, created_at, updated_at`
	return r.db.QueryRow(ctx, query,
		b.AttackerID, b.SquadID, b.TargetType, b.TargetID, roundsJSON, b.Status,
	).Scan(&b.ID, &b.CreatedAt, &b.UpdatedAt)
}

func (r *PgRepository) GetByID(ctx context.Context, id string) (*Battle, error) {
	query := `SELECT id, attacker_id, squad_id, target_type, target_id, rounds, status, created_at, updated_at
		FROM battles WHERE id = $1`
	row := r.db.QueryRow(ctx, query, id)

	var b Battle
	var roundsJSON []byte
	err := row.Scan(&b.ID, &b.AttackerID, &b.SquadID, &b.TargetType, &b.TargetID,
		&roundsJSON, &b.Status, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("battle not found")
		}
		return nil, err
	}

	if err := json.Unmarshal(roundsJSON, &b.Rounds); err != nil {
		return nil, fmt.Errorf("unmarshal rounds: %w", err)
	}
	return &b, nil
}

func (r *PgRepository) Update(ctx context.Context, b *Battle) error {
	roundsJSON, err := json.Marshal(b.Rounds)
	if err != nil {
		return fmt.Errorf("marshal rounds: %w", err)
	}
	query := `UPDATE battles SET rounds = $1, status = $2, updated_at = NOW() WHERE id = $3`
	_, err = r.db.Exec(ctx, query, roundsJSON, b.Status, b.ID)
	return err
}
