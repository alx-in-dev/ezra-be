package item

import (
	"net/http"

	"github.com/ezra-game/server/pkg/httputil"
)

// Handler exposes inventory HTTP endpoints.
type Handler struct {
	service *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{service: svc}
}

// List handles GET /items — the player's inventory (keys, resonance signatures).
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	items, err := h.service.ListByPlayer(r.Context(), playerID)
	if err != nil {
		httputil.Error(w, httputil.NewInternal("failed to list items"))
		return
	}
	// Also surface the full resonance spectrum set so the client can show
	// progress toward completion (owned vs missing) without hardcoding it.
	spectra := make([]map[string]string, 0, len(ResonanceSpectra))
	for _, sp := range ResonanceSpectra {
		spectra = append(spectra, map[string]string{"key": sp, "name": SpectrumRu(sp)})
	}
	httputil.JSON(w, http.StatusOK, map[string]any{
		"items":              items,
		"resonance_spectra":  spectra,
	})
}
