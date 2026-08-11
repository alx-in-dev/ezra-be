package pvp

import (
	"net/http"
	"strconv"

	"github.com/ezra-game/server/pkg/httputil"
)

type Handler struct {
	service *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{service: svc}
}

// Targets handles GET /pvp/targets?lat=&lng= — nearby enemy domes (Symbiont).
func (h *Handler) Targets(w http.ResponseWriter, r *http.Request) {
	lat, err1 := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	lng, err2 := strconv.ParseFloat(r.URL.Query().Get("lng"), 64)
	if err1 != nil || err2 != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_param", "lat and lng are required"))
		return
	}
	targets, err := h.service.Targets(r.Context(), httputil.GetPlayerID(r.Context()), lat, lng)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to list pvp targets"))
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"targets": targets, "total": len(targets)})
}

// BreachRequest is the body for POST /pvp/breach.
type BreachRequest struct {
	CellID string `json:"cell_id"`
}

// Breach handles POST /pvp/breach — puncture an enemy dome.
func (h *Handler) Breach(w http.ResponseWriter, r *http.Request) {
	var req BreachRequest
	if err := httputil.Decode(r, &req); err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", err.Error()))
		return
	}
	res, err := h.service.Breach(r.Context(), httputil.GetPlayerID(r.Context()), req.CellID)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to breach dome"))
		return
	}
	httputil.JSON(w, http.StatusOK, res)
}
