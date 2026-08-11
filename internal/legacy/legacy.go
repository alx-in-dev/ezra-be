// Package legacy runs the legacy human-network degradation lifecycle (R4-4a,
// legacy_human_network.md). When a player sides with the Symbionts their human
// beacons become legacy infrastructure (stamped legacy_since) that degrades
// stable → fading → offline over time, unless an active human nearby sustains it
// (resets the timer). The network service already excludes offline legacy
// beacons from power/dome (see network.activeNodes); this worker maintains the
// legacy_since stamps and recomputes affected networks so domes drop on time.
package legacy

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/ezra-game/server/internal/canon"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TypeDegrade is the recurring legacy-degradation task.
const TypeDegrade = "legacy:degrade"

// Repository owns the legacy_since bookkeeping (raw SQL over towers / cores /
// player_factions).
type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

// StampNewSymbiontLegacy stamps legacy_since on any beacon whose owner is now a
// Symbiont but isn't yet marked legacy (fresh switch / pre-existing ex-humans).
func (r *Repository) StampNewSymbiontLegacy(ctx context.Context) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE towers SET legacy_since = NOW()
		WHERE legacy_since IS NULL
		  AND owner_id IN (SELECT player_id FROM player_factions WHERE faction = 'symbiont')`)
	return tag.RowsAffected(), err
}

// ClearNonSymbiontLegacy un-marks beacons whose owner is no longer a Symbiont
// (switched back to human) — they become active again.
func (r *Repository) ClearNonSymbiontLegacy(ctx context.Context) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE towers SET legacy_since = NULL
		WHERE legacy_since IS NOT NULL
		  AND owner_id NOT IN (SELECT player_id FROM player_factions WHERE faction = 'symbiont')`)
	return tag.RowsAffected(), err
}

// SustainLegacyNearHumans resets legacy_since (→ back to stable) on any legacy
// beacon with a non-Symbiont player's node within radiusM (the "active human
// nearby sustains it" rule).
func (r *Repository) SustainLegacyNearHumans(ctx context.Context, radiusM float64) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE towers t SET legacy_since = NOW()
		WHERE t.legacy_since IS NOT NULL
		  AND EXISTS (
		      SELECT 1 FROM (
		          SELECT geom, owner_id AS pid FROM towers WHERE legacy_since IS NULL
		          UNION ALL
		          SELECT geom, player_id AS pid FROM cores
		      ) n
		      WHERE n.pid NOT IN (SELECT player_id FROM player_factions WHERE faction = 'symbiont')
		        AND ST_DWithin(t.geom::geography, n.geom::geography, $1)
		  )`, radiusM)
	return tag.RowsAffected(), err
}

// LegacyOwners returns the distinct owners of any legacy beacon (for recompute).
func (r *Repository) LegacyOwners(ctx context.Context) ([]string, error) {
	rows, err := r.db.Query(ctx, `SELECT DISTINCT owner_id::text FROM towers WHERE legacy_since IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// Recomputer re-derives a player's network (so domes reflect current legacy
// state). Implemented by network.Service.
type Recomputer interface {
	Recompute(ctx context.Context, playerID string) error
}

// Worker drives the periodic legacy degradation.
type Worker struct {
	repo    *Repository
	network Recomputer
}

func NewWorker(repo *Repository, network Recomputer) *Worker {
	return &Worker{repo: repo, network: network}
}

// HandleDegrade is the asynq handler: maintain legacy stamps, apply support, and
// recompute legacy owners so offline beacons stop doming on schedule.
func (w *Worker) HandleDegrade(ctx context.Context, _ *asynq.Task) error {
	stamped, err := w.repo.StampNewSymbiontLegacy(ctx)
	if err != nil {
		return err
	}
	cleared, err := w.repo.ClearNonSymbiontLegacy(ctx)
	if err != nil {
		return err
	}
	sustained, err := w.repo.SustainLegacyNearHumans(ctx, canon.LegacySupportRadiusM)
	if err != nil {
		return err
	}
	owners, err := w.repo.LegacyOwners(ctx)
	if err != nil {
		return err
	}
	recomputed := 0
	if w.network != nil {
		for _, pid := range owners {
			if rErr := w.network.Recompute(ctx, pid); rErr != nil {
				slog.Warn("legacy: recompute failed", "player_id", pid, "error", rErr)
				continue
			}
			recomputed++
		}
	}
	slog.Info("legacy degradation tick",
		"stamped", stamped, "cleared", cleared, "sustained", sustained,
		"legacy_owners", len(owners), "recomputed", recomputed)
	return nil
}

// NewDegradeTask builds the recurring task.
func NewDegradeTask() *asynq.Task {
	payload, _ := json.Marshal(map[string]string{})
	return asynq.NewTask(TypeDegrade, payload)
}
