package battle

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisRiftLimiter is the production RiftDailyLimiter: a per-player, per-tier,
// per-day counter in Redis with a TTL to the next local midnight. Mirrors the
// push daily-limit pattern (push.Service.Send). Only successful closures are
// recorded (the loot moment), so abandoned or lost battles don't burn the cap.
type RedisRiftLimiter struct {
	rdb *redis.Client
}

// NewRedisRiftLimiter builds the Redis-backed daily rift-closure limiter.
func NewRedisRiftLimiter(rdb *redis.Client) *RedisRiftLimiter {
	return &RedisRiftLimiter{rdb: rdb}
}

func (l *RedisRiftLimiter) key(playerID, riftType string) string {
	return fmt.Sprintf("rift:daily:%s:%s:%s", playerID, riftType, time.Now().Format("2006-01-02"))
}

// ClosuresToday returns how many rifts of riftType the player has closed today.
// A missing key (no closures yet) reads as 0, not an error.
func (l *RedisRiftLimiter) ClosuresToday(ctx context.Context, playerID, riftType string) (int, error) {
	n, err := l.rdb.Get(ctx, l.key(playerID, riftType)).Int()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return n, nil
}

// RecordClosure increments today's closure counter for the tier, setting a
// TTL to the next local midnight on the first increment of the day.
func (l *RedisRiftLimiter) RecordClosure(ctx context.Context, playerID, riftType string) error {
	key := l.key(playerID, riftType)
	count, err := l.rdb.Incr(ctx, key).Result()
	if err != nil {
		return err
	}
	if count == 1 {
		now := time.Now()
		midnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
		l.rdb.ExpireAt(ctx, key, midnight)
	}
	return nil
}
