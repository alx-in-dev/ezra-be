package shop

import (
	"context"
	"log/slog"

	"github.com/hibiken/asynq"
)

const TypeExpireSubscriptions = "shop:expire_subscriptions"

func NewExpireSubscriptionsTask() *asynq.Task {
	return asynq.NewTask(TypeExpireSubscriptions, nil)
}

type Worker struct {
	svc *Service
}

func NewWorker(svc *Service) *Worker {
	return &Worker{svc: svc}
}

func (w *Worker) HandleExpireSubscriptions(ctx context.Context, t *asynq.Task) error {
	slog.Info("running subscription expiry check")
	return w.svc.ExpireSubscriptions(ctx)
}
