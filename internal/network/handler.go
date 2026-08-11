package network

import (
	"net/http"
	"strconv"

	"github.com/ezra-game/server/pkg/httputil"
)

// Handler exposes beacon-network HTTP endpoints.
type Handler struct {
	service *Service
}

// NewHandler creates a network handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{service: svc}
}

// CoreRequest is the body for placing/relocating a Core.
type CoreRequest struct {
	CellID string `json:"cell_id"`
}

// PlaceCore handles POST /core.
func (h *Handler) PlaceCore(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	var req CoreRequest
	if err := httputil.Decode(r, &req); err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", err.Error()))
		return
	}
	// Empty cell_id = "place at my current location" (server picks the nearest
	// cell to the player's position).
	core, err := h.service.PlaceCore(r.Context(), playerID, req.CellID)
	if err != nil {
		writeErr(w, err)
		return
	}
	httputil.JSON(w, http.StatusCreated, map[string]any{"core": core})
}

// RelocateCore handles POST /core/relocate.
func (h *Handler) RelocateCore(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	var req CoreRequest
	if err := httputil.Decode(r, &req); err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", err.Error()))
		return
	}
	// Empty cell_id = "place at my current location" (server picks the nearest
	// cell to the player's position).
	core, err := h.service.RelocateCore(r.Context(), playerID, req.CellID)
	if err != nil {
		writeErr(w, err)
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"core": core})
}

// UpgradeCore handles POST /core/upgrade.
func (h *Handler) UpgradeCore(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	core, err := h.service.UpgradeCore(r.Context(), playerID)
	if err != nil {
		writeErr(w, err)
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"core": core})
}

// GetNetwork handles GET /network.
func (h *Handler) GetNetwork(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	state, err := h.service.GetNetwork(r.Context(), playerID)
	if err != nil {
		writeErr(w, err)
		return
	}
	httputil.JSON(w, http.StatusOK, state)
}

// GetRegionFields handles GET /map/fields?lat=&lng=&radius_km= — the merged
// energy-field overlay for the view: powered disks of ALL players in range,
// ownership stripped, so the client renders neighbouring domes as one shield.
func (h *Handler) GetRegionFields(w http.ResponseWriter, r *http.Request) {
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
	radiusKm, err := strconv.ParseFloat(r.URL.Query().Get("radius_km"), 64)
	if err != nil || radiusKm <= 0 {
		radiusKm = 1.5
	}
	if radiusKm > 2.0 {
		httputil.Error(w, httputil.NewBadRequest("invalid_param", "radius_km must be <= 2.0"))
		return
	}
	disks, err := h.service.RegionFields(r.Context(), lat, lng, radiusKm*1000)
	if err != nil {
		writeErr(w, err)
		return
	}
	if disks == nil {
		disks = []FieldDisk{}
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"disks": disks})
}

func writeErr(w http.ResponseWriter, err error) {
	if appErr, ok := err.(*httputil.AppError); ok {
		httputil.Error(w, appErr)
		return
	}
	httputil.Error(w, httputil.NewInternal("network operation failed"))
}
