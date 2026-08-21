package nest

import (
	"net/http"
	"strconv"

	"github.com/ezra-game/server/pkg/httputil"
)

// Handler exposes nest HTTP endpoints (N3, ADR-N3). All are behind auth
// middleware; the owner is taken from the request context, never the body.
type Handler struct {
	service *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{service: svc}
}

type nestRequest struct {
	CellID string `json:"cell_id"`
	// FirstOnly (onboarding auto-open): only open a FREE first nest. If the
	// player has owned one before, this is a no-op — never silently charge for a
	// rebuild (a faction flip must not bill crystals without an explicit choice).
	FirstOnly bool `json:"first_only"`
	// NestID (coop repair): repair this ally nest instead of the caller's own.
	NestID string `json:"nest_id"`
}

// Get handles GET /nest — the caller's live nest, or 404 if they have none.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	n, err := h.service.GetForOwner(r.Context(), playerID)
	if err != nil {
		httputil.Error(w, httputil.NewInternal("failed to load nest"))
		return
	}
	if n == nil {
		httputil.Error(w, httputil.NewNotFound("no_nest", "у вас нет гнезда"))
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"nest": n})
}

// Create handles POST /nest — (re)build a nest. The service picks the free
// first-open vs the paid rebuild path from ownership history.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	var req nestRequest
	if err := httputil.Decode(r, &req); err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", err.Error()))
		return
	}
	// First nest is free (onboarding also calls OpenFirstNest directly); a
	// player who has owned one before rebuilds through the paid path.
	ever, err := h.service.repo.HasEverOwned(r.Context(), playerID)
	if err != nil {
		httputil.Error(w, httputil.NewInternal("failed to open nest"))
		return
	}
	if ever && req.FirstOnly {
		// Onboarding auto-open for someone who has owned a nest before (e.g. a
		// human→symbiont flip): do nothing rather than route into the paid
		// rebuild and bill crystals without consent. Rebuild is an explicit
		// button elsewhere.
		httputil.JSON(w, http.StatusOK, map[string]any{"nest": nil, "skipped": "already_owned"})
		return
	}
	var n *Nest
	if ever {
		n, err = h.service.Create(r.Context(), playerID, req.CellID)
	} else {
		n, err = h.service.OpenFirstNest(r.Context(), playerID, req.CellID)
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	httputil.JSON(w, http.StatusCreated, map[string]any{"nest": n})
}

// Relocate handles POST /nest/relocate.
func (h *Handler) Relocate(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	var req nestRequest
	if err := httputil.Decode(r, &req); err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", err.Error()))
		return
	}
	n, err := h.service.Relocate(r.Context(), playerID, req.CellID)
	if err != nil {
		writeErr(w, err)
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"nest": n})
}

// Feed handles POST /nest/feed — the daily "tend the garden" support refill.
func (h *Handler) Feed(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	n, err := h.service.Feed(r.Context(), playerID)
	if err != nil {
		writeErr(w, err)
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"nest": n})
}

// Collect handles POST /nest/collect — move the trickle buffer into the profile.
func (h *Handler) Collect(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	amount, err := h.service.Collect(r.Context(), playerID)
	if err != nil {
		writeErr(w, err)
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"collected": amount})
}

// Repair handles POST /nest/repair — the owner (or an ally, coop) pours in to
// cancel a pending collapse. Repairs the caller's OWN nest.
func (h *Handler) Repair(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	// Optional nest_id → coop repair of an ALLY's nest (T-840): any Symbiont may
	// pour into a fellow Symbiont's nest. Without it, repair the caller's own.
	var req nestRequest
	_ = httputil.Decode(r, &req)
	if req.NestID != "" {
		repaired, err := h.service.RepairAlly(r.Context(), playerID, req.NestID)
		if err != nil {
			writeErr(w, err)
			return
		}
		httputil.JSON(w, http.StatusOK, map[string]any{"nest": repaired})
		return
	}
	n, err := h.service.GetForOwner(r.Context(), playerID)
	if err != nil {
		httputil.Error(w, httputil.NewInternal("failed to load nest"))
		return
	}
	if n == nil {
		httputil.Error(w, httputil.NewNotFound("no_nest", "у вас нет гнезда"))
		return
	}
	repaired, err := h.service.Repair(r.Context(), n.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"nest": repaired})
}

// Nearby handles GET /nests/nearby?lat=&lng=&radius_m= — live nests near a
// point, so a human can find nests to assault (mirror of hive.List).
func (h *Handler) Nearby(w http.ResponseWriter, r *http.Request) {
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
	radiusM, err := strconv.ParseFloat(r.URL.Query().Get("radius_m"), 64)
	if err != nil || radiusM <= 0 {
		radiusM = 2000
	}
	if radiusM > 5000 {
		radiusM = 5000
	}
	nests, err := h.service.GetNearby(r.Context(), lat, lng, radiusM)
	if err != nil {
		httputil.Error(w, httputil.NewInternal("failed to list nests"))
		return
	}
	if nests == nil {
		nests = []Nest{}
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"nests": nests, "total": len(nests)})
}

// writeErr maps an AppError through, else a 500.
func writeErr(w http.ResponseWriter, err error) {
	if appErr, ok := err.(*httputil.AppError); ok {
		httputil.Error(w, appErr)
		return
	}
	httputil.Error(w, httputil.NewInternal(err.Error()))
}
