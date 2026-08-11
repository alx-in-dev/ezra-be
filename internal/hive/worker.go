package hive

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/hibiken/asynq"
)

// TypePulse is the asynq task type for the hive lifecycle tick (E1).
const TypePulse = "hive:pulse"

type Worker struct {
	service *Service
}

func NewWorker(svc *Service) *Worker {
	return &Worker{service: svc}
}

// HandlePulse runs one hive lifecycle tick.
func (w *Worker) HandlePulse(ctx context.Context, _ *asynq.Task) error {
	slog.Info("hive pulse started")
	if err := w.service.Pulse(ctx); err != nil {
		slog.Error("hive pulse failed", "error", err)
		return err
	}
	slog.Info("hive pulse completed")
	return nil
}

// NewPulseTask creates a new asynq task for the hive lifecycle.
func NewPulseTask() *asynq.Task {
	payload, _ := json.Marshal(map[string]string{})
	return asynq.NewTask(TypePulse, payload)
}
