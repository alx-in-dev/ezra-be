package push

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/ezra-game/server/internal/platform"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// --- Mock Repository ---

type mockTokenRepo struct {
	mock.Mock
}

func (m *mockTokenRepo) Upsert(ctx context.Context, token *PushToken) error {
	args := m.Called(ctx, token)
	return args.Error(0)
}

func (m *mockTokenRepo) GetByPlayerID(ctx context.Context, playerID string) (*PushToken, error) {
	args := m.Called(ctx, playerID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*PushToken), args.Error(1)
}

func (m *mockTokenRepo) Delete(ctx context.Context, playerID string) error {
	args := m.Called(ctx, playerID)
	return args.Error(0)
}

// --- Mock Messenger ---

type mockMessenger struct {
	mock.Mock
}

func (m *mockMessenger) SendToToken(ctx context.Context, token, title, body string, data map[string]string) error {
	args := m.Called(ctx, token, title, body, data)
	return args.Error(0)
}

// deadRedis returns a client pointing nowhere: the daily-limit check fails
// open by design ("don't block on Redis errors"), so Send proceeds.
func deadRedis() *redis.Client {
	return redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1})
}

func testMessage() PushMessage {
	return PushMessage{
		PlayerID: "p1",
		Type:     "tower_ready",
		Title:    "t",
		Body:     "b",
		Data:     map[string]string{"event": "tower_ready"},
	}
}

func TestSend_DeliversViaMessenger(t *testing.T) {
	tokens := new(mockTokenRepo)
	fcm := new(mockMessenger)
	svc := NewService(tokens, deadRedis(), nil, fcm)

	tokens.On("GetByPlayerID", mock.Anything, "p1").
		Return(&PushToken{PlayerID: "p1", FCMToken: "tok", Platform: "android"}, nil)
	fcm.On("SendToToken", mock.Anything, "tok", "t", "b",
		map[string]string{"type": "tower_ready", "event": "tower_ready"}).Return(nil)

	err := svc.Send(context.Background(), testMessage())

	assert.NoError(t, err)
	fcm.AssertExpectations(t)
}

func TestSend_PurgesStaleToken(t *testing.T) {
	tokens := new(mockTokenRepo)
	fcm := new(mockMessenger)
	svc := NewService(tokens, deadRedis(), nil, fcm)

	tokens.On("GetByPlayerID", mock.Anything, "p1").
		Return(&PushToken{PlayerID: "p1", FCMToken: "stale", Platform: "android"}, nil)
	fcm.On("SendToToken", mock.Anything, "stale", "t", "b", mock.Anything).
		Return(fmt.Errorf("%w: gone", platform.ErrFCMTokenNotRegistered))
	tokens.On("Delete", mock.Anything, "p1").Return(nil)

	err := svc.Send(context.Background(), testMessage())

	assert.NoError(t, err, "stale token is not a retryable error")
	tokens.AssertCalled(t, "Delete", mock.Anything, "p1")
}

func TestSend_ReturnsErrorForRetryableFailure(t *testing.T) {
	tokens := new(mockTokenRepo)
	fcm := new(mockMessenger)
	svc := NewService(tokens, deadRedis(), nil, fcm)

	tokens.On("GetByPlayerID", mock.Anything, "p1").
		Return(&PushToken{PlayerID: "p1", FCMToken: "tok", Platform: "android"}, nil)
	fcm.On("SendToToken", mock.Anything, "tok", "t", "b", mock.Anything).
		Return(errors.New("fcm unavailable"))

	err := svc.Send(context.Background(), testMessage())

	assert.Error(t, err)
	tokens.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
}

func TestSend_LogOnlyWithoutMessenger(t *testing.T) {
	tokens := new(mockTokenRepo)
	svc := NewService(tokens, deadRedis(), nil, nil)

	tokens.On("GetByPlayerID", mock.Anything, "p1").
		Return(&PushToken{PlayerID: "p1", FCMToken: "tok", Platform: "android"}, nil)

	err := svc.Send(context.Background(), testMessage())

	assert.NoError(t, err)
}

func TestSend_SkipsSilentlyWithoutToken(t *testing.T) {
	tokens := new(mockTokenRepo)
	fcm := new(mockMessenger)
	svc := NewService(tokens, deadRedis(), nil, fcm)

	tokens.On("GetByPlayerID", mock.Anything, "p1").
		Return(nil, errors.New("not found"))

	err := svc.Send(context.Background(), testMessage())

	assert.NoError(t, err)
	fcm.AssertNotCalled(t, "SendToToken", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}
