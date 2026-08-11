package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ezra-game/server/internal/player"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeAuthService struct {
	verifyFirebaseToken func(ctx context.Context, token string) (string, error)
	checkPassword       func(hash, password string) bool
	createSession       func(ctx context.Context, playerID string) (string, error)
}

func (f *fakeAuthService) HashPassword(password string) (string, error) {
	return "hashed:" + password, nil
}

func (f *fakeAuthService) CheckPassword(hash, password string) bool {
	if f.checkPassword != nil {
		return f.checkPassword(hash, password)
	}
	return hash == "hashed:"+password
}

func (f *fakeAuthService) CreateSession(ctx context.Context, playerID string) (string, error) {
	if f.createSession != nil {
		return f.createSession(ctx, playerID)
	}
	return "session-token", nil
}

func (f *fakeAuthService) DeleteSession(ctx context.Context, token string) error {
	return nil
}

func (f *fakeAuthService) VerifyFirebaseToken(ctx context.Context, token string) (string, error) {
	if f.verifyFirebaseToken != nil {
		return f.verifyFirebaseToken(ctx, token)
	}
	return "", errors.New("not configured")
}

type fakePlayerService struct {
	getByLogin       func(ctx context.Context, login string) (*player.Player, error)
	createWithLogin  func(ctx context.Context, login, passwordHash string) (*player.Player, error)
	getByFirebaseUID func(ctx context.Context, uid string) (*player.Player, error)
	createPlayer     func(ctx context.Context, firebaseUID, username string) (*player.Player, error)
}

func (f *fakePlayerService) GetByLogin(ctx context.Context, login string) (*player.Player, error) {
	return f.getByLogin(ctx, login)
}

func (f *fakePlayerService) CreatePlayerWithLogin(ctx context.Context, login, passwordHash string) (*player.Player, error) {
	return f.createWithLogin(ctx, login, passwordHash)
}

func (f *fakePlayerService) GetByFirebaseUID(ctx context.Context, uid string) (*player.Player, error) {
	return f.getByFirebaseUID(ctx, uid)
}

func (f *fakePlayerService) CreatePlayer(ctx context.Context, firebaseUID, username string) (*player.Player, error) {
	return f.createPlayer(ctx, firebaseUID, username)
}

func TestLoginFirebaseExistingPlayer(t *testing.T) {
	handler := &Handler{
		authService: &fakeAuthService{
			verifyFirebaseToken: func(ctx context.Context, token string) (string, error) {
				assert.Equal(t, "firebase-id-token", token)
				return "firebase-uid", nil
			},
		},
		playerService: &fakePlayerService{
			getByLogin: func(ctx context.Context, login string) (*player.Player, error) {
				return nil, errors.New("not used")
			},
			createWithLogin: func(ctx context.Context, login, passwordHash string) (*player.Player, error) {
				return nil, errors.New("not used")
			},
			getByFirebaseUID: func(ctx context.Context, uid string) (*player.Player, error) {
				assert.Equal(t, "firebase-uid", uid)
				return &player.Player{ID: "p1", Username: "Ezra", UsernameIsCustom: true, Level: 1}, nil
			},
			createPlayer: func(ctx context.Context, firebaseUID, username string) (*player.Player, error) {
				return nil, errors.New("not used")
			},
		},
	}

	body := bytes.NewBufferString(`{"firebase_token":"firebase-id-token"}`)
	req := httptest.NewRequest(http.MethodPost, "/auth/login", body)
	rr := httptest.NewRecorder()

	handler.Login(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp AuthResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, "session-token", resp.SessionToken)
	payload, ok := resp.Player.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Ezra", payload["username"])
}

func TestLoginFirebaseCreatesPlayer(t *testing.T) {
	handler := &Handler{
		authService: &fakeAuthService{
			verifyFirebaseToken: func(ctx context.Context, token string) (string, error) {
				return "new-firebase-uid", nil
			},
		},
		playerService: &fakePlayerService{
			getByLogin: func(ctx context.Context, login string) (*player.Player, error) {
				return nil, errors.New("not used")
			},
			createWithLogin: func(ctx context.Context, login, passwordHash string) (*player.Player, error) {
				return nil, errors.New("not used")
			},
			getByFirebaseUID: func(ctx context.Context, uid string) (*player.Player, error) {
				return nil, player.ErrPlayerNotFound
			},
			createPlayer: func(ctx context.Context, firebaseUID, username string) (*player.Player, error) {
				assert.Equal(t, "new-firebase-uid", firebaseUID)
				assert.Equal(t, "Nomad", username)
				return &player.Player{ID: "p2", Username: "Nomad", UsernameIsCustom: true, Level: 1}, nil
			},
		},
	}

	body := bytes.NewBufferString(`{"firebase_token":"firebase-id-token","username":"Nomad"}`)
	req := httptest.NewRequest(http.MethodPost, "/auth/login", body)
	rr := httptest.NewRecorder()

	handler.Login(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code)
	var resp AuthResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, "session-token", resp.SessionToken)
}

func TestLoginFirebaseLookupFailureReturnsInternalError(t *testing.T) {
	handler := &Handler{
		authService: &fakeAuthService{
			verifyFirebaseToken: func(ctx context.Context, token string) (string, error) {
				return "firebase-uid", nil
			},
		},
		playerService: &fakePlayerService{
			getByLogin: func(ctx context.Context, login string) (*player.Player, error) {
				return nil, errors.New("not used")
			},
			createWithLogin: func(ctx context.Context, login, passwordHash string) (*player.Player, error) {
				return nil, errors.New("not used")
			},
			getByFirebaseUID: func(ctx context.Context, uid string) (*player.Player, error) {
				return nil, errors.New("database offline")
			},
			createPlayer: func(ctx context.Context, firebaseUID, username string) (*player.Player, error) {
				return nil, errors.New("must not create player")
			},
		},
	}

	body := bytes.NewBufferString(`{"firebase_token":"firebase-id-token"}`)
	req := httptest.NewRequest(http.MethodPost, "/auth/login", body)
	rr := httptest.NewRecorder()

	handler.Login(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestLoginPasswordFallback(t *testing.T) {
	handler := &Handler{
		authService: &fakeAuthService{
			checkPassword: func(hash, password string) bool {
				return hash == "stored-hash" && password == "secret123"
			},
		},
		playerService: &fakePlayerService{
			getByLogin: func(ctx context.Context, login string) (*player.Player, error) {
				assert.Equal(t, "tester", login)
				return &player.Player{ID: "p3", Username: "tester", PasswordHash: "stored-hash", Level: 1}, nil
			},
			createWithLogin: func(ctx context.Context, login, passwordHash string) (*player.Player, error) {
				return nil, errors.New("not used")
			},
			getByFirebaseUID: func(ctx context.Context, uid string) (*player.Player, error) {
				return nil, errors.New("not used")
			},
			createPlayer: func(ctx context.Context, firebaseUID, username string) (*player.Player, error) {
				return nil, errors.New("not used")
			},
		},
	}

	body := bytes.NewBufferString(`{"login":"tester","password":"secret123"}`)
	req := httptest.NewRequest(http.MethodPost, "/auth/login", body)
	rr := httptest.NewRecorder()

	handler.Login(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
}
