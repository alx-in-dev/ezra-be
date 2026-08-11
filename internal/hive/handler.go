package hive

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/ezra-game/server/pkg/httputil"
)

// Handler exposes hive HTTP endpoints.
type Handler struct {
	service *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{service: svc}
}

// List handles GET /hives?lat=&lng=&radius_m= — open Symbiont hives near a
// point, for the client map (mirrors the survivors/rifts fetch pattern).
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	lat, err := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	if err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_param", "lat is required"))
		return
	}
	lng, err := strconv.ParseFloat(r.URL.Query().Get("lng"), 64)
	if err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_param", "lng is required"))
		return
	}
	radiusM, err := strconv.ParseFloat(r.URL.Query().Get("radius_m"), 64)
	if err != nil || radiusM <= 0 {
		radiusM = 2000
	}
	if radiusM > 5000 {
		radiusM = 5000
	}

	hives, err := h.service.GetNearby(r.Context(), lat, lng, radiusM)
	if err != nil {
		httputil.Error(w, httputil.NewInternal("failed to list hives"))
		return
	}
	if hives == nil {
		hives = []Hive{}
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"hives": hives, "total": len(hives)})
}

// Empower handles POST /hives/{id}/empower — the Symbiont feeds a hive.
func (h *Handler) Empower(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	hiveID := chi.URLParam(r, "id")
	hv, err := h.service.Empower(r.Context(), playerID, hiveID)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to empower hive"))
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"hive": hv})
}
