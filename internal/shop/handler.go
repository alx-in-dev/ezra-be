package shop

import (
	"net/http"

	"github.com/ezra-game/server/pkg/httputil"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// GetCatalog returns all shop items.
func (h *Handler) GetCatalog(w http.ResponseWriter, r *http.Request) {
	httputil.JSON(w, http.StatusOK, map[string]any{
		"items": h.svc.GetCatalog(),
	})
}

type BuyRequest struct {
	ItemID string `json:"item_id"`
}

// Buy purchases a shop item.
func (h *Handler) Buy(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())

	var req BuyRequest
	if err := httputil.Decode(r, &req); err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", err.Error()))
		return
	}
	if req.ItemID == "" {
		httputil.Error(w, httputil.NewBadRequest("missing_item_id", "item_id is required"))
		return
	}

	purchase, err := h.svc.BuyItem(r.Context(), playerID, req.ItemID)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("purchase failed"))
		return
	}

	httputil.JSON(w, http.StatusOK, purchase)
}

type AddCrystalsRequest struct {
	Amount   int    `json:"amount"`
	Receipt  string `json:"receipt"`
	Platform string `json:"platform"` // ios, android
}

// AddCrystals validates IAP receipt and adds crystals.
func (h *Handler) AddCrystals(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())

	var req AddCrystalsRequest
	if err := httputil.Decode(r, &req); err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", err.Error()))
		return
	}
	if req.Amount <= 0 {
		httputil.Error(w, httputil.NewBadRequest("invalid_amount", "amount must be positive"))
		return
	}

	valid, err := ValidateReceipt(req.Platform, req.Receipt)
	if err != nil || !valid {
		httputil.Error(w, httputil.NewBadRequest("invalid_receipt", "receipt validation failed"))
		return
	}

	if err := h.svc.AddCrystals(r.Context(), playerID, req.Amount); err != nil {
		httputil.Error(w, httputil.NewInternal("failed to add crystals"))
		return
	}

	crystals, _ := h.svc.repo.GetCrystals(r.Context(), playerID)
	httputil.JSON(w, http.StatusOK, map[string]any{
		"crystals": crystals,
	})
}

// GetSubscription returns the player's current subscription.
func (h *Handler) GetSubscription(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())

	sub, err := h.svc.CheckSubscription(r.Context(), playerID)
	if err != nil {
		httputil.Error(w, httputil.NewInternal("failed to check subscription"))
		return
	}
	if sub == nil {
		httputil.JSON(w, http.StatusOK, map[string]any{"subscription": nil})
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]any{
		"subscription": sub,
	})
}

type ActivateSubscriptionRequest struct {
	Receipt  string `json:"receipt"`
	Platform string `json:"platform"`
}

// ActivateSubscription starts or renews an energy_pack subscription.
func (h *Handler) ActivateSubscription(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())

	var req ActivateSubscriptionRequest
	if err := httputil.Decode(r, &req); err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", err.Error()))
		return
	}
	if req.Receipt == "" {
		httputil.Error(w, httputil.NewBadRequest("missing_receipt", "receipt is required"))
		return
	}

	sub, err := h.svc.ActivateSubscription(r.Context(), playerID, "energy_pack", req.Receipt, req.Platform)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to activate subscription"))
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]any{
		"subscription": sub,
	})
}
