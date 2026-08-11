package factionwar

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/hibiken/asynq"
)

// TypeSettle is the asynq task type for settling finished faction-war seasons.
const TypeSettle = "factionwar:settle"

type Worker struct {
	service *Service
}

func NewWorker(svc *Service) *Worker {
	return &Worker{service: svc}
}

// HandleSettle pays out any past, unsettled seasons.
func (w *Worker) HandleSettle(ctx context.Context, _ *asynq.Task) error {
	slog.Info("faction season settlement started")
	if err := w.service.SettleDue(ctx); err != nil {
		slog.Error("faction season settlement failed", "error", err)
		return err
	}
	slog.Info("faction season settlement completed")
	return nil
}

// NewSettleTask creates a new asynq task for season settlement.
func NewSettleTask() *asynq.Task {
	payload, _ := json.Marshal(map[string]string{})
	return asynq.NewTask(TypeSettle, payload)
}
