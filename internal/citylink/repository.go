package citylink

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository persists city links and link requests (Postgres).
type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

// SearchPlayers finds players whose username matches q (case-insensitive),
// excluding the requester. Bounded by limit.
func (r *Repository) SearchPlayers(ctx context.Context, q, excludeID string, limit int) ([]PlayerBrief, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, username, level FROM players
		WHERE username ILIKE '%' || $1 || '%' AND id <> $2
		ORDER BY username LIMIT $3`, q, excludeID, limit)
	if err != nil {
		return nil, fmt.Errorf("search players: %w", err)
	}
	defer rows.Close()
	var out []PlayerBrief
	for rows.Next() {
		var p PlayerBrief
		if err := rows.Scan(&p.ID, &p.Username, &p.Level); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// AreLinked reports whether a and b have an active City Link (order-agnostic).
func (r *Repository) AreLinked(ctx context.Context, a, b string) (bool, error) {
	lo, hi := canonical(a, b)
	var exists bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM city_links WHERE player_low = $1 AND player_high = $2)`, lo, hi).Scan(&exists)
	return exists, err
}

// CreateRequest inserts a pending request. The unique partial index makes a
// duplicate pending request error; callers translate that to a friendly message.
func (r *Repository) CreateRequest(ctx context.Context, from, to string) (*LinkRequest, error) {
	var req LinkRequest
	err := r.db.QueryRow(ctx, `
		INSERT INTO link_requests (from_player, to_player) VALUES ($1, $2)
		RETURNING id::text, from_player::text, to_player::text, state, created_at`,
		from, to).Scan(&req.ID, &req.FromPlayer, &req.ToPlayer, &req.State, &req.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &req, nil
}

// PendingBetween reports whether a pending request exists in EITHER direction
// between a and b (so we don't pile up cross requests).
func (r *Repository) PendingBetween(ctx context.Context, a, b string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM link_requests
		WHERE state = 'pending' AND
		      ((from_player = $1 AND to_player = $2) OR (from_player = $2 AND to_player = $1)))`,
		a, b).Scan(&exists)
	return exists, err
}

func (r *Repository) GetRequest(ctx context.Context, id string) (*LinkRequest, error) {
	var req LinkRequest
	err := r.db.QueryRow(ctx, `
		SELECT id::text, from_player::text, to_player::text, state, created_at
		FROM link_requests WHERE id = $1`, id).
		Scan(&req.ID, &req.FromPlayer, &req.ToPlayer, &req.State, &req.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &req, nil
}

func (r *Repository) SetRequestState(ctx context.Context, id, state string) error {
	_, err := r.db.Exec(ctx, `UPDATE link_requests SET state = $2 WHERE id = $1`, id, state)
	return err
}

// PendingInbox lists pending requests addressed to playerID, with sender name.
func (r *Repository) PendingInbox(ctx context.Context, toPlayer string) ([]LinkRequest, error) {
	rows, err := r.db.Query(ctx, `
		SELECT r.id::text, r.from_player::text, r.to_player::text, r.state, r.created_at, p.username
		FROM link_requests r JOIN players p ON p.id = r.from_player
		WHERE r.to_player = $1 AND r.state = 'pending'
		ORDER BY r.created_at DESC`, toPlayer)
	if err != nil {
		return nil, fmt.Errorf("pending inbox: %w", err)
	}
	defer rows.Close()
	var out []LinkRequest
	for rows.Next() {
		var req LinkRequest
		if err := rows.Scan(&req.ID, &req.FromPlayer, &req.ToPlayer, &req.State, &req.CreatedAt, &req.FromUsername); err != nil {
			return nil, err
		}
		out = append(out, req)
	}
	return out, rows.Err()
}

// CreateLink inserts the alliance (canonicalized, idempotent on conflict).
func (r *Repository) CreateLink(ctx context.Context, a, b string) error {
	lo, hi := canonical(a, b)
	_, err := r.db.Exec(ctx, `
		INSERT INTO city_links (player_low, player_high) VALUES ($1, $2)
		ON CONFLICT (player_low, player_high) DO NOTHING`, lo, hi)
	return err
}

// ListLinks returns playerID's active links from their perspective (partner side).
func (r *Repository) ListLinks(ctx context.Context, playerID string) ([]CityLink, error) {
	rows, err := r.db.Query(ctx, `
		SELECT l.id::text,
		       CASE WHEN l.player_low = $1 THEN l.player_high ELSE l.player_low END::text AS partner_id,
		       p.username, l.created_at
		FROM city_links l
		JOIN players p ON p.id = CASE WHEN l.player_low = $1 THEN l.player_high ELSE l.player_low END
		WHERE l.player_low = $1 OR l.player_high = $1
		ORDER BY l.created_at DESC`, playerID)
	if err != nil {
		return nil, fmt.Errorf("list links: %w", err)
	}
	defer rows.Close()
	var out []CityLink
	for rows.Next() {
		var cl CityLink
		if err := rows.Scan(&cl.ID, &cl.PartnerID, &cl.PartnerUsername, &cl.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, cl)
	}
	return out, rows.Err()
}

// RemoveLink deletes the alliance if playerID is a member. Returns rows affected.
func (r *Repository) RemoveLink(ctx context.Context, id, playerID string) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM city_links WHERE id = $1 AND (player_low = $2 OR player_high = $2)`, id, playerID)
	return tag.RowsAffected(), err
}

// LinkedPartnerIDs returns the ids of all players linked to playerID.
func (r *Repository) LinkedPartnerIDs(ctx context.Context, playerID string) ([]string, error) {
	rows, err := r.db.Query(ctx, `
		SELECT CASE WHEN player_low = $1 THEN player_high ELSE player_low END::text
		FROM city_links WHERE player_low = $1 OR player_high = $1`, playerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// canonical orders two ids so a pair maps to one row regardless of direction.
func canonical(a, b string) (string, string) {
	if a < b {
		return a, b
	}
	return b, a
}
