package quest

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/pkg/httputil"
)

// Handler exposes quest and streak HTTP endpoints.
type Handler struct {
	quests  *QuestService
	streaks *StreakService
	players player.Repository
}

// NewHandler creates a new quest handler.
func NewHandler(quests *QuestService, streaks *StreakService, players player.Repository) *Handler {
	return &Handler{quests: quests, streaks: streaks, players: players}
}

func (h *Handler) playerResourceSnapshot(r *http.Request, playerID string) map[string]any {
	p, err := h.players.GetByID(r.Context(), playerID)
	if err != nil || p == nil {
		return nil
	}
	return map[string]any{
		"energy":        p.Energy,
		"energy_max":    p.EnergyMax(),
		"materials":     p.Materials,
		"materials_max": p.MaterialsMax(),
	}
}

func streakSummary(s *Streak) map[string]any {
	if s == nil {
		s = &Streak{Days: 0}
	}
	threshold, reward, daysUntil, ok := NextBonusInfo(s.Days)
	nextBonus := reward.Energy
	if reward.Materials > 0 {
		nextBonus = reward.Materials
	}
	if !ok {
		nextBonus = 0
		threshold = 0
	}
	return map[string]any{
		"days":           s.Days,
		"next_bonus":     nextBonus,
		"next_bonus_day": threshold,
		"days_until":     daysUntil,
	}
}

// GetQuests handles GET /quests — list today's daily quests (generates if needed).
func (h *Handler) GetQuests(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())

	quests, err := h.quests.Generate3Daily(r.Context(), playerID)
	if err != nil {
		httputil.Error(w, httputil.NewInternal("failed to get quests"))
		return
	}
	if quests == nil {
		quests = []DailyQuest{}
	}
	streak, err := h.streaks.Get(r.Context(), playerID)
	if err != nil {
		httputil.Error(w, httputil.NewInternal("failed to get streak"))
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]any{
		"quests": quests,
		"total":  len(quests),
		"streak": streakSummary(streak),
	})
}

// ClaimQuest handles POST /quests/{id}/claim — claim a completed quest reward.
func (h *Handler) ClaimQuest(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	questID := chi.URLParam(r, "id")

	q, err := h.quests.Claim(r.Context(), questID, playerID)
	if err != nil {
		if appErr, ok := err.(*httputil.AppError); ok {
			httputil.Error(w, appErr)
			return
		}
		httputil.Error(w, httputil.NewInternal("failed to claim quest"))
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]any{
		"quest":     q,
		"resources": h.playerResourceSnapshot(r, playerID),
	})
}

// CheckIn handles POST /streaks/checkin — daily streak check-in.
func (h *Handler) CheckIn(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())

	streak, bonus, err := h.streaks.CheckIn(r.Context(), playerID)
	if err != nil {
		httputil.Error(w, httputil.NewInternal("failed to check in"))
		return
	}

	resp := map[string]any{
		"streak":    streakSummary(streak),
		"resources": h.playerResourceSnapshot(r, playerID),
	}
	if bonus != nil {
		resp["bonus"] = bonus
	}
	httputil.JSON(w, http.StatusOK, resp)
}
