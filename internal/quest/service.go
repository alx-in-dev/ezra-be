package quest

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/pkg/httputil"
)

// QuestService implements daily quest business logic.
type QuestService struct {
	quests    QuestRepository
	resources *player.ResourceService
}

// NewQuestService creates a new quest service.
func NewQuestService(quests QuestRepository, resources *player.ResourceService) *QuestService {
	return &QuestService{quests: quests, resources: resources}
}

// Generate3Daily creates 3 random daily quests for the player if not already generated today.
func (s *QuestService) Generate3Daily(ctx context.Context, playerID string) ([]DailyQuest, error) {
	today := time.Now().Truncate(24 * time.Hour)

	existing, err := s.quests.GetByPlayerAndDate(ctx, playerID, today)
	if err != nil {
		return nil, fmt.Errorf("get existing quests: %w", err)
	}
	if len(existing) > 0 {
		return existing, nil
	}

	// Pick 3 random templates without duplicates
	templates := make([]QuestTemplate, len(QuestTemplates))
	copy(templates, QuestTemplates)
	rand.Shuffle(len(templates), func(i, j int) {
		templates[i], templates[j] = templates[j], templates[i]
	})

	count := 3
	if count > len(templates) {
		count = len(templates)
	}

	var quests []DailyQuest
	for i := 0; i < count; i++ {
		t := templates[i]
		q := &DailyQuest{
			PlayerID:    playerID,
			Type:        t.Type,
			Description: t.Description,
			Progress:    0,
			Target:      t.Target,
			Reward:      t.Reward,
			Date:        today,
			Claimed:     false,
		}
		if err := s.quests.Create(ctx, q); err != nil {
			return nil, fmt.Errorf("create quest: %w", err)
		}
		quests = append(quests, *q)
	}

	return quests, nil
}

// UpdateProgress increments progress for quests matching the event type.
func (s *QuestService) UpdateProgress(ctx context.Context, playerID, eventType string) error {
	// Ensure today's daily quests exist before matching. They are otherwise
	// generated lazily on the first /quests fetch, so progress events that fire
	// earlier — placing the starter beacon, sending a pet, winning a fight during
	// onboarding — would arrive before any quest row existed and be silently lost
	// (the player later sees 0/1 despite having done it). Generate3Daily is
	// idempotent: it returns the existing set when already created today.
	if _, err := s.Generate3Daily(ctx, playerID); err != nil {
		return fmt.Errorf("ensure daily quests: %w", err)
	}

	today := time.Now().Truncate(24 * time.Hour)
	quests, err := s.quests.GetByPlayerAndDate(ctx, playerID, today)
	if err != nil {
		return fmt.Errorf("get quests: %w", err)
	}

	for _, q := range quests {
		if q.Type == eventType && !q.Claimed && q.Progress < q.Target {
			newProgress := q.Progress + 1
			if newProgress > q.Target {
				newProgress = q.Target
			}
			if err := s.quests.UpdateProgress(ctx, q.ID, newProgress); err != nil {
				return fmt.Errorf("update progress: %w", err)
			}
		}
	}
	return nil
}

// Claim marks a quest as claimed and awards the reward.
func (s *QuestService) Claim(ctx context.Context, questID, playerID string) (*DailyQuest, error) {
	q, err := s.quests.GetByID(ctx, questID)
	if err != nil {
		return nil, httputil.NewNotFound("not_found", "quest not found")
	}
	if q.PlayerID != playerID {
		return nil, httputil.NewForbidden("not_owner", "not your quest")
	}
	if q.Claimed {
		return nil, httputil.NewBadRequest("already_claimed", "quest already claimed")
	}
	if q.Progress < q.Target {
		return nil, httputil.NewBadRequest("not_complete", "quest not completed")
	}

	// Award reward via ResourceService
	if _, err := s.resources.Add(ctx, playerID, q.Reward.Energy, q.Reward.Materials); err != nil {
		return nil, fmt.Errorf("add reward: %w", err)
	}

	if err := s.quests.Claim(ctx, questID); err != nil {
		return nil, fmt.Errorf("claim quest: %w", err)
	}

	q.Claimed = true
	return q, nil
}

// StreakService implements daily check-in streak logic.
type StreakService struct {
	streaks   StreakRepository
	resources *player.ResourceService
	crystals  interface {
		IncrementCrystals(ctx context.Context, playerID string, delta int) error
	}
}

// NewStreakService creates a new streak service.
func NewStreakService(streaks StreakRepository, resources *player.ResourceService) *StreakService {
	return &StreakService{streaks: streaks, resources: resources}
}

// SetCrystalsGranter wires the 💎 part of streak milestone bonuses
// (7/14/30 days). Optional; nil in tests.
func (s *StreakService) SetCrystalsGranter(g interface {
	IncrementCrystals(ctx context.Context, playerID string, delta int) error
}) {
	s.crystals = g
}

func (s *StreakService) Get(ctx context.Context, playerID string) (*Streak, error) {
	return s.streaks.Get(ctx, playerID)
}

func NextBonusInfo(days int) (threshold int, reward Reward, daysUntil int, ok bool) {
	ordered := []int{3, 7, 14, 30}
	for _, milestone := range ordered {
		if days < milestone {
			return milestone, StreakBonuses[milestone], milestone - days, true
		}
	}
	reward, ok = StreakBonuses[30]
	return 30, reward, 0, ok
}

// CheckIn processes a daily check-in for the player.
// If last_checkin = yesterday -> days++, if today -> skip, else reset to 1.
// Returns the updated streak and any bonus reward.
func (s *StreakService) CheckIn(ctx context.Context, playerID string) (*Streak, *Reward, error) {
	streak, err := s.streaks.Get(ctx, playerID)
	if err != nil {
		return nil, nil, fmt.Errorf("get streak: %w", err)
	}

	today := time.Now().Truncate(24 * time.Hour)

	if streak.LastCheckin != nil {
		lastDay := streak.LastCheckin.Truncate(24 * time.Hour)
		if lastDay.Equal(today) {
			// Already checked in today
			return streak, nil, nil
		}
		yesterday := today.AddDate(0, 0, -1)
		if lastDay.Equal(yesterday) {
			streak.Days++
		} else {
			// Streak broken — reset
			streak.Days = 1
		}
	} else {
		streak.Days = 1
	}

	streak.LastCheckin = &today

	if err := s.streaks.Upsert(ctx, streak); err != nil {
		return nil, nil, fmt.Errorf("upsert streak: %w", err)
	}

	// Check bonus threshold
	var bonus *Reward
	if b, ok := StreakBonuses[streak.Days]; ok {
		bonus = &b
		if _, err := s.resources.Add(ctx, playerID, b.Energy, b.Materials); err != nil {
			return nil, nil, fmt.Errorf("add streak bonus: %w", err)
		}
		if b.Crystals > 0 && s.crystals != nil {
			if err := s.crystals.IncrementCrystals(ctx, playerID, b.Crystals); err != nil {
				return nil, nil, fmt.Errorf("add streak crystals: %w", err)
			}
		}
	}

	// TODO(T-160): send push notification when streak is about to expire (T-134)

	return streak, bonus, nil
}
