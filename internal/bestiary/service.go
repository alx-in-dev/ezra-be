package bestiary

import (
	"context"
	"log/slog"
)

// CrystalGranter awards the one-time completion bonus. Optional (nil-safe).
type CrystalGranter interface {
	IncrementCrystals(ctx context.Context, playerID string, delta int) error
}

// Service builds the bestiary status and grants the completion reward once.
type Service struct {
	repo     Repository
	crystals CrystalGranter
}

func NewService(repo Repository, crystals CrystalGranter) *Service {
	return &Service{repo: repo, crystals: crystals}
}

// Discover persists encounter discoveries (spirit:/rift:). Best-effort caller
// hook from the battle flow; dedup handled by the repo. Errors are returned for
// logging by the caller but shouldn't block gameplay.
func (s *Service) Discover(ctx context.Context, playerID string, keys []string) error {
	return s.repo.MarkDiscovered(ctx, playerID, keys)
}

// Result is the bestiary payload.
type Result struct {
	Entries        []EntryStatus `json:"entries"`
	Discovered     int           `json:"discovered"`
	Total          int           `json:"total"`
	Complete       bool          `json:"complete"`
	RewardCrystals int           `json:"reward_crystals"` // granted on THIS call (0 otherwise)
}

// Status returns every catalog entry with its discovered flag and grants the
// completion bonus once when the whole set is discovered.
func (s *Service) Status(ctx context.Context, playerID string) (*Result, error) {
	discovered, err := s.repo.DiscoveredKeys(ctx, playerID)
	if err != nil {
		return nil, err
	}

	res := &Result{Entries: make([]EntryStatus, 0, len(Catalog)), Total: len(Catalog)}
	for _, e := range Catalog {
		ok := discovered[e.Key]
		if ok {
			res.Discovered++
		}
		res.Entries = append(res.Entries, EntryStatus{
			Key: e.Key, Category: string(e.Category), Name: e.Name,
			Description: e.Description, Discovered: ok,
		})
	}
	res.Complete = res.Discovered >= res.Total

	// One-time completion reward.
	if res.Complete && s.crystals != nil {
		already, err := s.repo.HasCompletionReward(ctx, playerID)
		if err != nil {
			return nil, err
		}
		if !already {
			if err := s.crystals.IncrementCrystals(ctx, playerID, CompletionRewardCrystals); err != nil {
				slog.Error("bestiary completion reward failed", "player_id", playerID, "error", err)
			} else if err := s.repo.SetCompletionReward(ctx, playerID); err != nil {
				slog.Error("bestiary completion mark failed", "player_id", playerID, "error", err)
			} else {
				res.RewardCrystals = CompletionRewardCrystals
			}
		}
	}
	return res, nil
}
