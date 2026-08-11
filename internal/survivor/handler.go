package survivor

import (
	"net/http"
	"strconv"

	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/pkg/httputil"
)

// Handler exposes survivor HTTP endpoints.
type Handler struct {
	service *Service
}

// NewHandler creates a new survivor handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{service: svc}
}

// RecruitRequest is the body for POST /survivors/recruit.
type RecruitRequest struct {
	Type string  `json:"type"`
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
}

// Spawn handles GET /survivors?lat=...&lng=... — returns nearby survivors.
func (h *Handler) Spawn(w http.ResponseWriter, r *http.Request) {
	lat, err := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	if err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_lat", "lat is required"))
		return
	}
	lng, err := strconv.ParseFloat(r.URL.Query().Get("lng"), 64)
	if err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_lng", "lng is required"))
		return
	}

	playerID := httputil.GetPlayerID(r.Context())
	survivors := h.service.SpawnForPlayer(r.Context(), playerID, lat, lng)
	httputil.JSON(w, http.StatusOK, map[string]any{"survivors": survivors, "count": len(survivors)})
}

// Recruit handles POST /survivors/recruit — recruit a survivor as a unit.
func (h *Handler) Recruit(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())

	var req RecruitRequest
	if err := httputil.Decode(r, &req); err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", err.Error()))
		return
	}
	if req.Type == "" {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", "type is required"))
		return
	}

	u, err := h.service.Recruit(r.Context(), playerID, req.Type, req.Lat, req.Lng)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to recruit survivor"))
		return
	}

	p, err := h.service.players.GetByID(r.Context(), playerID)
	if err == nil && p != nil {
		httputil.JSON(w, http.StatusCreated, map[string]any{
			"unit":       u,
			"profile":    player.BuildProfilePayload(p),
			"onboarding": player.BuildOnboardingPayload(p),
		})
		return
	}

	httputil.JSON(w, http.StatusCreated, map[string]any{"unit": u})
}
