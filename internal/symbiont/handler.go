package symbiont

import (
	"net/http"
	"strconv"

	"github.com/ezra-game/server/pkg/httputil"
)

// Handler exposes the Symbiont status endpoint.
type Handler struct {
	service *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{service: svc}
}

// Status handles GET /symbiont/status?lat=&lng= — whether the Symbiont stands
// under a hostile dome (and applies the throttled soft-drain). Humans get a
// cheap is_symbiont=false. lat/lng default to 0 (treated as no coverage).
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	lat, _ := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	lng, _ := strconv.ParseFloat(r.URL.Query().Get("lng"), 64)
	st, err := h.service.Evaluate(r.Context(), playerID, lat, lng)
	if err != nil {
		httputil.Error(w, httputil.NewInternal("failed to evaluate symbiont status"))
		return
	}
	httputil.JSON(w, http.StatusOK, st)
}

// RaiseRequest is the body for the pocket-carving verb.
type RaiseRequest struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// Raise handles POST /symbiont/raise — raise infection on the current cell
// (gated to under-a-foreign-dome; charges energy, feeds Resonance).
func (h *Handler) Raise(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	var req RaiseRequest
	if err := httputil.Decode(r, &req); err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", err.Error()))
		return
	}
	res, err := h.service.RaiseInfection(r.Context(), playerID, req.Lat, req.Lng)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to raise infection"))
		return
	}
	httputil.JSON(w, http.StatusOK, res)
}

// Overload handles POST /symbiont/overload — corrode the nearest hostile beacon
// (gated to under-a-foreign-dome; charges energy, feeds Resonance on a hit).
func (h *Handler) Overload(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	var req RaiseRequest // same {lat,lng} body shape
	if err := httputil.Decode(r, &req); err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", err.Error()))
		return
	}
	res, err := h.service.Overload(r.Context(), playerID, req.Lat, req.Lng)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to overload beacon"))
		return
	}
	httputil.JSON(w, http.StatusOK, res)
}

// Entities handles GET /symbiont/entities — the Symbiont command screen: the
// player's entity roster (materialized from the Resonance Pool on first read)
// plus the control-slot budget. Symbiont-only.
func (h *Handler) Entities(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	res, err := h.service.Roster(r.Context(), playerID)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to load roster"))
		return
	}
	httputil.JSON(w, http.StatusOK, res)
}

// AssignRequest is the body for dispatching an entity. The target is acquired
// server-side near (lat,lng) based on the entity's archetype.
type AssignRequest struct {
	EntityID string  `json:"entity_id"`
	Lat      float64 `json:"lat"`
	Lng      float64 `json:"lng"`
}

// AssignEntity handles POST /symbiont/entities/assign — dispatch an entity; it
// auto-acquires its target (weakest hostile beacon / nearest open rift) near the
// player. Control-cap and RL-unlock enforced.
func (h *Handler) AssignEntity(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	var req AssignRequest
	if err := httputil.Decode(r, &req); err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", err.Error()))
		return
	}
	res, err := h.service.AssignEntity(r.Context(), playerID, req.EntityID, req.Lat, req.Lng)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to assign entity"))
		return
	}
	httputil.JSON(w, http.StatusOK, res)
}

// RecallRequest is the body for recalling an entity.
type RecallRequest struct {
	EntityID string `json:"entity_id"`
}

// RecallEntity handles POST /symbiont/entities/recall — disengage an entity into
// recovery.
func (h *Handler) RecallEntity(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	var req RecallRequest
	if err := httputil.Decode(r, &req); err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", err.Error()))
		return
	}
	res, err := h.service.RecallEntity(r.Context(), playerID, req.EntityID)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to recall entity"))
		return
	}
	httputil.JSON(w, http.StatusOK, res)
}

// Recon handles GET /symbiont/recon?lat=&lng= — free intel scan for the nearest
// most-vulnerable hostile beacon (Symbiont-only).
func (h *Handler) Recon(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	lat, _ := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	lng, _ := strconv.ParseFloat(r.URL.Query().Get("lng"), 64)
	res, err := h.service.Recon(r.Context(), playerID, lat, lng)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to scan network"))
		return
	}
	httputil.JSON(w, http.StatusOK, res)
}
