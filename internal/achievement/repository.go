package achievement

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository reads derived achievement metrics and persists unlock events.
type Repository interface {
	Metrics(ctx context.Context, playerID string) (Metrics, error)
	UnlockedIDs(ctx context.Context, playerID string) (map[string]bool, error)
	MarkUnlocked(ctx context.Context, playerID, achievementID string) error
}

// PgRepository implements Repository over PostgreSQL.
type PgRepository struct {
	db *pgxpool.Pool
}

func NewPgRepository(db *pgxpool.Pool) *PgRepository {
	return &PgRepository{db: db}
}

// Metrics derives all measured stats in one round-trip via scalar subqueries.
// Everything comes from existing tables, so achievements credit past actions.
func (r *PgRepository) Metrics(ctx context.Context, playerID string) (Metrics, error) {
	const q = `
		SELECT
			(SELECT count(*) FROM towers WHERE owner_id = $1),
			COALESCE((SELECT level FROM cores WHERE player_id = $1), 0),
			(SELECT count(*) FROM battles WHERE attacker_id = $1 AND status = 'resolved_victory'),
			(SELECT count(DISTINCT variant) FROM player_items
			   WHERE player_id = $1 AND item_type = 'resonance_signature' AND qty > 0),
			COALESCE((SELECT level FROM players WHERE id = $1), 0)`
	var m Metrics
	err := r.db.QueryRow(ctx, q, playerID).Scan(
		&m.Beacons, &m.CoreLevel, &m.BattlesWon, &m.Spectra, &m.PlayerLevel)
	if err != nil {
		return Metrics{}, fmt.Errorf("achievement metrics: %w", err)
	}
	return m, nil
}

func (r *PgRepository) UnlockedIDs(ctx context.Context, playerID string) (map[string]bool, error) {
	rows, err := r.db.Query(ctx,
		`SELECT achievement_id FROM player_achievements WHERE player_id = $1`, playerID)
	if err != nil {
		return nil, fmt.Errorf("unlocked achievements: %w", err)
	}
	defer rows.Close()
	out := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

func (r *PgRepository) MarkUnlocked(ctx context.Context, playerID, achievementID string) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO player_achievements (player_id, achievement_id) VALUES ($1, $2)
		 ON CONFLICT (player_id, achievement_id) DO NOTHING`,
		playerID, achievementID)
	if err != nil {
		return fmt.Errorf("mark achievement unlocked: %w", err)
	}
	return nil
}
