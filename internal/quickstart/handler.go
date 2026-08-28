package quickstart

import (
	"net/http"

	"github.com/ezra-game/server/internal/faction"
	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/pkg/httputil"
)

type Handler struct {
	service *Service
	faction *faction.Service
	players *player.Service
}

func NewHandler(svc *Service, factionSvc *faction.Service, players *player.Service) *Handler {
	return &Handler{service: svc, faction: factionSvc, players: players}
}

type startRequest struct {
	Side string `json:"side"`
}

// Start handles POST /onboarding/quick-start — the very first onboarding
// gate's "choose a side immediately" option (docs/feature/onboarding_quick_start.md).
func (h *Handler) Start(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	var req startRequest
	if err := httputil.Decode(r, &req); err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", err.Error()))
		return
	}

	switch req.Side {
	case faction.Human:
		p, err := h.service.StartHuman(r.Context(), playerID)
		if err != nil {
			writeErr(w, err)
			return
		}
		httputil.JSON(w, http.StatusOK, map[string]any{
			"onboarding": player.BuildOnboardingPayload(p),
		})
	case faction.Symbiont:
		if err := h.service.FinishSymbiont(r.Context(), playerID); err != nil {
			writeErr(w, err)
			return
		}
		p, err := h.players.GetByID(r.Context(), playerID)
		if err != nil {
			httputil.Error(w, httputil.NewInternal("failed to load player"))
			return
		}
		httputil.JSON(w, http.StatusOK, map[string]any{
			"onboarding": player.BuildOnboardingPayload(p),
		})
	default:
		httputil.Error(w, httputil.NewBadRequest("invalid_side", "side must be human or symbiont"))
	}
}

func writeErr(w http.ResponseWriter, err error) {
	if appErr, ok := err.(*httputil.AppError); ok {
		httputil.Error(w, appErr)
		return
	}
	httputil.Error(w, httputil.NewInternal("quick-start failed"))
}
