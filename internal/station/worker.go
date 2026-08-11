package station

import (
	"context"

	"github.com/hibiken/asynq"
)

// TypeLifecycle is the periodic power-plant decay task (T-778).
const TypeLifecycle = "station:lifecycle"

// NewLifecycleTask builds the scheduled lifecycle task.
func NewLifecycleTask() *asynq.Task { return asynq.NewTask(TypeLifecycle, nil) }

// Worker advances power-plant lifecycle on a schedule.
type Worker struct {
	svc *Service
}

func NewWorker(svc *Service) *Worker { return &Worker{svc: svc} }

// HandleLifecycle degrades/ruins plants past their upkeep window.
func (w *Worker) HandleLifecycle(ctx context.Context, _ *asynq.Task) error {
	return w.svc.Tick(ctx)
}
