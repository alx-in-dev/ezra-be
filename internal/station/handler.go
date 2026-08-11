package station

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

// BuildRequest is the body for POST /stations.
type BuildRequest struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// Build handles POST /stations — build a power plant at the player's position.
func (h *Handler) Build(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	var req BuildRequest
	if err := httputil.Decode(r, &req); err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", err.Error()))
		return
	}
	p, err := h.service.Build(r.Context(), playerID, req.Lat, req.Lng)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to build station"))
		return
	}
	httputil.JSON(w, http.StatusCreated, map[string]any{"station": p})
}

// Upkeep handles POST /stations/upkeep — service the nearest owned plant.
func (h *Handler) Upkeep(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	var req BuildRequest
	if err := httputil.Decode(r, &req); err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", err.Error()))
		return
	}
	if err := h.service.Upkeep(r.Context(), playerID, req.Lat, req.Lng); err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to service station"))
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

// List handles GET /stations?lat&lng&radius_km — plants near the view.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	lat, _ := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	lng, _ := strconv.ParseFloat(r.URL.Query().Get("lng"), 64)
	radiusKm, err := strconv.ParseFloat(r.URL.Query().Get("radius_km"), 64)
	if err != nil || radiusKm <= 0 {
		radiusKm = 2.0
	}
	plants, err := h.service.Nearby(r.Context(), lat, lng, radiusKm*1000)
	if err != nil {
		httputil.Error(w, httputil.NewInternal("failed to list stations"))
		return
	}
	if plants == nil {
		plants = []PowerPlant{}
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"stations": plants})
}
