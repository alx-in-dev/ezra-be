package citylink

import (
	"net/http"

	"github.com/ezra-game/server/pkg/httputil"
	"github.com/go-chi/chi/v5"
)

// Handler exposes the City Link HTTP surface.
type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func writeErr(w http.ResponseWriter, err error, fallback string) {
	if appErr, ok := err.(*httputil.AppError); ok {
		httputil.Error(w, appErr)
		return
	}
	httputil.Error(w, httputil.NewInternal(fallback))
}

// Search handles GET /city-links/search?q= — find players to link with.
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	res, err := h.svc.Search(r.Context(), playerID, r.URL.Query().Get("q"))
	if err != nil {
		writeErr(w, err, "search failed")
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"players": res})
}

// RequestBody is the propose-link payload.
type RequestBody struct {
	ToPlayerID string `json:"to_player_id"`
}

// Request handles POST /city-links/request — propose a link.
func (h *Handler) Request(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	var body RequestBody
	if err := httputil.Decode(r, &body); err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", err.Error()))
		return
	}
	req, err := h.svc.Request(r.Context(), playerID, body.ToPlayerID)
	if err != nil {
		writeErr(w, err, "request failed")
		return
	}
	httputil.JSON(w, http.StatusOK, req)
}

// Inbox handles GET /city-links/requests — pending incoming requests.
func (h *Handler) Inbox(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	res, err := h.svc.Inbox(r.Context(), playerID)
	if err != nil {
		writeErr(w, err, "inbox failed")
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"requests": res})
}

// Accept handles POST /city-links/requests/{id}/accept.
func (h *Handler) Accept(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	req, err := h.svc.Accept(r.Context(), playerID, chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, err, "accept failed")
		return
	}
	httputil.JSON(w, http.StatusOK, req)
}

// Reject handles POST /city-links/requests/{id}/reject.
func (h *Handler) Reject(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	if err := h.svc.Reject(r.Context(), playerID, chi.URLParam(r, "id")); err != nil {
		writeErr(w, err, "reject failed")
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

// List handles GET /city-links — the player's active links.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	res, err := h.svc.Links(r.Context(), playerID)
	if err != nil {
		writeErr(w, err, "list failed")
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"links": res})
}

// Remove handles POST /city-links/{id}/remove — dissolve a link.
func (h *Handler) Remove(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	if err := h.svc.Remove(r.Context(), playerID, chi.URLParam(r, "id")); err != nil {
		writeErr(w, err, "remove failed")
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"ok": true})
}
