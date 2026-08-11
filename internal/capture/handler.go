package capture

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ezra-game/server/pkg/httputil"
)

// Handler exposes capture HTTP endpoints.
type Handler struct {
	service *Service
}

// NewHandler creates a new capture handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{service: svc}
}

// Lockpick handles POST /towers/{id}/capture/lockpick — silent capture.
func (h *Handler) Lockpick(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	towerID := chi.URLParam(r, "id")

	t, err := h.service.Lockpick(r.Context(), towerID, playerID)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to capture tower"))
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]any{"tower": t})
}

// ForceCapture handles POST /towers/{id}/capture/force — validates and returns tower info.
// The actual battle creation is handled by the battle service; this endpoint validates proximity.
func (h *Handler) ForceCapture(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	towerID := chi.URLParam(r, "id")

	var req ForceCaptureRequest
	if err := httputil.Decode(r, &req); err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", err.Error()))
		return
	}

	t, err := h.service.ValidateForceCapture(r.Context(), towerID, playerID)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to validate capture"))
		return
	}

	// Return tower data so the client can proceed to create a battle
	httputil.JSON(w, http.StatusOK, map[string]any{
		"tower":    t,
		"squad_id": req.SquadID,
		"status":   "ready_for_battle",
	})
}
