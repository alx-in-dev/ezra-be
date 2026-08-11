package shop

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ezra-game/server/pkg/httputil"
)

// PlayerEffects applies expansion purchases to the player record.
type PlayerEffects interface {
	IncrementArmyLimit(ctx context.Context, playerID string, delta int) error
	IncrementEnergyStorageBonus(ctx context.Context, playerID string, delta int) error
	IncrementPetSlots(ctx context.Context, playerID string, delta int) error
}

// ArmyEffects applies acceleration purchases to the player's units.
type ArmyEffects interface {
	HealAll(ctx context.Context, playerID string) error
}

// PetEffects applies acceleration purchases to the player's pet.
type PetEffects interface {
	CompleteActiveTask(ctx context.Context, playerID string) error
}

type Service struct {
	repo    Repository
	players PlayerEffects
	army    ArmyEffects
	pets    PetEffects
}

func NewService(repo Repository, players PlayerEffects, army ArmyEffects, pets PetEffects) *Service {
	return &Service{repo: repo, players: players, army: army, pets: pets}
}

// GetCatalog returns all available shop items.
func (s *Service) GetCatalog() []ShopItem {
	return Catalog
}

// BuyItem validates crystals, deducts cost, records purchase, and applies item effect.
func (s *Service) BuyItem(ctx context.Context, playerID, itemID string) (*Purchase, error) {
	// Find item in catalog
	var item *ShopItem
	for i := range Catalog {
		if Catalog[i].ID == itemID {
			item = &Catalog[i]
			break
		}
	}
	if item == nil {
		return nil, httputil.NewNotFound("item_not_found", "shop item not found")
	}

	// Check & spend crystals (atomic in DB)
	if err := s.repo.SpendCrystals(ctx, playerID, item.Cost); err != nil {
		return nil, httputil.NewBadRequest("insufficient_crystals", "not enough crystals")
	}

	// Record purchase
	purchase := &Purchase{
		PlayerID:      playerID,
		ItemType:      item.Category,
		ItemID:        item.ID,
		CrystalsSpent: item.Cost,
	}
	if err := s.repo.CreatePurchase(ctx, purchase); err != nil {
		// Refund crystals on failure
		_ = s.repo.AddCrystals(ctx, playerID, item.Cost)
		return nil, fmt.Errorf("create purchase: %w", err)
	}

	// Apply effect; refund on failure so the player is not charged for nothing.
	// The purchase row stays as an audit trail of the refunded attempt.
	if err := s.applyEffect(ctx, playerID, item); err != nil {
		_ = s.repo.AddCrystals(ctx, playerID, item.Cost)
		slog.Error("shop: effect failed, crystals refunded",
			"player", playerID, "item", item.ID, "error", err)
		return nil, fmt.Errorf("apply effect %s: %w", item.ID, err)
	}
	slog.Info("shop: purchase applied", "player", playerID, "item", item.ID)

	return purchase, nil
}

// applyEffect applies the purchased item's effect.
func (s *Service) applyEffect(ctx context.Context, playerID string, item *ShopItem) error {
	switch item.ID {
	case "army_limit_10":
		return s.players.IncrementArmyLimit(ctx, playerID, 10)
	case "storage_500":
		return s.players.IncrementEnergyStorageBonus(ctx, playerID, 500)
	case "storage_2000":
		return s.players.IncrementEnergyStorageBonus(ctx, playerID, 2000)
	case "pet_slot":
		return s.players.IncrementPetSlots(ctx, playerID, 1)
	case "army_heal":
		return s.army.HealAll(ctx, playerID)
	case "pet_speedup":
		return s.pets.CompleteActiveTask(ctx, playerID)
	case "instant_build_1h", "instant_build_long":
		// Towers activate instantly in MVP — there is no build queue to
		// complete yet (BuildTime is canon for v1.0). Purchase recorded.
		slog.Info("shop: instant_build is a no-op until build queue lands (v1.0)",
			"player", playerID, "item", item.ID)
		return nil
	case "beacon_skin", "pet_skin", "dome_color":
		// Cosmetic ownership IS the purchase record; the client reads the
		// purchase history. No extra state to mutate.
		return nil
	default:
		slog.Warn("shop: unknown item effect", "item", item.ID)
		return nil
	}
}

// AddCrystals adds crystals to a player's balance (for IAP receipts).
func (s *Service) AddCrystals(ctx context.Context, playerID string, amount int) error {
	if amount <= 0 {
		return httputil.NewBadRequest("invalid_amount", "amount must be positive")
	}
	return s.repo.AddCrystals(ctx, playerID, amount)
}

// SpendCrystals deducts crystals with validation.
func (s *Service) SpendCrystals(ctx context.Context, playerID string, amount int) error {
	if amount <= 0 {
		return httputil.NewBadRequest("invalid_amount", "amount must be positive")
	}
	return s.repo.SpendCrystals(ctx, playerID, amount)
}

// ActivateSubscription creates or renews a 30-day subscription.
func (s *Service) ActivateSubscription(ctx context.Context, playerID, subType, receipt, platform string) (*Subscription, error) {
	valid, err := ValidateReceipt(platform, receipt)
	if err != nil || !valid {
		return nil, httputil.NewBadRequest("invalid_receipt", "receipt validation failed")
	}

	now := time.Now()
	sub := &Subscription{
		PlayerID:  playerID,
		Type:      subType,
		Active:    true,
		StartedAt: now,
		ExpiresAt: now.Add(30 * 24 * time.Hour),
		Receipt:   receipt,
	}

	if err := s.repo.CreateSubscription(ctx, sub); err != nil {
		return nil, fmt.Errorf("create subscription: %w", err)
	}
	return sub, nil
}

// CheckSubscription returns the active subscription or nil.
func (s *Service) CheckSubscription(ctx context.Context, playerID string) (*Subscription, error) {
	sub, err := s.repo.GetSubscription(ctx, playerID)
	if err != nil {
		return nil, err
	}
	if sub == nil {
		return nil, nil
	}
	// Check expiry
	if sub.Active && sub.ExpiresAt.Before(time.Now()) {
		sub.Active = false
		_ = s.repo.UpdateSubscription(ctx, sub)
	}
	return sub, nil
}

// ExpireSubscriptions finds and deactivates expired subscriptions (for asynq worker).
func (s *Service) ExpireSubscriptions(ctx context.Context) error {
	count, err := s.repo.ExpireSubscriptions(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		slog.Info("expired subscriptions", "count", count)
	}
	return nil
}
