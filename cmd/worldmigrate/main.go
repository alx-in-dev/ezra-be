// Command worldmigrate runs the one-shot N1 localization migration: it snapshots
// the world's infection into infection_snapshots (backup / rollback path) and
// then quenches infection outside the reach of live sources. It is a standalone,
// manually-run maintenance tool — NOT wired into the server or asynq — and
// refuses to do anything without the -confirm flag. Snapshot + quench run in one
// transaction, so a failure rolls the whole thing back and the world is
// untouched.
//
// Usage:
//
//	DATABASE_URL=... go run ./cmd/worldmigrate -confirm
//
// It also refuses to run unless the Faction-War season window is open (T-826) —
// between seasons, so the first post-migration scoring snapshot is post-wipe.
// Pass -skip-season-gate to override on a throwaway stand (T-829).
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	"github.com/ezra-game/server/internal/factionwar"
	"github.com/ezra-game/server/internal/platform"
	"github.com/ezra-game/server/internal/worldmigrate"
)

func main() {
	confirm := flag.Bool("confirm", false, "actually run the destructive localization (required)")
	skipGate := flag.Bool("skip-season-gate", false, "bypass the Faction-War season-window check (throwaway stands only)")
	flag.Parse()

	if !*confirm {
		slog.Error("worldmigrate refuses to run without -confirm (this cleanses infection outside sources)")
		os.Exit(2)
	}

	ctx := context.Background()
	cfg := platform.LoadConfig()

	pool, err := platform.NewPostgres(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("worldmigrate: postgres connect failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	// T-826: gate the run on the between-seasons window unless explicitly skipped.
	var gate worldmigrate.SeasonGate
	if !*skipGate {
		gate = factionwar.NewService(factionwar.NewPgRepository(pool))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		slog.Error("worldmigrate: begin tx failed", "error", err)
		os.Exit(1)
	}
	// Rollback is a no-op once we commit; on any early return it undoes the run.
	defer func() { _ = tx.Rollback(ctx) }()

	res, err := worldmigrate.Run(ctx, worldmigrate.NewPgStore(tx), gate, time.Now)
	if err != nil {
		slog.Error("worldmigrate: aborted (rolled back)", "error", err)
		os.Exit(1)
	}

	if err := tx.Commit(ctx); err != nil {
		slog.Error("worldmigrate: commit failed (rolled back)", "error", err)
		os.Exit(1)
	}

	slog.Info("worldmigrate: localization complete",
		"snapshotted", res.Snapshotted, "quenched", res.Quenched)
}
