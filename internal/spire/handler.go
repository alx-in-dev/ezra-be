package spire

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

func latlng(r *http.Request) (float64, float64, bool) {
	lat, err1 := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	lng, err2 := strconv.ParseFloat(r.URL.Query().Get("lng"), 64)
	return lat, lng, err1 == nil && err2 == nil
}

// Status handles GET /spire?lat=&lng= — the region's Spire build state.
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	lat, lng, ok := latlng(r)
	if !ok {
		httputil.Error(w, httputil.NewBadRequest("invalid_param", "lat and lng are required"))
		return
	}
	st, err := h.service.Status(r.Context(), httputil.GetPlayerID(r.Context()), lat, lng)
	if err != nil {
		httputil.Error(w, httputil.NewInternal("failed to get spire status"))
		return
	}
	httputil.JSON(w, http.StatusOK, st)
}

// ContributeFragment handles POST /spire/contribute-fragment?lat=&lng=.
func (h *Handler) ContributeFragment(w http.ResponseWriter, r *http.Request) {
	h.contribute(w, r, true)
}

// ContributeResources handles POST /spire/contribute?lat=&lng=.
func (h *Handler) ContributeResources(w http.ResponseWriter, r *http.Request) {
	h.contribute(w, r, false)
}

func (h *Handler) contribute(w http.ResponseWriter, r *http.Request, fragment bool) {
	lat, lng, ok := latlng(r)
	if !ok {
		httputil.Error(w, httputil.NewBadRequest("invalid_param", "lat and lng are required"))
		return
	}
	playerID := httputil.GetPlayerID(r.Context())
	var st *Status
	var err error
	if fragment {
		st, err = h.service.ContributeFragment(r.Context(), playerID, lat, lng)
	} else {
		st, err = h.service.ContributeResources(r.Context(), playerID, lat, lng)
	}
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to contribute to spire"))
		return
	}
	httputil.JSON(w, http.StatusOK, st)
}
