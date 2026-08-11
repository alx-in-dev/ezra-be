package auth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

const sessionTTL = 7 * 24 * time.Hour // 7 days
const sessionPrefix = "session:"

// Service handles authentication: password hashing and Redis sessions.
type Service struct {
	redis    *redis.Client
	firebase FirebaseVerifier
}

// FirebaseVerifier verifies a Firebase ID token and returns its UID.
type FirebaseVerifier interface {
	VerifyIDToken(ctx context.Context, idToken string) (string, error)
}

// NewService creates a new auth Service.
func NewService(rdb *redis.Client, firebase FirebaseVerifier) *Service {
	return &Service{redis: rdb, firebase: firebase}
}

// HashPassword returns a bcrypt hash of the password.
func (s *Service) HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

// CheckPassword compares a bcrypt hash with a plain-text password.
func (s *Service) CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// CreateSession stores a new session in Redis and returns the session token.
func (s *Service) CreateSession(ctx context.Context, playerID string) (string, error) {
	token := uuid.New().String()
	key := sessionPrefix + token
	if err := s.redis.Set(ctx, key, playerID, sessionTTL).Err(); err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	return token, nil
}

// GetPlayerIDByToken looks up a session in Redis. Returns empty string if not found.
func (s *Service) GetPlayerIDByToken(ctx context.Context, token string) (string, error) {
	key := sessionPrefix + token
	playerID, err := s.redis.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get session: %w", err)
	}
	return playerID, nil
}

// DeleteSession removes a session from Redis.
func (s *Service) DeleteSession(ctx context.Context, token string) error {
	key := sessionPrefix + token
	return s.redis.Del(ctx, key).Err()
}

// VerifyFirebaseToken validates a Firebase ID token and returns its UID.
func (s *Service) VerifyFirebaseToken(ctx context.Context, idToken string) (string, error) {
	idToken = strings.TrimSpace(idToken)
	if idToken == "" {
		return "", fmt.Errorf("firebase token is required")
	}
	if s.firebase == nil {
		return "", fmt.Errorf("firebase auth is not configured")
	}

	uid, err := s.firebase.VerifyIDToken(ctx, idToken)
	if err != nil {
		return "", fmt.Errorf("verify firebase token: %w", err)
	}
	if strings.TrimSpace(uid) == "" {
		return "", fmt.Errorf("firebase token resolved to an empty uid")
	}
	return uid, nil
}
