package symbiont

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
)

const TypeDrainTick = "symbiont:drain_tick"

// drainActiveWindow bounds which symbionts the tick considers: only players
// active recently are "standing" anywhere — draining a week-offline player at
// their last known position would be pure punishment.
const drainActiveWindow = 15 * time.Minute

// ActiveSymbiontLister returns ids of Symbiont-faction players active within
// the window (player repo).
type ActiveSymbiontLister interface {
	ActiveSymbiontIDs(ctx context.Context, activeWithin time.Duration) ([]string, error)
}

// Worker applies the under-dome soft-drain on the server clock. Previously the
// drain fired only when the client polled /symbiont/status — a client that
// simply didn't poll never bled. The DB throttle (symbiont_last_drain_at)
// makes worker + poll paths safely idempotent per interval.
type Worker struct {
	service *Service
	players ActiveSymbiontLister
}

func NewWorker(svc *Service, players ActiveSymbiontLister) *Worker {
	return &Worker{service: svc, players: players}
}

func (w *Worker) HandleDrainTick(ctx context.Context, _ *asynq.Task) error {
	ids, err := w.players.ActiveSymbiontIDs(ctx, drainActiveWindow)
	if err != nil {
		return err
	}
	drained := 0
	for _, id := range ids {
		// Evaluate resolves the server-side position itself and applies the
		// throttled drain when the player stands under a hostile dome.
		st, err := w.service.Evaluate(ctx, id, 0, 0)
		if err != nil {
			slog.Warn("symbiont drain tick failed", "player_id", id, "error", err)
			continue
		}
		if st.EnergyDrained > 0 {
			drained++
		}
	}
	if drained > 0 {
		slog.Info("symbiont drain tick", "symbionts", len(ids), "drained", drained)
	}
	return nil
}

func NewDrainTickTask() *asynq.Task {
	payload, _ := json.Marshal(map[string]string{})
	return asynq.NewTask(TypeDrainTick, payload)
}
