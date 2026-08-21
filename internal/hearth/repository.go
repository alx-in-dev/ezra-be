package hearth

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository persists ephemeral hearths.
type Repository interface {
	Create(ctx context.Context, h *EphemeralHearth) error
	// LastByOwner returns the most recently created hearth for playerID
	// (active or expired), used for the cap=1 + cooldown checks in Summon.
	LastByOwner(ctx context.Context, playerID string) (*EphemeralHearth, bool, error)
	// ActiveNearby reports whether any still-active (unexpired) hearth is
	// within radiusM of (lat,lng) — any owner's, buffs are shared.
	ActiveNearby(ctx context.Context, lat, lng, radiusM float64) (bool, error)
}

type PgRepository struct {
	db *pgxpool.Pool
}

func NewPgRepository(db *pgxpool.Pool) *PgRepository {
	return &PgRepository{db: db}
}

func (r *PgRepository) Create(ctx context.Context, h *EphemeralHearth) error {
	const q = `INSERT INTO ephemeral_hearths (owner_id, lat, lng, geom, expires_at)
		VALUES ($1, $2, $3, ST_SetSRID(ST_MakePoint($3, $2), 4326), $4)
		RETURNING id, created_at`
	return r.db.QueryRow(ctx, q, h.OwnerID, h.Lat, h.Lng, h.ExpiresAt).Scan(&h.ID, &h.CreatedAt)
}

func (r *PgRepository) LastByOwner(ctx context.Context, playerID string) (*EphemeralHearth, bool, error) {
	var h EphemeralHearth
	err := r.db.QueryRow(ctx, `
		SELECT id::text, owner_id::text, lat, lng, created_at, expires_at
		FROM ephemeral_hearths WHERE owner_id = $1
		ORDER BY created_at DESC LIMIT 1`, playerID).
		Scan(&h.ID, &h.OwnerID, &h.Lat, &h.Lng, &h.CreatedAt, &h.ExpiresAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("last hearth by owner: %w", err)
	}
	return &h, true, nil
}

func (r *PgRepository) ActiveNearby(ctx context.Context, lat, lng, radiusM float64) (bool, error) {
	var found bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM ephemeral_hearths
			WHERE expires_at > now()
			  AND ST_DWithin(geom::geography, ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography, $3)
		)`, lng, lat, radiusM).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("active hearth nearby: %w", err)
	}
	return found, nil
}
