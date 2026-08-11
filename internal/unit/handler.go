package unit

import (
	"net/http"

	"github.com/ezra-game/server/pkg/httputil"
)

// Handler exposes unit HTTP endpoints.
type Handler struct {
	service *Service
}

// NewHandler creates a new unit handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{service: svc}
}

// RecruitRequest is the request body for recruiting a unit.
type RecruitRequest struct {
	Type string `json:"type"`
}

// Recruit handles POST /units — recruit a new unit.
func (h *Handler) Recruit(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())

	var req RecruitRequest
	if err := httputil.Decode(r, &req); err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", err.Error()))
		return
	}

	u, err := h.service.Recruit(r.Context(), playerID, req.Type)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to recruit unit"))
		return
	}

	httputil.JSON(w, http.StatusCreated, map[string]any{"unit": u})
}

// List handles GET /units — list player's units with army count/limit.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())

	units, count, limit, retention, err := h.service.ListByPlayer(r.Context(), playerID)
	if err != nil {
		httputil.Error(w, httputil.NewInternal("failed to list units"))
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]any{
		"units":      units,
		"army_count": count,
		"army_limit": limit,
		"retention":  retention,
	})
}
