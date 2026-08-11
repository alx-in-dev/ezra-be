package achievement

import (
	"context"
	"log/slog"
)

// CrystalGranter awards the one-time crystal reward when an achievement unlocks.
// Optional (nil-safe) so tests can skip the economy side.
type CrystalGranter interface {
	IncrementCrystals(ctx context.Context, playerID string, delta int) error
}

// Service evaluates achievements: it derives live progress, persists first-time
// unlocks, grants their rewards, and reports which unlocked just now (for a
// client toast).
type Service struct {
	repo     Repository
	crystals CrystalGranter
}

func NewService(repo Repository, crystals CrystalGranter) *Service {
	return &Service{repo: repo, crystals: crystals}
}

// Result is the achievement payload: every achievement's status plus the ids
// that unlocked on THIS evaluation (so the client can toast them once).
type Result struct {
	Achievements  []Status `json:"achievements"`
	NewlyUnlocked []Status `json:"newly_unlocked"`
	RewardCrystals int     `json:"reward_crystals"` // total crystals granted this call
}

// Evaluate computes progress, unlocks newly-completed achievements (persist +
// reward once), and returns the full list plus what just unlocked.
func (s *Service) Evaluate(ctx context.Context, playerID string) (*Result, error) {
	metrics, err := s.repo.Metrics(ctx, playerID)
	if err != nil {
		return nil, err
	}
	unlocked, err := s.repo.UnlockedIDs(ctx, playerID)
	if err != nil {
		return nil, err
	}

	res := &Result{
		Achievements:  make([]Status, 0, len(Catalog)),
		NewlyUnlocked: []Status{},
	}
	for _, def := range Catalog {
		progress := metrics.value(def.Metric)
		done := progress >= def.Threshold
		already := unlocked[def.ID]

		st := Status{
			ID:             def.ID,
			Title:          def.Title,
			Description:    def.Description,
			Threshold:      def.Threshold,
			Progress:       progress,
			RewardCrystals: def.RewardCrystals,
			Unlocked:       done || already,
		}

		// First time a threshold is met: persist, grant reward, flag for toast.
		if done && !already {
			if err := s.repo.MarkUnlocked(ctx, playerID, def.ID); err != nil {
				return nil, err
			}
			if s.crystals != nil && def.RewardCrystals > 0 {
				if err := s.crystals.IncrementCrystals(ctx, playerID, def.RewardCrystals); err != nil {
					// Reward is a side-effect; the unlock already persisted, so
					// don't fail the whole evaluation — just log and move on.
					slog.Error("achievement reward grant failed",
						"player_id", playerID, "achievement", def.ID, "error", err)
				} else {
					res.RewardCrystals += def.RewardCrystals
				}
			}
			res.NewlyUnlocked = append(res.NewlyUnlocked, st)
		}
		res.Achievements = append(res.Achievements, st)
	}
	return res, nil
}
