package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/pkg/httputil"
)

// Handler exposes HTTP endpoints for authentication.
type Handler struct {
	authService   authService
	playerService playerService
}

type authService interface {
	HashPassword(password string) (string, error)
	CheckPassword(hash, password string) bool
	CreateSession(ctx context.Context, playerID string) (string, error)
	DeleteSession(ctx context.Context, token string) error
	VerifyFirebaseToken(ctx context.Context, idToken string) (string, error)
}

type playerService interface {
	GetByLogin(ctx context.Context, login string) (*player.Player, error)
	CreatePlayerWithLogin(ctx context.Context, login, passwordHash string) (*player.Player, error)
	GetByFirebaseUID(ctx context.Context, uid string) (*player.Player, error)
	CreatePlayer(ctx context.Context, firebaseUID, username string) (*player.Player, error)
}

// NewHandler creates a new auth Handler.
func NewHandler(authSvc *Service, playerSvc *player.Service) *Handler {
	return &Handler{authService: authSvc, playerService: playerSvc}
}

// Login handles POST /auth/login.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := httputil.Decode(r, &req); err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", err.Error()))
		return
	}
	if req.FirebaseToken != "" {
		h.loginWithFirebase(w, r, req.FirebaseToken, req.Username)
		return
	}

	h.loginWithPassword(w, r, req.Login, req.Password)
}

func (h *Handler) loginWithPassword(w http.ResponseWriter, r *http.Request, login, password string) {
	if login == "" || password == "" {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", "firebase_token or login/password are required"))
		return
	}

	p, err := h.playerService.GetByLogin(r.Context(), login)
	if err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_credentials", "invalid login or password"))
		return
	}

	if !h.authService.CheckPassword(p.PasswordHash, password) {
		httputil.Error(w, httputil.NewBadRequest("invalid_credentials", "invalid login or password"))
		return
	}

	h.respondWithSession(w, r, p, http.StatusOK)
}

func (h *Handler) loginWithFirebase(w http.ResponseWriter, r *http.Request, firebaseToken, username string) {
	uid, err := h.authService.VerifyFirebaseToken(r.Context(), firebaseToken)
	if err != nil {
		httputil.Error(w, httputil.NewUnauthorized("firebase_auth_failed", err.Error()))
		return
	}

	p, err := h.playerService.GetByFirebaseUID(r.Context(), uid)
	if err != nil {
		if !errors.Is(err, player.ErrPlayerNotFound) {
			httputil.Error(w, httputil.NewInternal("failed to load firebase player"))
			return
		}
		p, err = h.playerService.CreatePlayer(r.Context(), uid, strings.TrimSpace(username))
		if err != nil {
			httputil.Error(w, httputil.NewBadRequest("register_failed", "failed to create firebase player"))
			return
		}
		h.respondWithSession(w, r, p, http.StatusCreated)
		return
	}

	h.respondWithSession(w, r, p, http.StatusOK)
}

// Register handles POST /auth/register.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := httputil.Decode(r, &req); err != nil {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", err.Error()))
		return
	}
	if req.FirebaseToken != "" {
		h.loginWithFirebase(w, r, req.FirebaseToken, req.Username)
		return
	}

	if req.Login == "" || req.Password == "" {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", "firebase_token or login/password are required"))
		return
	}
	if len(req.Password) < 6 {
		httputil.Error(w, httputil.NewBadRequest("invalid_body", "password must be at least 6 characters"))
		return
	}

	hash, err := h.authService.HashPassword(req.Password)
	if err != nil {
		httputil.Error(w, httputil.NewInternal("failed to hash password"))
		return
	}

	p, err := h.playerService.CreatePlayerWithLogin(r.Context(), req.Login, hash)
	if err != nil {
		httputil.Error(w, httputil.NewBadRequest("register_failed", "login already taken or invalid"))
		return
	}

	h.respondWithSession(w, r, p, http.StatusCreated)
}

func (h *Handler) respondWithSession(w http.ResponseWriter, r *http.Request, p *player.Player, status int) {
	sessionToken, err := h.authService.CreateSession(r.Context(), p.ID)
	if err != nil {
		httputil.Error(w, httputil.NewInternal("failed to create session"))
		return
	}

	httputil.JSON(w, status, AuthResponse{
		Player:       player.BuildProfilePayload(p),
		SessionToken: sessionToken,
	})
}

// Logout handles POST /auth/logout.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("Authorization")
	if len(token) > 7 && token[:7] == "Bearer " {
		token = token[7:]
		_ = h.authService.DeleteSession(r.Context(), token)
	}
	w.WriteHeader(http.StatusNoContent)
}
