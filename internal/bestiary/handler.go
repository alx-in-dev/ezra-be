package bestiary

import (
	"net/http"

	"github.com/ezra-game/server/pkg/httputil"
)

type Handler struct {
	service *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{service: svc}
}

// Get handles GET /bestiary — the full catalog with discovered flags, progress
// and (once) the completion reward.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	res, err := h.service.Status(r.Context(), playerID)
	if err != nil {
		httputil.Error(w, httputil.NewInternal("failed to load bestiary"))
		return
	}
	httputil.JSON(w, http.StatusOK, res)
}
