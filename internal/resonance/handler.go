package resonance

import (
	"net/http"

	"github.com/ezra-game/server/pkg/httputil"
)

// Handler exposes the resonance-endgame HTTP endpoints.
type Handler struct {
	service *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{service: svc}
}

// Status handles GET /resonance — spectrum collection + wave history.
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	st, err := h.service.Status(r.Context(), playerID)
	if err != nil {
		httputil.Error(w, httputil.NewInternal("failed to get resonance status"))
		return
	}
	httputil.JSON(w, http.StatusOK, st)
}

// Activate handles POST /resonance/activate — launch the counter-wave.
func (h *Handler) Activate(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	res, err := h.service.Activate(r.Context(), playerID)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to activate counter-wave"))
		return
	}
	httputil.JSON(w, http.StatusOK, res)
}
