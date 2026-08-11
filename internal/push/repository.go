package push

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository defines push token persistence operations.
type Repository interface {
	Upsert(ctx context.Context, token *PushToken) error
	GetByPlayerID(ctx context.Context, playerID string) (*PushToken, error)
	Delete(ctx context.Context, playerID string) error
}

// PgRepository implements Repository using PostgreSQL.
type PgRepository struct {
	db *pgxpool.Pool
}

// NewPgRepository creates a new push token repository.
func NewPgRepository(db *pgxpool.Pool) *PgRepository {
	return &PgRepository{db: db}
}

func (r *PgRepository) Upsert(ctx context.Context, token *PushToken) error {
	query := `
		INSERT INTO push_tokens (player_id, fcm_token, platform, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (player_id) DO UPDATE SET fcm_token=$2, platform=$3, updated_at=NOW()`
	_, err := r.db.Exec(ctx, query, token.PlayerID, token.FCMToken, token.Platform)
	if err != nil {
		return fmt.Errorf("upsert push token: %w", err)
	}
	return nil
}

func (r *PgRepository) GetByPlayerID(ctx context.Context, playerID string) (*PushToken, error) {
	var t PushToken
	query := `SELECT player_id, fcm_token, platform FROM push_tokens WHERE player_id = $1`
	err := r.db.QueryRow(ctx, query, playerID).Scan(&t.PlayerID, &t.FCMToken, &t.Platform)
	if err != nil {
		return nil, fmt.Errorf("get push token: %w", err)
	}
	return &t, nil
}

func (r *PgRepository) Delete(ctx context.Context, playerID string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM push_tokens WHERE player_id = $1`, playerID)
	return err
}
