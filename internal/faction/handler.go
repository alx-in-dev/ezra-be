package faction

import (
	"net/http"

	"github.com/ezra-game/server/pkg/httputil"
)

type Handler struct {
	service *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{service: svc}
}

// Status handles GET /faction — the player's faction-onboarding state.
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	st, err := h.service.Status(r.Context(), httputil.GetPlayerID(r.Context()))
	if err != nil {
		httputil.Error(w, httputil.NewInternal("failed to get faction state"))
		return
	}
	httputil.JSON(w, http.StatusOK, st)
}

// ChooseRequest is the body for POST /faction/choose.
type ChooseRequest struct {
	Side string `json:"side"`
}

// Choose handles POST /faction/choose — declare allegiance after Contact.
func (h *Handler) Choose(w http.ResponseWriter, r *http.Request) {
	var req ChooseRequest
	if err := httputil.Decode(r, &req); err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", err.Error()))
		return
	}
	st, err := h.service.Choose(r.Context(), httputil.GetPlayerID(r.Context()), req.Side)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to choose faction"))
		return
	}
	httputil.JSON(w, http.StatusOK, st)
}
