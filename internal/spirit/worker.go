package spirit

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/hibiken/asynq"
)

// TypeTick is the asynq task type for the wild-spirit lifecycle (N2, ADR-N2):
// spawn waves, apply arrivals (drain + brownout cascade), expire. Scheduled with
// a jitter offset from the other heavy cell/network workers (40P01 gotcha).
const TypeTick = "spirit:tick"

type Worker struct{ service *Service }

func NewWorker(svc *Service) *Worker { return &Worker{service: svc} }

func (w *Worker) HandleTick(ctx context.Context, _ *asynq.Task) error {
	slog.Info("spirit tick started")
	if err := w.service.Tick(ctx); err != nil {
		slog.Error("spirit tick failed", "error", err)
		return err
	}
	slog.Info("spirit tick completed")
	return nil
}

func NewTickTask() *asynq.Task {
	payload, _ := json.Marshal(map[string]string{})
	return asynq.NewTask(TypeTick, payload)
}
