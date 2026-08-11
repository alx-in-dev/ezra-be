package battle

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ezra-game/server/pkg/httputil"
)

// Handler exposes battle HTTP endpoints.
type Handler struct {
	service *Service
}

// NewHandler creates a new battle handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{service: svc}
}

// StartRequest is the request body for POST /battles/start.
type StartRequest struct {
	SquadID    string `json:"squad_id"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
}

// ActionRequest is the request body for POST /battles/{id}/action.
type ActionRequest struct {
	Tactic string `json:"tactic"`
}

// Start handles POST /battles/start — initiate a new battle.
func (h *Handler) Start(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())

	var req StartRequest
	if err := httputil.Decode(r, &req); err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", err.Error()))
		return
	}
	if req.SquadID == "" || req.TargetType == "" || req.TargetID == "" {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", "squad_id, target_type, and target_id are required"))
		return
	}

	b, err := h.service.Start(r.Context(), playerID, req.SquadID, req.TargetType, req.TargetID)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		// Non-AppError → unexpected failure (DB constraint, network, etc).
		// Log the raw error so we can diagnose without needing to dig into
		// request body & service trace separately.
		slog.Error("battle start failed",
			"player_id", playerID,
			"squad_id", req.SquadID,
			"target_type", req.TargetType,
			"target_id", req.TargetID,
			"error", err)
		httputil.Error(w, httputil.NewInternal("failed to start battle"))
		return
	}

	snapshot, err := h.service.Snapshot(r.Context(), b)
	if err != nil {
		httputil.Error(w, httputil.NewInternal("failed to build battle snapshot"))
		return
	}

	httputil.JSON(w, http.StatusCreated, snapshot)
}

// Action handles POST /battles/{id}/action — execute a round with a tactic.
func (h *Handler) Action(w http.ResponseWriter, r *http.Request) {
	battleID := chi.URLParam(r, "id")

	var req ActionRequest
	if err := httputil.Decode(r, &req); err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", err.Error()))
		return
	}
	if req.Tactic == "" {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", "tactic is required"))
		return
	}

	b, err := h.service.ExecuteRound(r.Context(), battleID, req.Tactic)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		slog.Error("battle execute round failed",
			"battle_id", battleID, "tactic", req.Tactic, "error", err)
		httputil.Error(w, httputil.NewInternal("failed to execute round"))
		return
	}

	snapshot, err := h.service.Snapshot(r.Context(), b)
	if err != nil {
		httputil.Error(w, httputil.NewInternal("failed to build battle snapshot"))
		return
	}

	httputil.JSON(w, http.StatusOK, snapshot)
}

// Overcharge handles POST /battles/{id}/overcharge — activate overcharge.
func (h *Handler) Overcharge(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	battleID := chi.URLParam(r, "id")

	b, err := h.service.Overcharge(r.Context(), battleID, playerID)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		slog.Error("battle overcharge failed",
			"battle_id", battleID, "player_id", playerID, "error", err)
		httputil.Error(w, httputil.NewInternal("failed to activate overcharge"))
		return
	}

	// Return the same normalized Snapshot shape as Action so the client renders
	// the resulting round/HP/loot/level-ups uniformly (was raw {"battle": b},
	// which the client couldn't parse — overcharge results never showed).
	snapshot, err := h.service.Snapshot(r.Context(), b)
	if err != nil {
		httputil.Error(w, httputil.NewInternal("failed to build battle snapshot"))
		return
	}

	httputil.JSON(w, http.StatusOK, snapshot)
}
