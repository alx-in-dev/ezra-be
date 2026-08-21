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
	// maxGrowingRifts caps how many open rifts expand in a single tick (T-827,
	// N0 §6: ~60 simultaneously-growing rifts/region). The regional source
	// budget (T-823) already bounds open rifts to ~60/region, so in the single-
	// region world this never truncates; it's a safety belt against a heavy
	// expand sweep. Overflow (a warning below) waits for the next tick.
	maxGrowingRifts = 60
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

	// T-827: cap simultaneously-growing rifts. Overflow waits for the next tick.
	if len(openRifts) > maxGrowingRifts {
		slog.Warn("rift expand: growing-rift cap hit", "open", len(openRifts), "cap", maxGrowingRifts)
		openRifts = openRifts[:maxGrowingRifts]
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
		touched, err := w.raiseInfectionInRadius(ctx, c.Lat, c.Lng, radiusM, expandInfectionPct)
		if err != nil {
			slog.Warn("rift expand: radius update failed", "rift_id", r.ID, "error", err)
			continue
		}
		cellsTouched += touched
	}

	slog.Info("rift expansion completed", "rifts_processed", len(openRifts), "cells_touched", cellsTouched)
	return nil
}

func NewExpandTask() *asynq.Task {
	payload, _ := json.Marshal(map[string]string{})
	return asynq.NewTask(TypeExpand, payload)
}

// raiseInfectionInRadius adds delta infection to every cell within radiusM in
// a single set-based UPDATE when the underlying cell.Repository supports it
// (production: cell.CachedRepository → cell.PgRepository.RaiseInfectionInRadius).
// Falls back to the old GetInRadius + per-cell Update loop otherwise (unit
// tests use a plain mock that doesn't implement the optional interface).
func (w *Worker) raiseInfectionInRadius(ctx context.Context, lat, lng, radiusM, delta float64) (int, error) {
	type raiser interface {
		RaiseInfectionInRadius(ctx context.Context, lat, lng, radiusM, delta float64) (int64, error)
	}
	if rr, ok := w.service.cells.(raiser); ok {
		touched, err := rr.RaiseInfectionInRadius(ctx, lat, lng, radiusM, delta)
		return int(touched), err
	}

	affected, err := w.service.cells.GetInRadius(ctx, lat, lng, radiusM)
	if err != nil {
		return 0, err
	}
	touched := 0
	for _, n := range affected {
		newInfection := n.Infection + delta
		if newInfection > 100 {
			newInfection = 100
		}
		if err := w.service.cells.Update(ctx, n.ID, newInfection); err != nil {
			slog.Warn("rift expand: cell update failed", "cell_id", n.ID, "error", err)
			continue
		}
		touched++
	}
	return touched, nil
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
	// T-823 floor: reignite regions with live players but no sources (localized
	// worlds have no infection≥75 cells for organic spawn to catch).
	floored, err := w.service.EnsureFloorSources(ctx)
	if err != nil {
		slog.Warn("rift floor pass failed", "error", err)
		return nil
	}
	if floored > 0 {
		slog.Info("rift floor spawn", "spawned", floored)
	}
	return nil
}

func NewSpawnOrganicTask() *asynq.Task {
	payload, _ := json.Marshal(map[string]string{})
	return asynq.NewTask(TypeSpawnOrganic, payload)
}
