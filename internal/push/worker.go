package push

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/hibiken/asynq"
)

// TypeSend is the asynq task type for sending push notifications.
const TypeSend = "push:send"

// Worker handles asynchronous push notification tasks.
type Worker struct {
	service *Service
}

// NewWorker creates a new push worker.
func NewWorker(svc *Service) *Worker {
	return &Worker{service: svc}
}

// HandleSend processes a push:send task.
func (w *Worker) HandleSend(ctx context.Context, task *asynq.Task) error {
	var msg PushMessage
	if err := json.Unmarshal(task.Payload(), &msg); err != nil {
		slog.Error("push worker: invalid payload", "error", err)
		return err
	}

	return w.service.Send(ctx, msg)
}

// NewSendTask creates a new asynq task for sending a push notification.
func NewSendTask(msg PushMessage) (*asynq.Task, error) {
	payload, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeSend, payload), nil
}
