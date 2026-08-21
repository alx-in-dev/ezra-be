package spirit

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/ezra-game/server/pkg/httputil"
)

// Handler exposes wild-spirit HTTP endpoints (N2/N4). All behind auth middleware.
type Handler struct {
	service *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{service: svc} }

// Nearby handles GET /spirits/nearby?lat=&lng=&radius_m= — visible wild spirits
// near a point, for the map layer (US-SW1). Position is the live interpolated one.
func (h *Handler) Nearby(w http.ResponseWriter, r *http.Request) {
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
	spirits, err := h.service.GetNearby(r.Context(), lat, lng, radiusM)
	if err != nil {
		httputil.Error(w, httputil.NewInternal("failed to list spirits"))
		return
	}
	if spirits == nil {
		spirits = []Spirit{}
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"spirits": spirits, "total": len(spirits)})
}

// Weaken handles POST /spirits/{id}/weaken — soften a spirit toward tameable (N4).
func (h *Handler) Weaken(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	id := chi.URLParam(r, "id")
	sp, err := h.service.Weaken(r.Context(), playerID, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"spirit": sp})
}

// Tame handles POST /spirits/{id}/tame — attempt to tame it into the roster (N4).
func (h *Handler) Tame(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	id := chi.URLParam(r, "id")
	tamed, err := h.service.Tame(r.Context(), playerID, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"tamed": tamed})
}

// Repel handles POST /spirits/repel?lat=&lng= — spend a hack_key to clear live
// spirits around the player (the Ionized Charge repellent, N5/T-881).
func (h *Handler) Repel(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
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
	repelled, err := h.service.Repel(r.Context(), playerID, lat, lng)
	if err != nil {
		writeErr(w, err)
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"repelled": repelled})
}

func writeErr(w http.ResponseWriter, err error) {
	if appErr, ok := err.(*httputil.AppError); ok {
		httputil.Error(w, appErr)
		return
	}
	httputil.Error(w, httputil.NewInternal(err.Error()))
}
