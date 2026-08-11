package bestiary

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository persists encounter discoveries and derives owned-type entries.
type Repository interface {
	// DiscoveredKeys returns the player's persisted discovery keys plus the
	// derived ones (tower types owned, spectra collected) — the full set of
	// keys the player counts as discovered.
	DiscoveredKeys(ctx context.Context, playerID string) (map[string]bool, error)
	// MarkDiscovered persists encounter discoveries (spirit:/rift:), ignoring
	// duplicates. No-op for empty input.
	MarkDiscovered(ctx context.Context, playerID string, keys []string) error
	// HasCompletionReward / SetCompletionReward gate the one-time full-set bonus.
	HasCompletionReward(ctx context.Context, playerID string) (bool, error)
	SetCompletionReward(ctx context.Context, playerID string) error
}

type PgRepository struct {
	db *pgxpool.Pool
}

func NewPgRepository(db *pgxpool.Pool) *PgRepository {
	return &PgRepository{db: db}
}

func (r *PgRepository) DiscoveredKeys(ctx context.Context, playerID string) (map[string]bool, error) {
	out := make(map[string]bool)

	// Persisted encounter discoveries (spirit:/rift:/sentinel).
	rows, err := r.db.Query(ctx,
		`SELECT discovery_key FROM player_discoveries WHERE player_id = $1`, playerID)
	if err != nil {
		return nil, fmt.Errorf("discovered keys: %w", err)
	}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			rows.Close()
			return nil, err
		}
		out[k] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Derived: tower types owned.
	tRows, err := r.db.Query(ctx, `SELECT DISTINCT type FROM towers WHERE owner_id = $1`, playerID)
	if err != nil {
		return nil, fmt.Errorf("derived tower types: %w", err)
	}
	for tRows.Next() {
		var t string
		if err := tRows.Scan(&t); err != nil {
			tRows.Close()
			return nil, err
		}
		out["tower:"+t] = true
	}
	tRows.Close()
	if err := tRows.Err(); err != nil {
		return nil, err
	}

	// Derived: resonance spectra collected.
	sRows, err := r.db.Query(ctx,
		`SELECT DISTINCT variant FROM player_items
		   WHERE player_id = $1 AND item_type = 'resonance_signature' AND qty > 0 AND variant <> ''`, playerID)
	if err != nil {
		return nil, fmt.Errorf("derived spectra: %w", err)
	}
	for sRows.Next() {
		var v string
		if err := sRows.Scan(&v); err != nil {
			sRows.Close()
			return nil, err
		}
		out["spectrum:"+v] = true
	}
	sRows.Close()
	return out, sRows.Err()
}

func (r *PgRepository) MarkDiscovered(ctx context.Context, playerID string, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	for _, k := range keys {
		if _, err := r.db.Exec(ctx,
			`INSERT INTO player_discoveries (player_id, discovery_key) VALUES ($1, $2)
			 ON CONFLICT (player_id, discovery_key) DO NOTHING`, playerID, k); err != nil {
			return fmt.Errorf("mark discovered %q: %w", k, err)
		}
	}
	return nil
}

func (r *PgRepository) HasCompletionReward(ctx context.Context, playerID string) (bool, error) {
	var found bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM player_discoveries WHERE player_id = $1 AND discovery_key = $2)`,
		playerID, completionKey).Scan(&found)
	return found, err
}

func (r *PgRepository) SetCompletionReward(ctx context.Context, playerID string) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO player_discoveries (player_id, discovery_key) VALUES ($1, $2)
		 ON CONFLICT (player_id, discovery_key) DO NOTHING`, playerID, completionKey)
	return err
}
