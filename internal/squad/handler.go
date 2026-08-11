package squad

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ezra-game/server/pkg/httputil"
)

// Handler exposes squad HTTP endpoints.
type Handler struct {
	service *Service
}

// NewHandler creates a new squad handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{service: svc}
}

// List handles GET /squads — list player's squads.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())

	squads, err := h.service.List(r.Context(), playerID)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to list squads"))
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]any{"squads": squads, "total": len(squads)})
}

// CreateRequest is the request body for creating a squad.
type CreateRequest struct {
	Name    string   `json:"name"`
	UnitIDs []string `json:"unit_ids"`
}

// Create handles POST /squads — create a new squad.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())

	var req CreateRequest
	if err := httputil.Decode(r, &req); err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", err.Error()))
		return
	}

	sq, err := h.service.Create(r.Context(), playerID, req.Name, req.UnitIDs)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to create squad"))
		return
	}

	httputil.JSON(w, http.StatusCreated, map[string]any{"squad": sq})
}

// SendRequest is the request body for sending a squad on a mission.
type SendRequest struct {
	MissionType string `json:"mission_type"`
	TargetID    string `json:"target_id"`
}

// Send handles POST /squads/{id}/send — send a squad on a mission.
func (h *Handler) Send(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	squadID := chi.URLParam(r, "id")

	var req SendRequest
	if err := httputil.Decode(r, &req); err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", err.Error()))
		return
	}

	sq, err := h.service.Send(r.Context(), squadID, playerID, req.MissionType, req.TargetID)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to send squad"))
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]any{"squad": sq})
}

// UpdateRequest is the request body for editing a squad (rename + membership).
type UpdateRequest struct {
	Name    string   `json:"name"`
	UnitIDs []string `json:"unit_ids"`
}

// Update handles PATCH /squads/{id} — rename and/or set the squad's units.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	squadID := chi.URLParam(r, "id")

	var req UpdateRequest
	if err := httputil.Decode(r, &req); err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", err.Error()))
		return
	}

	sq, err := h.service.UpdateComposition(r.Context(), squadID, playerID, req.Name, req.UnitIDs)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to update squad"))
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]any{"squad": sq})
}

// Disband handles DELETE /squads/{id} — disband an idle squad, freeing its units.
func (h *Handler) Disband(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	squadID := chi.URLParam(r, "id")

	if err := h.service.Disband(r.Context(), squadID, playerID); err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to disband squad"))
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]any{"disbanded": true})
}

// Return handles POST /squads/{id}/recall — recall a squad from a mission.
func (h *Handler) Return(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	squadID := chi.URLParam(r, "id")

	sq, err := h.service.Return(r.Context(), squadID, playerID)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to recall squad"))
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]any{"squad": sq})
}
