package item

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository persists player inventory items.
type Repository interface {
	Add(ctx context.Context, playerID, itemType, variant string, delta int) (int, error)
	ListByPlayer(ctx context.Context, playerID string) ([]Item, error)
}

// PgRepository implements Repository with PostgreSQL.
type PgRepository struct {
	db *pgxpool.Pool
}

func NewPgRepository(db *pgxpool.Pool) *PgRepository {
	return &PgRepository{db: db}
}

// Add upserts a stack by delta and returns the new quantity.
func (r *PgRepository) Add(ctx context.Context, playerID, itemType, variant string, delta int) (int, error) {
	var qty int
	err := r.db.QueryRow(ctx, `
		INSERT INTO player_items (player_id, item_type, variant, qty, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (player_id, item_type, variant)
		DO UPDATE SET qty = player_items.qty + EXCLUDED.qty, updated_at = now()
		RETURNING qty`,
		playerID, itemType, variant, delta,
	).Scan(&qty)
	if err != nil {
		return 0, fmt.Errorf("add item: %w", err)
	}
	return qty, nil
}

// ListByPlayer returns all non-empty item stacks for a player.
func (r *PgRepository) ListByPlayer(ctx context.Context, playerID string) ([]Item, error) {
	rows, err := r.db.Query(ctx,
		`SELECT item_type, variant, qty FROM player_items WHERE player_id = $1 AND qty > 0 AND item_type <> 'pity_blueprint' ORDER BY item_type, variant`,
		playerID)
	if err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}
	defer rows.Close()

	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[Item])
	if err != nil {
		return nil, fmt.Errorf("scan items: %w", err)
	}
	for i := range items {
		items[i].decorate()
	}
	return items, nil
}
