package pet

import (
	"log/slog"
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/pkg/httputil"
)

// Handler exposes pet HTTP endpoints.
type Handler struct {
	service *Service
	player  interface {
		AddXP(ctx context.Context, playerID string, amount int) (*player.Player, error)
	}
}

// NewHandler creates a new pet handler.
func NewHandler(svc *Service, playerSvc interface {
	AddXP(ctx context.Context, playerID string, amount int) (*player.Player, error)
}) *Handler {
	return &Handler{service: svc, player: playerSvc}
}

// SendRequest is the request body for sending a pet on a task.
type SendRequest struct {
	TaskType string `json:"task_type"`
}

// GetPets handles GET /pets — list player's pets.
func (h *Handler) GetPets(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	pets, err := h.service.GetByPlayer(r.Context(), playerID)
	if err != nil {
		httputil.Error(w, httputil.NewInternal("failed to get pets"))
		return
	}
	if pets == nil {
		pets = []Pet{}
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"pets": pets, "total": len(pets)})
}

// ClaimStarter handles POST /pets/starter-claim — grant the onboarding pet.
func (h *Handler) ClaimStarter(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())

	pet, profile, err := h.service.ClaimStarter(r.Context(), playerID)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to claim starter pet"))
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]any{
		"pet":        pet,
		"profile":    player.BuildProfilePayload(profile),
		"onboarding": player.BuildOnboardingPayload(profile),
	})
}

// Send handles POST /pets/{id}/send — send pet on a task.
func (h *Handler) Send(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	petID := chi.URLParam(r, "id")

	var req SendRequest
	if err := httputil.Decode(r, &req); err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", err.Error()))
		return
	}
	if req.TaskType == "" {
		req.TaskType = "search"
	}

	p, err := h.service.Send(r.Context(), petID, playerID, req.TaskType)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		slog.Error("pet send failed", "pet_id", petID, "player_id", playerID, "task", req.TaskType, "error", err)
		httputil.Error(w, httputil.NewInternal("failed to send pet"))
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]any{"pet": p})
}

// Recall handles POST /pets/{id}/recall — recall pet from task.
func (h *Handler) Recall(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	petID := chi.URLParam(r, "id")

	p, loot, err := h.service.Recall(r.Context(), petID, playerID)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to recall pet"))
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]any{"pet": p, "loot": loot})
	if h.player != nil {
		xp := 15
		if loot != nil && loot.Scout != nil {
			xp = 25
		}
		_, _ = h.player.AddXP(r.Context(), playerID, xp)
	}
}
