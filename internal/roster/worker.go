package roster

import (
	"context"
	"encoding/json"

	"github.com/hibiken/asynq"
)

// TypeEntityTick advances the autonomous Symbiont entity roster (R4-2 L2):
// manifesting→assigned promotion, per-entity effect application, and
// recovering→available wake-up. Delegates to roster.Service.RunTick.
const TypeEntityTick = "symbiont:entity_tick"

// Worker runs the recurring entity tick.
type Worker struct {
	svc *Service
}

func NewWorker(svc *Service) *Worker {
	return &Worker{svc: svc}
}

// HandleEntityTick is the asynq handler for the entity tick.
func (w *Worker) HandleEntityTick(ctx context.Context, _ *asynq.Task) error {
	_, _, err := w.svc.RunTick(ctx)
	return err
}

// NewEntityTickTask builds the recurring entity-tick task.
func NewEntityTickTask() *asynq.Task {
	payload, _ := json.Marshal(map[string]string{})
	return asynq.NewTask(TypeEntityTick, payload)
}
