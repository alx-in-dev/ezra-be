package nest

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/hibiken/asynq"
)

// TypeTick is the asynq task type for the nest lifecycle tick (N3, ADR-N3-9):
// accrue trickle, apply support decay (+ siege advance / pocket refresh once the
// defense wave lands). Scheduled with a jitter offset from the other heavy
// cell-touching workers to avoid the 40P01 deadlock seen in the N0 spike.
const TypeTick = "nest:tick"

// Worker runs the nest lifecycle tick.
type Worker struct {
	service *Service
}

func NewWorker(svc *Service) *Worker {
	return &Worker{service: svc}
}

// HandleTick runs one nest lifecycle pass.
func (w *Worker) HandleTick(ctx context.Context, _ *asynq.Task) error {
	slog.Info("nest tick started")
	if err := w.service.Tick(ctx); err != nil {
		slog.Error("nest tick failed", "error", err)
		return err
	}
	slog.Info("nest tick completed")
	return nil
}

// NewTickTask creates a new asynq task for the nest lifecycle.
func NewTickTask() *asynq.Task {
	payload, _ := json.Marshal(map[string]string{})
	return asynq.NewTask(TypeTick, payload)
}
