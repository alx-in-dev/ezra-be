package player

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ezra-game/server/pkg/geo"
	"github.com/ezra-game/server/pkg/httputil"
)

type Handler struct {
	service *Service
	// posObserver is an optional hook run after a validated position update
	// (N2: the spirit service checks for a field touch). Kept an interface so
	// player stays decoupled from spirit.
	posObserver PositionObserver
}

// PositionObserver is notified of a player's validated new position.
type PositionObserver interface {
	OnPlayerMoved(ctx context.Context, playerID string, lat, lng float64)
}

// SetPositionObserver wires the optional post-move hook (fluent-free setter).
func (h *Handler) SetPositionObserver(o PositionObserver) { h.posObserver = o }

// maxSpeedKmh is the anti-cheat movement speed limit (W-03). Overridable via
// MAX_SPEED_KMH so dev environments can teleport with FakeGPS without every
// PATCH /player/position getting stuck behind impossible_speed.
var maxSpeedKmh = func() float64 {
	if v := os.Getenv("MAX_SPEED_KMH"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			return f
		}
	}
	return 50.0
}()

type UpdateUsernameRequest struct {
	Username string `json:"username"`
}

type AdvanceOnboardingRequest struct {
	Action string `json:"action"`
}

func NewHandler(svc *Service) *Handler {
	return &Handler{service: svc}
}

// GetPlayer returns the current player profile.
func (h *Handler) GetPlayer(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	p, err := h.service.GetByID(r.Context(), playerID)
	if err != nil {
		httputil.Error(w, httputil.NewInternal("failed to get player"))
		return
	}

	httputil.JSON(w, http.StatusOK, BuildProfilePayload(p))
}

func (h *Handler) GetProfile(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	p, err := h.service.GetByID(r.Context(), playerID)
	if err != nil {
		httputil.Error(w, httputil.NewInternal("failed to get profile"))
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]any{"profile": BuildProfilePayload(p)})
}

func (h *Handler) GetSkills(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	tree, err := h.service.GetSkillTree(r.Context(), playerID)
	if err != nil {
		httputil.Error(w, httputil.NewInternal("failed to get skills"))
		return
	}

	httputil.JSON(w, http.StatusOK, tree)
}

func (h *Handler) UnlockSkill(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	skillID := chi.URLParam(r, "id")

	p, def, err := h.service.UnlockSkill(r.Context(), playerID, skillID)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to unlock skill"))
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]any{
		"profile": BuildProfilePayload(p),
		"skill": map[string]any{
			"id":          def.ID,
			"branch":      def.Branch,
			"name":        def.Name,
			"description": def.Description,
			"tier":        def.Tier,
		},
	})
}

func (h *Handler) UpdateUsername(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	var req UpdateUsernameRequest
	if err := httputil.Decode(r, &req); err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", err.Error()))
		return
	}

	p, err := h.service.UpdateUsername(r.Context(), playerID, req.Username)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to update username"))
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]any{
		"profile":    BuildProfilePayload(p),
		"onboarding": BuildOnboardingPayload(p),
	})
}

func (h *Handler) GetOnboarding(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	p, err := h.service.GetByID(r.Context(), playerID)
	if err != nil {
		httputil.Error(w, httputil.NewInternal("failed to get onboarding"))
		return
	}

	httputil.JSON(w, http.StatusOK, BuildOnboardingPayload(p))
}

func (h *Handler) AdvanceOnboarding(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	var req AdvanceOnboardingRequest
	if err := httputil.Decode(r, &req); err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", err.Error()))
		return
	}

	p, err := h.service.AdvanceOnboarding(r.Context(), playerID, req.Action)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to advance onboarding"))
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]any{
		"profile":    BuildProfilePayload(p),
		"onboarding": BuildOnboardingPayload(p),
	})
}

type UpdatePositionRequest struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// UpdatePosition updates player position with anti-cheat validation.
func (h *Handler) UpdatePosition(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())

	var req UpdatePositionRequest
	if err := httputil.Decode(r, &req); err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", err.Error()))
		return
	}

	p, err := h.service.GetByID(r.Context(), playerID)
	if err != nil {
		httputil.Error(w, httputil.NewInternal("failed to get player"))
		return
	}

	// Anti-cheat: validate speed < 50 km/h
	if p.Position.Lat != 0 || p.Position.Lng != 0 {
		elapsed := time.Since(p.Position.UpdatedAt).Seconds()
		// Rapid successive updates (client re-sync, async double-send) give
		// elapsed≈0, turning GPS jitter into "infinite" speed (W-03). Clamp
		// to 1s instead of skipping the check so jitter-scale moves pass
		// while teleports within the same second are still rejected.
		if elapsed < 1.0 {
			elapsed = 1.0
		}
		if err := geo.ValidateSpeed(p.Position.Lat, p.Position.Lng, req.Lat, req.Lng, elapsed, maxSpeedKmh); err != nil {
			slog.Warn("position REJECTED: impossible_speed — server keeps the old point",
				"player_id", playerID,
				"old_lat", p.Position.Lat, "old_lng", p.Position.Lng,
				"new_lat", req.Lat, "new_lng", req.Lng,
				"dist_m", geo.Haversine(p.Position.Lat, p.Position.Lng, req.Lat, req.Lng),
				"elapsed_s", elapsed)
			httputil.Error(w, &httputil.AppError{Code: "impossible_speed", Message: err.Error(), Status: 422})
			return
		}
	}

	if err := h.service.repo.UpdatePosition(r.Context(), playerID, req.Lat, req.Lng); err != nil {
		httputil.Error(w, httputil.NewInternal("failed to update position"))
		return
	}

	slog.Info("position updated",
		"player_id", playerID,
		"lat", req.Lat, "lng", req.Lng,
		"moved_m", geo.Haversine(p.Position.Lat, p.Position.Lng, req.Lat, req.Lng))

	// N2: let the spirit service check for a field touch at the new position
	// (novice-inert players are ignored inside the observer).
	if h.posObserver != nil {
		h.posObserver.OnPlayerMoved(r.Context(), playerID, req.Lat, req.Lng)
	}

	httputil.JSON(w, http.StatusOK, map[string]any{
		"position": map[string]any{"lat": req.Lat, "lng": req.Lng, "updated_at": time.Now()},
	})
}
