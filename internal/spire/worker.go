package spire

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/hibiken/asynq"
)

// TypeLifecycle is the asynq task type for the Spire upkeep/degrade tick.
const TypeLifecycle = "spire:lifecycle"

type Worker struct {
	service *Service
}

func NewWorker(svc *Service) *Worker {
	return &Worker{service: svc}
}

// HandleLifecycle degrades/ruins Spires that lost upkeep.
func (w *Worker) HandleLifecycle(ctx context.Context, _ *asynq.Task) error {
	slog.Info("spire lifecycle tick started")
	if err := w.service.LifecycleTick(ctx); err != nil {
		slog.Error("spire lifecycle tick failed", "error", err)
		return err
	}
	slog.Info("spire lifecycle tick completed")
	return nil
}

// NewLifecycleTask creates a new asynq task for the Spire lifecycle.
func NewLifecycleTask() *asynq.Task {
	payload, _ := json.Marshal(map[string]string{})
	return asynq.NewTask(TypeLifecycle, payload)
}
