package faction

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	// Get returns the player's faction row, defaulting to Human/uncontacted if
	// none exists yet.
	Get(ctx context.Context, playerID string) (faction string, contacted, chosen bool, err error)
	SetContacted(ctx context.Context, playerID string) error
	Choose(ctx context.Context, playerID, side string) error
	// SetFaction sets the side + contacted with an explicit `chosen` flag. Used
	// for the temporary-symbiont state during onboarding (chosen=false).
	SetFaction(ctx context.Context, playerID, side string, chosen bool) error
}

type PgRepository struct{ db *pgxpool.Pool }

func NewPgRepository(db *pgxpool.Pool) *PgRepository { return &PgRepository{db: db} }

func (r *PgRepository) Get(ctx context.Context, playerID string) (string, bool, bool, error) {
	var faction string
	var contacted, chosen bool
	err := r.db.QueryRow(ctx,
		`SELECT faction, contacted, chosen FROM player_factions WHERE player_id = $1`, playerID).
		Scan(&faction, &contacted, &chosen)
	if err == pgx.ErrNoRows {
		return Human, false, false, nil
	}
	if err != nil {
		return "", false, false, fmt.Errorf("get faction: %w", err)
	}
	return faction, contacted, chosen, nil
}

func (r *PgRepository) SetContacted(ctx context.Context, playerID string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO player_factions (player_id, contacted, updated_at)
		VALUES ($1, true, now())
		ON CONFLICT (player_id) DO UPDATE SET contacted = true, updated_at = now()`,
		playerID)
	return err
}

// LastChangeAt returns when the player's faction row last changed (Choose
// stamps updated_at). Off the Repository contract — the switch-cooldown check
// consumes it via an optional interface; ok=false when no row exists.
func (r *PgRepository) LastChangeAt(ctx context.Context, playerID string) (time.Time, bool, error) {
	var at time.Time
	err := r.db.QueryRow(ctx,
		`SELECT updated_at FROM player_factions WHERE player_id = $1`, playerID).Scan(&at)
	if err == pgx.ErrNoRows {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("faction last change: %w", err)
	}
	return at, true, nil
}

func (r *PgRepository) Choose(ctx context.Context, playerID, side string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO player_factions (player_id, faction, contacted, chosen, updated_at)
		VALUES ($1, $2, true, true, now())
		ON CONFLICT (player_id) DO UPDATE SET faction = $2, chosen = true, updated_at = now()`,
		playerID, side)
	return err
}

func (r *PgRepository) SetFaction(ctx context.Context, playerID, side string, chosen bool) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO player_factions (player_id, faction, contacted, chosen, updated_at)
		VALUES ($1, $2, true, $3, now())
		ON CONFLICT (player_id) DO UPDATE SET faction = $2, contacted = true, chosen = $3, updated_at = now()`,
		playerID, side, chosen)
	return err
}
