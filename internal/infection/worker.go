package infection

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/hibiken/asynq"
)

// TypeRecalculate is the asynq task type for periodic infection recalculation.
const TypeRecalculate = "infection:recalculate"

// TypeTideAdvance is the asynq task type for the D2 antagonist tide — directed
// infection advance toward player beacons.
const TypeTideAdvance = "infection:tide"

// Worker handles asynchronous infection tasks.
type Worker struct {
	service *Service
}

// NewWorker creates a new infection Worker.
func NewWorker(svc *Service) *Worker {
	return &Worker{service: svc}
}

// HandleRecalculate processes the periodic recalculation task.
func (w *Worker) HandleRecalculate(ctx context.Context, _ *asynq.Task) error {
	slog.Info("infection recalculation started")
	if err := w.service.RecalculateAll(ctx); err != nil {
		slog.Error("infection recalculation failed", "error", err)
		return err
	}
	slog.Info("infection recalculation completed")
	return nil
}

// NewRecalculateTask creates a new asynq task for infection recalculation.
func NewRecalculateTask() *asynq.Task {
	payload, _ := json.Marshal(map[string]string{})
	return asynq.NewTask(TypeRecalculate, payload)
}

// HandleTideAdvance processes one tick of the antagonist tide (D2).
func (w *Worker) HandleTideAdvance(ctx context.Context, _ *asynq.Task) error {
	slog.Info("antagonist tide started")
	if err := w.service.AdvanceTide(ctx); err != nil {
		slog.Error("antagonist tide failed", "error", err)
		return err
	}
	slog.Info("antagonist tide completed")
	return nil
}

// NewTideAdvanceTask creates a new asynq task for the antagonist tide.
func NewTideAdvanceTask() *asynq.Task {
	payload, _ := json.Marshal(map[string]string{})
	return asynq.NewTask(TypeTideAdvance, payload)
}
