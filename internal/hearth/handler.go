package hearth

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

// SummonRequest is the body for POST /symbiont/hearth.
type SummonRequest struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// Summon handles POST /symbiont/hearth — plant an Ephemeral Hearth at the
// player's position.
func (h *Handler) Summon(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	var req SummonRequest
	if err := httputil.Decode(r, &req); err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", err.Error()))
		return
	}
	res, err := h.service.Summon(r.Context(), playerID, req.Lat, req.Lng)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to summon hearth"))
		return
	}
	httputil.JSON(w, http.StatusCreated, map[string]any{"hearth": res})
}
