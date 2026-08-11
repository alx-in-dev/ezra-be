package push

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	fb "github.com/ezra-game/server/internal/platform"
	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/pkg/geo"
	"github.com/redis/go-redis/v9"
)

// Messenger delivers a notification to a device token (FCM in production).
// Implementations signal a stale token with platform.ErrFCMTokenNotRegistered.
type Messenger interface {
	SendToToken(ctx context.Context, token, title, body string, data map[string]string) error
}

// Service handles push token registration and message sending.
type Service struct {
	tokens  Repository
	rdb     *redis.Client
	players player.Repository
	fcm     Messenger // nil → log-only mode (local dev without credentials)
}

// NewService creates a new push service. Pass a nil fcm to run in log-only
// mode (notifications are logged, not delivered).
func NewService(tokens Repository, rdb *redis.Client, players player.Repository, fcm Messenger) *Service {
	return &Service{tokens: tokens, rdb: rdb, players: players, fcm: fcm}
}

// RegisterToken stores or updates a player's FCM push token.
func (s *Service) RegisterToken(ctx context.Context, playerID, fcmToken, platform string) error {
	if platform == "" {
		platform = "android"
	}
	token := &PushToken{
		PlayerID: playerID,
		FCMToken: fcmToken,
		Platform: platform,
	}
	return s.tokens.Upsert(ctx, token)
}

// Send delivers a push notification via FCM (or logs it when no Messenger
// is configured). Enforces a daily limit of 3 notifications per player via
// Redis. A token FCM reports as unregistered is purged and not retried.
func (s *Service) Send(ctx context.Context, msg PushMessage) error {
	// Check daily limit (3/day per player)
	key := fmt.Sprintf("push:daily:%s:%s", msg.PlayerID, time.Now().Format("2006-01-02"))
	count, err := s.rdb.Incr(ctx, key).Result()
	if err != nil {
		slog.Error("push daily limit check failed", "error", err)
		// Don't block on Redis errors — proceed with send
	} else {
		if count == 1 {
			// Set TTL to end of day (next midnight)
			now := time.Now()
			midnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
			s.rdb.ExpireAt(ctx, key, midnight)
		}
		if count > 3 {
			slog.Info("push daily limit reached", "player_id", msg.PlayerID, "count", count)
			return nil
		}
	}

	// Look up FCM token
	token, err := s.tokens.GetByPlayerID(ctx, msg.PlayerID)
	if err != nil {
		slog.Warn("push token not found", "player_id", msg.PlayerID)
		return nil // No token — silently skip
	}

	if s.fcm == nil {
		// Local dev without Firebase credentials: log instead of sending.
		slog.Info("push notification (log-only)",
			"player_id", msg.PlayerID,
			"type", msg.Type,
			"title", msg.Title,
			"body", msg.Body,
			"platform", token.Platform,
		)
		return nil
	}

	data := map[string]string{"type": msg.Type}
	for k, v := range msg.Data {
		data[k] = v
	}
	if err := s.fcm.SendToToken(ctx, token.FCMToken, msg.Title, msg.Body, data); err != nil {
		if errors.Is(err, fb.ErrFCMTokenNotRegistered) {
			slog.Info("push token stale, purging", "player_id", msg.PlayerID)
			if delErr := s.tokens.Delete(ctx, msg.PlayerID); delErr != nil {
				slog.Error("purge stale push token failed", "player_id", msg.PlayerID, "error", delErr)
			}
			return nil // not retryable
		}
		// Direct callers discard this error (`_ = push.Notify...`), so log
		// here; the asynq worker path also returns it for retry.
		slog.Error("push send failed",
			"player_id", msg.PlayerID, "type", msg.Type, "error", err)
		return fmt.Errorf("send push to %s: %w", msg.PlayerID, err)
	}

	slog.Info("push notification sent",
		"player_id", msg.PlayerID, "type", msg.Type, "platform", token.Platform)
	return nil
}

func (s *Service) NotifyTowerPlaced(ctx context.Context, playerID string) error {
	return s.Send(ctx, PushMessage{
		PlayerID: playerID,
		Type:     "tower_ready",
		Title:    "Маяк активен",
		Body:     "Купол поднят, safe zone начала работу.",
		Data:     map[string]string{"event": "tower_ready"},
	})
}

func (s *Service) NotifyPetReturned(ctx context.Context, playerID, taskType string) error {
	body := "Питомец вернулся с миссии и ждёт вашего решения."
	if taskType == "scout" {
		body = "Разведка завершена: питомец вернулся с данными по safe zone."
	}
	return s.Send(ctx, PushMessage{
		PlayerID: playerID,
		Type:     "pet_returned",
		Title:    "Питомец вернулся",
		Body:     body,
		Data:     map[string]string{"event": "pet_returned", "task_type": taskType},
	})
}

// NotifySquadReturned pushes the player that a timed squad mission finished
// and the reward is waiting.
func (s *Service) NotifySquadReturned(ctx context.Context, playerID, missionType string) error {
	body := "Отряд вернулся с задания с трофеями."
	if missionType == "patrol" {
		body = "Патруль завершён: зона зачищена, отряд вернулся."
	} else if missionType == "capture" {
		body = "Захват завершён: отряд вернулся с ресурсами."
	}
	return s.Send(ctx, PushMessage{
		PlayerID: playerID,
		Type:     "squad_returned",
		Title:    "Отряд вернулся",
		Body:     body,
		Data:     map[string]string{"event": "squad_returned", "mission_type": missionType},
	})
}

// NotifyTowerCaptured pushes the previous owner that their tower has been
// taken (lockpick capture). Called after ownership transfer succeeds.
func (s *Service) NotifyTowerCaptured(ctx context.Context, defenderID, attackerName string) error {
	body := "Ваш маяк был взломан и захвачен."
	if attackerName != "" {
		body = fmt.Sprintf("Игрок %s взломал ваш маяк и забрал контроль над куполом.", attackerName)
	}
	return s.Send(ctx, PushMessage{
		PlayerID: defenderID,
		Type:     "tower_captured",
		Title:    "Маяк потерян",
		Body:     body,
		Data:     map[string]string{"event": "tower_captured"},
	})
}

// NotifyTowerUnderAttack pushes the defender that an attacker has begun a
// force-capture flow on their tower (pre-battle warning).
func (s *Service) NotifyTowerUnderAttack(ctx context.Context, defenderID, attackerName string) error {
	body := "Кто-то атакует ваш маяк."
	if attackerName != "" {
		body = fmt.Sprintf("Игрок %s начал штурм вашего маяка — отряды на позиции.", attackerName)
	}
	return s.Send(ctx, PushMessage{
		PlayerID: defenderID,
		Type:     "tower_under_attack",
		Title:    "Маяк под атакой",
		Body:     body,
		Data:     map[string]string{"event": "tower_under_attack"},
	})
}

func (s *Service) NotifyNearbyRift(ctx context.Context, lat, lng float64, riftType string) error {
	if s.players == nil {
		return nil
	}
	players, err := s.players.GetAll(ctx)
	if err != nil {
		return err
	}

	for _, p := range players {
		if p.Position.Lat == 0 && p.Position.Lng == 0 {
			continue
		}
		if geo.Haversine(p.Position.Lat, p.Position.Lng, lat, lng) > 1500 {
			continue
		}
		if err := s.Send(ctx, PushMessage{
			PlayerID: p.ID,
			Type:     "rift_nearby",
			Title:    "Рядом разлом",
			Body:     fmt.Sprintf("Поблизости открылся %s разлом. Можно собирать отряд.", riftType),
			Data:     map[string]string{"event": "rift_nearby", "rift_type": riftType},
		}); err != nil {
			slog.Warn("notify nearby rift failed", "player_id", p.ID, "error", err)
		}
	}
	return nil
}
