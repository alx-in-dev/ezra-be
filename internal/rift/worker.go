package rift

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/hibiken/asynq"
)

const (
	TypeExpand       = "rift:expand"
	TypeSpawnOrganic = "rift:spawn_organic"
)

// Expansion caps: canon flags radius >15 as a regional crisis
// (docs/03_world_system.md «Кризис зоны»); without a cap an abandoned rift
// grows forever. Intensity shares the entity-amplification ceiling.
const (
	maxRadiusCells = 15
	maxIntensity   = 100 // = canon.RiftAmplifyCap
)

type Worker struct {
	service *Service
}

func NewWorker(svc *Service) *Worker {
	return &Worker{service: svc}
}

// cellSizeM is the grid cell edge in meters (50x50m cells, see seeder/canon).
const cellSizeM = 50.0

// expandInfectionPct is the infection added to each cell inside a rift's
// radius per expansion tick.
const expandInfectionPct = 5.0

// HandleExpand expands all open rifts: radius +1 cell, +5% infection across
// every cell within the rift's (now larger) radius — so spread scales with
// RadiusCells instead of only touching the ~75m direct neighbors.
func (w *Worker) HandleExpand(ctx context.Context, _ *asynq.Task) error {
	slog.Info("rift expansion started")

	openRifts, err := w.service.rifts.GetOpen(ctx)
	if err != nil {
		return err
	}

	cellsTouched := 0
	for _, r := range openRifts {
		if r.RadiusCells < maxRadiusCells {
			r.RadiusCells++
		}
		if r.Intensity < maxIntensity {
			r.Intensity++
		}
		if err := w.service.rifts.Update(ctx, &r); err != nil {
			slog.Error("rift expand failed", "rift_id", r.ID, "error", err)
			continue
		}

		c, err := w.service.cells.GetByID(ctx, r.CellID)
		if err != nil {
			slog.Warn("rift expand: cell lookup failed", "rift_id", r.ID, "cell_id", r.CellID, "error", err)
			continue
		}

		// Radius in meters: RadiusCells cells out, using the cell diagonal
		// (×~1.41) so corner cells inside the square footprint are covered.
		radiusM := float64(r.RadiusCells) * cellSizeM * 1.4142
		affected, err := w.service.cells.GetInRadius(ctx, c.Lat, c.Lng, radiusM)
		if err != nil {
			slog.Warn("rift expand: radius query failed", "rift_id", r.ID, "error", err)
			continue
		}

		for _, n := range affected {
			newInfection := n.Infection + expandInfectionPct
			if newInfection > 100 {
				newInfection = 100
			}
			if err := w.service.cells.Update(ctx, n.ID, newInfection); err != nil {
				slog.Warn("rift expand: cell update failed", "cell_id", n.ID, "error", err)
				continue
			}
			cellsTouched++
		}
	}

	slog.Info("rift expansion completed", "rifts_processed", len(openRifts), "cells_touched", cellsTouched)
	return nil
}

func NewExpandTask() *asynq.Task {
	payload, _ := json.Marshal(map[string]string{})
	return asynq.NewTask(TypeExpand, payload)
}

// HandleSpawnOrganic opens density-capped rifts on hot cells (canon
// "infection>75% breeds rifts") — previously the only rift sources were hive
// pulses and PvP breaches, so the 95–100% world stayed sterile.
func (w *Worker) HandleSpawnOrganic(ctx context.Context, _ *asynq.Task) error {
	spawned, err := w.service.SpawnOrganic(ctx)
	if err != nil {
		return err
	}
	if spawned > 0 {
		slog.Info("organic rift spawn", "spawned", spawned)
	}
	return nil
}

func NewSpawnOrganicTask() *asynq.Task {
	payload, _ := json.Marshal(map[string]string{})
	return asynq.NewTask(TypeSpawnOrganic, payload)
}
