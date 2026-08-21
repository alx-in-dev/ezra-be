package item

import "context"

// Service holds inventory business logic.
type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Grant adds delta of an item to a player and returns the new quantity.
func (s *Service) Grant(ctx context.Context, playerID, itemType, variant string, delta int) (int, error) {
	return s.repo.Add(ctx, playerID, itemType, variant, delta)
}

// Pity for the Spire blueprint fragment rare drop (audit C2). The base per-rift
// tier chance applies until softPityBlueprint consecutive misses, then the
// effective chance ramps linearly to a guaranteed drop at hardPityBlueprint.
// Kills the "100 rifts, zero fragments" tail that stalls the megastructure gate.
const (
	softPityBlueprint = 10
	hardPityBlueprint = 30
)

// effectivePityChance ramps base toward 1.0 between the soft and hard pity
// thresholds. streak counts consecutive misses INCLUDING the current attempt,
// so a guaranteed drop lands exactly on the hardPityBlueprint-th dry roll.
func effectivePityChance(streak int, base float64) float64 {
	if streak >= hardPityBlueprint {
		return 1.0
	}
	if streak <= softPityBlueprint {
		return base
	}
	t := float64(streak-softPityBlueprint) / float64(hardPityBlueprint-softPityBlueprint)
	return base + (1.0-base)*t
}

// RollBlueprintFragment is a pity-aware rare-drop roll for the Spire blueprint
// fragment. It bumps a hidden per-player dry-streak counter, computes the
// effective chance for that streak, and compares against roll ∈ [0,1). On a
// drop it grants one fragment, resets the streak, and returns true; otherwise
// the streak persists for next time. roll is injected so the caller owns the
// RNG (and tests stay deterministic).
func (s *Service) RollBlueprintFragment(ctx context.Context, playerID string, base, roll float64) (bool, error) {
	streak, err := s.repo.Add(ctx, playerID, TypePityBlueprint, "", 1)
	if err != nil {
		return false, err
	}
	if roll >= effectivePityChance(streak, base) {
		return false, nil // still dry — streak stays bumped
	}
	if _, err := s.repo.Add(ctx, playerID, TypeBlueprintFragment, "", 1); err != nil {
		return false, err
	}
	// Reset the streak. If this fails the fragment is already granted; the next
	// roll just starts from an inflated streak (benign — slightly more generous).
	if _, err := s.repo.Add(ctx, playerID, TypePityBlueprint, "", -streak); err != nil {
		return true, nil
	}
	return true, nil
}

// Consume atomically spends qty of a fungible item, returning false if the
// player is short. Used as a resource sink (e.g. the Ionized Charge repellent
// spends hack_key, T-881).
func (s *Service) Consume(ctx context.Context, playerID, itemType string, qty int) (bool, error) {
	return s.repo.Consume(ctx, playerID, itemType, qty)
}

// ListByPlayer returns the player's inventory.
func (s *Service) ListByPlayer(ctx context.Context, playerID string) ([]Item, error) {
	items, err := s.repo.ListByPlayer(ctx, playerID)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []Item{}
	}
	return items, nil
}
