package shop

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	CreatePurchase(ctx context.Context, p *Purchase) error
	GetPurchasesByPlayer(ctx context.Context, playerID string) ([]Purchase, error)
	GetSubscription(ctx context.Context, playerID string) (*Subscription, error)
	CreateSubscription(ctx context.Context, s *Subscription) error
	UpdateSubscription(ctx context.Context, s *Subscription) error
	ExpireSubscriptions(ctx context.Context) (int64, error)
	GetCrystals(ctx context.Context, playerID string) (int, error)
	AddCrystals(ctx context.Context, playerID string, amount int) error
	SpendCrystals(ctx context.Context, playerID string, amount int) error
}

type PgRepository struct {
	db *pgxpool.Pool
}

func NewPgRepository(db *pgxpool.Pool) *PgRepository {
	return &PgRepository{db: db}
}

func (r *PgRepository) CreatePurchase(ctx context.Context, p *Purchase) error {
	query := `
		INSERT INTO purchases (player_id, item_type, item_id, crystals_spent)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`
	return r.db.QueryRow(ctx, query, p.PlayerID, p.ItemType, p.ItemID, p.CrystalsSpent).
		Scan(&p.ID, &p.CreatedAt)
}

func (r *PgRepository) GetPurchasesByPlayer(ctx context.Context, playerID string) ([]Purchase, error) {
	query := `SELECT id, player_id, item_type, item_id, crystals_spent, created_at
		FROM purchases WHERE player_id = $1 ORDER BY created_at DESC`
	rows, err := r.db.Query(ctx, query, playerID)
	if err != nil {
		return nil, fmt.Errorf("get purchases: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[Purchase])
}

func (r *PgRepository) GetSubscription(ctx context.Context, playerID string) (*Subscription, error) {
	query := `SELECT player_id, type, active, started_at, expires_at, COALESCE(receipt, '') as receipt
		FROM subscriptions WHERE player_id = $1`
	rows, err := r.db.Query(ctx, query, playerID)
	if err != nil {
		return nil, fmt.Errorf("get subscription: %w", err)
	}
	defer rows.Close()

	sub, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[Subscription])
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get subscription scan: %w", err)
	}
	return &sub, nil
}

func (r *PgRepository) CreateSubscription(ctx context.Context, s *Subscription) error {
	query := `
		INSERT INTO subscriptions (player_id, type, active, started_at, expires_at, receipt)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (player_id)
		DO UPDATE SET type = $2, active = $3, started_at = $4, expires_at = $5, receipt = $6`
	_, err := r.db.Exec(ctx, query, s.PlayerID, s.Type, s.Active, s.StartedAt, s.ExpiresAt, s.Receipt)
	return err
}

func (r *PgRepository) UpdateSubscription(ctx context.Context, s *Subscription) error {
	query := `UPDATE subscriptions SET type=$1, active=$2, expires_at=$3, receipt=$4 WHERE player_id=$5`
	_, err := r.db.Exec(ctx, query, s.Type, s.Active, s.ExpiresAt, s.Receipt, s.PlayerID)
	return err
}

func (r *PgRepository) ExpireSubscriptions(ctx context.Context) (int64, error) {
	query := `UPDATE subscriptions SET active = FALSE WHERE active = TRUE AND expires_at < $1`
	tag, err := r.db.Exec(ctx, query, time.Now())
	if err != nil {
		return 0, fmt.Errorf("expire subscriptions: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (r *PgRepository) GetCrystals(ctx context.Context, playerID string) (int, error) {
	var crystals int
	err := r.db.QueryRow(ctx, `SELECT crystals FROM players WHERE id = $1`, playerID).Scan(&crystals)
	if err != nil {
		return 0, fmt.Errorf("get crystals: %w", err)
	}
	return crystals, nil
}

func (r *PgRepository) AddCrystals(ctx context.Context, playerID string, amount int) error {
	_, err := r.db.Exec(ctx, `UPDATE players SET crystals = crystals + $1 WHERE id = $2`, amount, playerID)
	return err
}

func (r *PgRepository) SpendCrystals(ctx context.Context, playerID string, amount int) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE players SET crystals = crystals - $1 WHERE id = $2 AND crystals >= $1`,
		amount, playerID)
	if err != nil {
		return fmt.Errorf("spend crystals: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("insufficient crystals")
	}
	return nil
}
