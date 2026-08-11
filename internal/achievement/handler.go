package achievement

import (
	"net/http"

	"github.com/ezra-game/server/pkg/httputil"
)

// Handler exposes the achievements endpoint.
type Handler struct {
	service *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{service: svc}
}

// Get handles GET /achievements — the full achievement list with live progress;
// any milestone newly met on this call is unlocked (reward granted once) and
// echoed in newly_unlocked so the client can toast it.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	res, err := h.service.Evaluate(r.Context(), playerID)
	if err != nil {
		httputil.Error(w, httputil.NewInternal("failed to evaluate achievements"))
		return
	}
	httputil.JSON(w, http.StatusOK, res)
}
