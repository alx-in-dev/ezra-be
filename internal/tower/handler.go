package tower

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ezra-game/server/internal/canon"
	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/pkg/httputil"
)

// Handler exposes tower HTTP endpoints.
type Handler struct {
	service *Service
	player  interface {
		AddXP(ctx context.Context, playerID string, amount int) (*player.Player, error)
	}
}

// NewHandler creates a new tower handler.
func NewHandler(svc *Service, playerSvc interface {
	AddXP(ctx context.Context, playerID string, amount int) (*player.Player, error)
}) *Handler {
	return &Handler{service: svc, player: playerSvc}
}

// PlaceRequest is the request body for tower creation.
type PlaceRequest struct {
	CellID string `json:"cell_id"`
}

func (h *Handler) playerResourceSnapshot(ctx context.Context, playerID string) map[string]any {
	p, err := h.service.players.GetByID(ctx, playerID)
	if err != nil || p == nil {
		return nil
	}
	return map[string]any{
		"energy":                   p.Energy,
		"energy_max":               p.EnergyMax(),
		"materials":                p.Materials,
		"materials_max":            p.MaterialsMax(),
		"onboarding_step":          p.OnboardingStep,
		"starter_beacon_available": p.StarterBeaconAvailable,
	}
}

func (h *Handler) playerProfileSnapshot(ctx context.Context, playerID string) map[string]any {
	p, err := h.service.players.GetByID(ctx, playerID)
	if err != nil || p == nil {
		return nil
	}
	return player.BuildProfilePayload(p)
}

// Create handles POST /towers — place a new tower.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())

	var req PlaceRequest
	if err := httputil.Decode(r, &req); err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", err.Error()))
		return
	}
	// Empty cell_id = "place at my current location": server picks the cell
	// nearest the player's position (no client-side cell selection).
	var t *Tower
	var err error
	if req.CellID == "" {
		t, err = h.service.PlaceAtPlayer(r.Context(), playerID)
	} else {
		t, err = h.service.Place(r.Context(), playerID, req.CellID)
	}
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to place tower"))
		return
	}

	httputil.JSON(w, http.StatusCreated, map[string]any{
		"tower":     h.enrichTower(r.Context(), t),
		"resources": h.playerResourceSnapshot(r.Context(), playerID),
		"profile":   h.playerProfileSnapshot(r.Context(), playerID),
	})
	if h.player != nil {
		_, _ = h.player.AddXP(r.Context(), playerID, 25)
	}
}

// Upgrade handles PATCH /towers/{id}/upgrade — upgrade a tower.
func (h *Handler) Upgrade(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	towerID := chi.URLParam(r, "id")

	t, err := h.service.Upgrade(r.Context(), towerID, playerID)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to upgrade tower"))
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]any{
		"tower":     h.enrichTower(r.Context(), t),
		"resources": h.playerResourceSnapshot(r.Context(), playerID),
	})
}

// Repair handles POST /towers/{id}/repair — restore a damaged beacon to full
// HP for energy, on site (the defense-race counter to infection pressure).
func (h *Handler) Repair(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	towerID := chi.URLParam(r, "id")

	t, err := h.service.Repair(r.Context(), towerID, playerID)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to repair tower"))
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]any{
		"tower":     h.enrichTower(r.Context(), t),
		"resources": h.playerResourceSnapshot(r.Context(), playerID),
	})
}

// Delete handles DELETE /towers/{id} — remove a tower.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	towerID := chi.URLParam(r, "id")

	energy, materials, err := h.service.Remove(r.Context(), towerID, playerID)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to delete tower"))
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]any{
		"returned":  map[string]int{"energy": energy, "materials": materials},
		"resources": h.playerResourceSnapshot(r.Context(), playerID),
	})
}

// GetMine handles GET /towers/mine — list player's towers.
func (h *Handler) GetMine(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	towers, err := h.service.towers.GetByOwner(r.Context(), playerID)
	if err != nil {
		httputil.Error(w, httputil.NewInternal("failed to get towers"))
		return
	}
	for i := range towers {
		h.enrichTower(r.Context(), &towers[i])
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"towers": towers, "total": len(towers)})
}

func (h *Handler) enrichTower(rctx context.Context, t *Tower) *Tower {
	if t == nil {
		return nil
	}

	c, err := h.service.cells.GetByID(rctx, t.CellID)
	if err != nil || c == nil {
		return t
	}

	overloadCount, err := h.service.towers.CountAllInRadius(rctx, c.Lat, c.Lng, canon.TowerOverloadRadiusM)
	if err == nil {
		// Place-aware grid capacity (#7) so the detail view's limit/state match
		// what placement enforces. Single-tower path → the extra count is fine.
		capacity := h.service.areaCapacity(rctx, c.Lat, c.Lng)
		t.OverloadCount = overloadCount
		t.OverloadLimit = capacity
		t.State = canon.ResolveTowerStateCap(overloadCount, capacity)
	} else if t.State == "" {
		t.OverloadLimit = canon.TowerOverloadMaxCount
		t.State = canon.TowerStateActive
	}

	t.SafeZoneState = canon.InfectionBand(c.Infection)
	t.SafeZoneInfection = c.Infection
	return t
}
