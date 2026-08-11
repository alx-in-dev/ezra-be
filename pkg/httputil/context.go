package httputil

import "context"

type contextKey string

// PlayerIDKey is the context key for the authenticated player ID.
const PlayerIDKey contextKey = "player_id"

// GetPlayerID extracts the player ID from request context.
func GetPlayerID(ctx context.Context) string {
	if v, ok := ctx.Value(PlayerIDKey).(string); ok {
		return v
	}
	return ""
}

// WithPlayerID stores the player ID in context.
func WithPlayerID(ctx context.Context, playerID string) context.Context {
	return context.WithValue(ctx, PlayerIDKey, playerID)
}
