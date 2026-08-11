package push

import (
	"net/http"

	"github.com/ezra-game/server/pkg/httputil"
)

// Handler exposes push notification HTTP endpoints.
type Handler struct {
	service *Service
}

// NewHandler creates a new push handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{service: svc}
}

// RegisterTokenRequest is the body for POST /player/push-token.
type RegisterTokenRequest struct {
	FCMToken string `json:"fcm_token"`
	Platform string `json:"platform"`
}

// RegisterToken handles POST /player/push-token — register or update FCM token.
func (h *Handler) RegisterToken(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())

	var req RegisterTokenRequest
	if err := httputil.Decode(r, &req); err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", err.Error()))
		return
	}
	if req.FCMToken == "" {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", "fcm_token is required"))
		return
	}

	if err := h.service.RegisterToken(r.Context(), playerID, req.FCMToken, req.Platform); err != nil {
		httputil.Error(w, httputil.NewInternal("failed to register push token"))
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"status": "registered"})
}
