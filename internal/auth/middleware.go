package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/ezra-game/server/pkg/httputil"
)

// AuthMiddleware validates the session token from the Authorization header.
func AuthMiddleware(authSvc *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if header == "" || !strings.HasPrefix(header, "Bearer ") {
				httputil.Error(w, httputil.NewUnauthorized("unauthorized", "missing authorization header"))
				return
			}
			token := header[7:]

			playerID, err := authSvc.GetPlayerIDByToken(r.Context(), token)
			if err != nil {
				httputil.Error(w, httputil.NewInternal("session check failed"))
				return
			}
			if playerID == "" {
				httputil.Error(w, httputil.NewUnauthorized("unauthorized", "invalid or expired session"))
				return
			}

			ctx := httputil.WithPlayerID(r.Context(), playerID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetPlayerID extracts player ID from request context.
// Deprecated: use httputil.GetPlayerID instead.
func GetPlayerID(ctx context.Context) string {
	return httputil.GetPlayerID(ctx)
}
