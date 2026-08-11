package pet

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/hibiken/asynq"
)

// TypeAutoClaim settles pets whose mission timer has elapsed (loot is credited
// and the pet freed) without the player tapping anything.
const TypeAutoClaim = "pet:auto_claim"

// NewAutoClaimTask builds the recurring pet auto-claim task.
func NewAutoClaimTask() *asynq.Task {
	payload, _ := json.Marshal(map[string]string{})
	return asynq.NewTask(TypeAutoClaim, payload)
}

// Worker runs scheduled pet jobs.
type Worker struct {
	svc *Service
}

// NewWorker creates a pet worker bound to the pet service.
func NewWorker(svc *Service) *Worker {
	return &Worker{svc: svc}
}

// HandleAutoClaim settles all returned pets.
func (w *Worker) HandleAutoClaim(ctx context.Context, _ *asynq.Task) error {
	claimed, err := w.svc.AutoClaimReturned(ctx)
	if err != nil {
		slog.Error("pet auto-claim failed", "error", err)
		return err
	}
	if claimed > 0 {
		slog.Info("pet auto-claim", "claimed", claimed)
	}
	return nil
}
