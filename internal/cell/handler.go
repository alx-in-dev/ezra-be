package cell

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/ezra-game/server/internal/canon"
	"github.com/ezra-game/server/pkg/httputil"
)

// TowerReader is the subset of tower.Repository needed by the cell handler.
// Defined here to avoid an import cycle (tower already imports cell).
type TowerReader interface {
	GetByIDs(ctx context.Context, ids []string) ([]TowerData, error)
	CountAllInRadius(ctx context.Context, lat, lng, radiusM float64) (int, error)
}

// TowerData mirrors the tower fields we need without importing the tower package.
type TowerData struct {
	ID            string  `db:"id"`
	OwnerID       string  `db:"owner_id"`
	Lat           float64 `db:"lat"`
	Lng           float64 `db:"lng"`
	Level         int     `db:"level"`
	RadiusM       float64 `db:"radius_m"`
	EffectPerHour float64 `db:"effect_per_hour"`
	HP            int     `db:"hp"`
	HPMax         int     `db:"hp_max"`
}

// RegionSeeder triggers lazy on-demand population of a geographic region.
// Defined as an interface so cell.Handler doesn't depend on the concrete
// Seeder type and can be nil-safe for tests.
type RegionSeeder interface {
	EnsureRegion(ctx context.Context, lat, lng float64) error
}

type Handler struct {
	repo      Repository
	towerRepo TowerReader
	seeder    RegionSeeder
}

func NewHandler(repo Repository, towerRepo TowerReader, seeder RegionSeeder) *Handler {
	return &Handler{repo: repo, towerRepo: towerRepo, seeder: seeder}
}

func (h *Handler) GetCells(w http.ResponseWriter, r *http.Request) {
	lat, err := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	if err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_param", "lat is required"))
		return
	}
	lng, err := strconv.ParseFloat(r.URL.Query().Get("lng"), 64)
	if err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_param", "lng is required"))
		return
	}
	radiusKm, err := strconv.ParseFloat(r.URL.Query().Get("radius_km"), 64)
	if err != nil || radiusKm <= 0 {
		radiusKm = 1.5
	}
	if radiusKm > 2.0 {
		httputil.Error(w, httputil.NewBadRequest("invalid_param", "radius_km must be <= 2.0"))
		return
	}

	// Lazy on-demand region seed: first call in a new 0.1° grid cell
	// triggers an Overpass fetch + cell generation. Subsequent calls
	// hit the seeded_regions fast path and return immediately.
	// On error we log and continue — the request still returns whatever
	// cells exist in the area, the client can retry next poll.
	if h.seeder != nil {
		if err := h.seeder.EnsureRegion(r.Context(), lat, lng); err != nil {
			slog.Warn("ensure region failed", "error", err, "lat", lat, "lng", lng)
		}
	}

	radiusM := radiusKm * 1000

	// Conditional fetch: the response only changes when the infection job
	// ticks (5 min) or a tower changes. The signature is a cheap aggregate
	// query; on a match we skip building ~500KB of JSON entirely and the
	// client skips parsing it.
	sig, sigErr := h.repo.StateSignature(r.Context(), lat, lng, radiusM)
	if sigErr != nil {
		slog.Warn("cells state signature failed", "error", sigErr)
	} else if clientTag := r.URL.Query().Get("etag"); clientTag != "" && clientTag == sig {
		httputil.JSON(w, http.StatusOK, map[string]any{"not_modified": true, "etag": sig})
		return
	}

	cells, err := h.repo.GetInRadius(r.Context(), lat, lng, radiusM)
	if err != nil {
		httputil.Error(w, httputil.NewInternal("failed to get cells"))
		return
	}

	// Collect non-nil tower IDs for batch fetch.
	var towerIDs []string
	for _, c := range cells {
		if c.TowerID != nil {
			towerIDs = append(towerIDs, *c.TowerID)
		}
	}

	// Batch-fetch towers and index by ID.
	towerMap := make(map[string]*TowerSummary, len(towerIDs))
	if len(towerIDs) > 0 {
		towers, err := h.towerRepo.GetByIDs(r.Context(), towerIDs)
		if err != nil {
			slog.Error("failed to fetch towers for cells", "error", err)
			// Non-fatal: return cells without tower data.
		} else {
			for _, t := range towers {
				towerMap[t.ID] = &TowerSummary{
					ID:            t.ID,
					OwnerID:       t.OwnerID,
					Lat:           t.Lat,
					Lng:           t.Lng,
					Level:         t.Level,
					RadiusM:       t.RadiusM,
					EffectPerHour: t.EffectPerHour,
					HP:            t.HP,
					HPMax:         t.HPMax,
					OverloadLimit: canon.TowerOverloadMaxCount,
				}
			}
		}
	}

	// Build DTOs.
	dtos := make([]CellDTO, len(cells))
	for i, c := range cells {
		dtos[i] = CellDTO{
			ID:                c.ID,
			Lat:               c.Lat,
			Lng:               c.Lng,
			Infection:         c.Infection,
			Terrain:           c.Terrain,
			TowerID:           c.TowerID,
			RiftID:            c.RiftID,
			HasNearbyBuilding: c.HasNearbyBuilding,
			HasNearbyRoad:     c.HasNearbyRoad,
			NearestRoadClass:  c.NearestRoadClass,
		}
		if c.TowerID != nil {
			if towerSummary := towerMap[*c.TowerID]; towerSummary != nil {
				overloadCount, err := h.towerRepo.CountAllInRadius(r.Context(), c.Lat, c.Lng, canon.TowerOverloadRadiusM)
				if err == nil {
					towerSummary.OverloadCount = overloadCount
					towerSummary.State = canon.ResolveTowerState(overloadCount)
				} else {
					towerSummary.State = canon.TowerStateActive
				}
				towerSummary.SafeZoneState = canon.InfectionBand(c.Infection)
				towerSummary.SafeZoneInfection = c.Infection
				dtos[i].Tower = towerSummary
			}
		}
	}

	httputil.JSON(w, http.StatusOK, map[string]any{
		"cells": dtos,
		"etag":  sig,
	})
}
