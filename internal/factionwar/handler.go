package factionwar

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

// Status handles GET /faction-war?lat=&lng= — the regional Human-vs-Symbiont
// balance, the player's seasonal contribution, and the leaderboard.
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
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
	status, err := h.service.Status(r.Context(), playerID, lat, lng)
	if err != nil {
		httputil.Error(w, httputil.NewInternal("failed to get faction war status"))
		return
	}
	httputil.JSON(w, http.StatusOK, status)
}

// Settle handles POST /faction-war/settle?season= — pays out a finished season.
// Idempotent and refuses the current (ongoing) season, so it is safe to expose;
// the daily worker also runs it automatically.
func (h *Handler) Settle(w http.ResponseWriter, r *http.Request) {
	season := r.URL.Query().Get("season")
	if season == "" {
		httputil.Error(w, httputil.NewBadRequest("invalid_param", "season is required"))
		return
	}
	if err := h.service.SettleSeason(r.Context(), season); err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to settle season"))
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"settled": season})
}
