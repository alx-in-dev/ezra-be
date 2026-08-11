package spire

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Contribution is one player's ledger entry for a Spire (escrow + tier).
type Contribution struct {
	PlayerID  string
	Fragments int
	Resources int
	Tier      string
}

type Repository interface {
	GetByRegion(ctx context.Context, regionKey string) (*Spire, error)
	Create(ctx context.Context, s *Spire) error
	AddFragment(ctx context.Context, spireID, playerID string) (int, error)
	SetState(ctx context.Context, spireID, state string) error
	OpenBuild(ctx context.Context, spireID string, target, pZone int, deadline time.Time) error
	CountActivePlayers(ctx context.Context, lat, lng float64, radiusM, activeDays int) (int, error)
	AddResources(ctx context.Context, spireID, playerID string, amount int) (int, error)
	Activate(ctx context.Context, spireID string) error
	Contribution(ctx context.Context, spireID, playerID string) (fragments, resources int, tier string, err error)
	ListContributions(ctx context.Context, spireID string) ([]Contribution, error)
	SetTier(ctx context.Context, spireID, playerID, tier string) error
	Maintain(ctx context.Context, spireID, playerID string, amount int) error
	TransitionStale(ctx context.Context, degradeBefore, ruinsBefore time.Time) (int64, error)
	ExpiredBuilds(ctx context.Context, now time.Time) ([]Spire, error)
	ResetForRefund(ctx context.Context, spireID string) error
	Rebuild(ctx context.Context, spireID string) error
}

type PgRepository struct{ db *pgxpool.Pool }

func NewPgRepository(db *pgxpool.Pool) *PgRepository { return &PgRepository{db: db} }

const spireCols = `id, region_key, lat, lng, state, fragments, build_progress, build_target, p_zone, radius_m, created_at, activated_at, build_deadline, last_activity_at`

func (r *PgRepository) GetByRegion(ctx context.Context, regionKey string) (*Spire, error) {
	rows, err := r.db.Query(ctx, `SELECT `+spireCols+` FROM spires WHERE region_key = $1`, regionKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	s, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[Spire])
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get spire: %w", err)
	}
	return &s, nil
}

func (r *PgRepository) Create(ctx context.Context, s *Spire) error {
	return r.db.QueryRow(ctx, `
		INSERT INTO spires (region_key, lat, lng, geom, state, fragments, build_progress, build_target, radius_m)
		VALUES ($1, $2, $3, ST_SetSRID(ST_MakePoint($3, $2), 4326), $4, 0, 0, 0, $5)
		ON CONFLICT (region_key) DO UPDATE SET region_key = EXCLUDED.region_key
		RETURNING id, created_at`,
		s.RegionKey, s.Lat, s.Lng, s.State, s.RadiusM,
	).Scan(&s.ID, &s.CreatedAt)
}

func (r *PgRepository) AddFragment(ctx context.Context, spireID, playerID string) (int, error) {
	if _, err := r.db.Exec(ctx, `
		INSERT INTO spire_contributions (spire_id, player_id, fragments, updated_at)
		VALUES ($1, $2, 1, now())
		ON CONFLICT (spire_id, player_id) DO UPDATE SET fragments = spire_contributions.fragments + 1, updated_at = now()`,
		spireID, playerID); err != nil {
		return 0, fmt.Errorf("log fragment contribution: %w", err)
	}
	var total int
	err := r.db.QueryRow(ctx,
		`UPDATE spires SET fragments = fragments + 1 WHERE id = $1 RETURNING fragments`, spireID).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("add spire fragment: %w", err)
	}
	return total, nil
}

func (r *PgRepository) SetState(ctx context.Context, spireID, state string) error {
	_, err := r.db.Exec(ctx, `UPDATE spires SET state = $2 WHERE id = $1`, spireID, state)
	return err
}

// OpenBuild flips a Spire to building with a population-derived target and an
// escrow deadline (frozen at open).
func (r *PgRepository) OpenBuild(ctx context.Context, spireID string, target, pZone int, deadline time.Time) error {
	_, err := r.db.Exec(ctx, `
		UPDATE spires SET state = 'building', build_target = $2, p_zone = $3,
			build_deadline = $4, last_activity_at = now()
		WHERE id = $1`, spireID, target, pZone, deadline)
	return err
}

// CountActivePlayers returns the number of recently-active residents (cores) in
// the Spire's radius — the P_zone snapshot used to size the build target.
func (r *PgRepository) CountActivePlayers(ctx context.Context, lat, lng float64, radiusM, activeDays int) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `
		SELECT count(*) FROM cores c
		JOIN players p ON p.id = c.player_id
		WHERE ST_DWithin(c.geom::geography, ST_SetSRID(ST_MakePoint($2, $1), 4326)::geography, $3)
		  AND p.last_active > now() - make_interval(days => $4)`,
		lat, lng, radiusM, activeDays).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count active players in zone: %w", err)
	}
	return n, nil
}

func (r *PgRepository) AddResources(ctx context.Context, spireID, playerID string, amount int) (int, error) {
	if _, err := r.db.Exec(ctx, `
		INSERT INTO spire_contributions (spire_id, player_id, resources, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (spire_id, player_id) DO UPDATE SET resources = spire_contributions.resources + $3, updated_at = now()`,
		spireID, playerID, amount); err != nil {
		return 0, fmt.Errorf("log resource contribution: %w", err)
	}
	var progress int
	err := r.db.QueryRow(ctx,
		`UPDATE spires SET build_progress = build_progress + $2, last_activity_at = now() WHERE id = $1 RETURNING build_progress`,
		spireID, amount).Scan(&progress)
	if err != nil {
		return 0, fmt.Errorf("add spire resources: %w", err)
	}
	return progress, nil
}

// Maintain bumps a Spire's upkeep timer and revives a degrading Spire back to
// active. Logs the player's resource contribution.
func (r *PgRepository) Maintain(ctx context.Context, spireID, playerID string, amount int) error {
	if _, err := r.db.Exec(ctx, `
		INSERT INTO spire_contributions (spire_id, player_id, resources, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (spire_id, player_id) DO UPDATE SET resources = spire_contributions.resources + $3, updated_at = now()`,
		spireID, playerID, amount); err != nil {
		return fmt.Errorf("log maintain: %w", err)
	}
	_, err := r.db.Exec(ctx, `
		UPDATE spires SET last_activity_at = now(),
			state = CASE WHEN state = 'degrading' THEN 'active' ELSE state END
		WHERE id = $1`, spireID)
	return err
}

// TransitionStale advances spires past their upkeep windows: active→degrading,
// degrading→ruins. Returns the number transitioned.
func (r *PgRepository) TransitionStale(ctx context.Context, degradeBefore, ruinsBefore time.Time) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE spires SET state = CASE
			WHEN state = 'active'    AND last_activity_at < $1 THEN 'degrading'
			WHEN state = 'degrading' AND last_activity_at < $2 THEN 'ruins'
			ELSE state END
		WHERE (state = 'active' AND last_activity_at < $1)
		   OR (state = 'degrading' AND last_activity_at < $2)`,
		degradeBefore, ruinsBefore)
	if err != nil {
		return 0, fmt.Errorf("transition stale spires: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ExpiredBuilds returns building Spires whose escrow deadline passed without
// reaching target (to be refunded and reset).
func (r *PgRepository) ExpiredBuilds(ctx context.Context, now time.Time) ([]Spire, error) {
	rows, err := r.db.Query(ctx, `SELECT `+spireCols+`
		FROM spires
		WHERE state = 'building' AND build_deadline IS NOT NULL
		  AND build_deadline < $1 AND build_progress < build_target`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list, err := pgx.CollectRows(rows, pgx.RowToStructByName[Spire])
	if err != nil {
		return nil, fmt.Errorf("list expired builds: %w", err)
	}
	return list, nil
}

// ResetForRefund clears the contribution ledger and returns the Spire to
// assembling for a fresh cycle (called after refunding contributors).
func (r *PgRepository) ResetForRefund(ctx context.Context, spireID string) error {
	if _, err := r.db.Exec(ctx, `DELETE FROM spire_contributions WHERE spire_id = $1`, spireID); err != nil {
		return fmt.Errorf("clear contributions: %w", err)
	}
	_, err := r.db.Exec(ctx, `
		UPDATE spires SET state = 'assembling', fragments = 0, build_progress = 0,
			p_zone = 0, build_target = 0, build_deadline = NULL, activated_at = NULL, last_activity_at = now()
		WHERE id = $1`, spireID)
	return err
}

// Rebuild resets a ruined Spire to assembling for a fresh build cycle.
func (r *PgRepository) Rebuild(ctx context.Context, spireID string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE spires SET state = 'assembling', fragments = 0, build_progress = 0,
			p_zone = 0, build_target = 0, build_deadline = NULL, activated_at = NULL, last_activity_at = now()
		WHERE id = $1 AND state = 'ruins'`, spireID)
	return err
}

func (r *PgRepository) Activate(ctx context.Context, spireID string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE spires SET state = 'active', activated_at = now(), build_deadline = NULL WHERE id = $1 AND state <> 'active'`, spireID)
	return err
}

func (r *PgRepository) Contribution(ctx context.Context, spireID, playerID string) (int, int, string, error) {
	var frags, res int
	var tier *string
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(fragments, 0), COALESCE(resources, 0), tier
		FROM spire_contributions WHERE spire_id = $1 AND player_id = $2`, spireID, playerID).Scan(&frags, &res, &tier)
	if err == pgx.ErrNoRows {
		return 0, 0, "", nil
	}
	t := ""
	if tier != nil {
		t = *tier
	}
	return frags, res, t, err
}

func (r *PgRepository) ListContributions(ctx context.Context, spireID string) ([]Contribution, error) {
	rows, err := r.db.Query(ctx, `
		SELECT player_id, COALESCE(fragments, 0), COALESCE(resources, 0), COALESCE(tier, '')
		FROM spire_contributions WHERE spire_id = $1`, spireID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Contribution
	for rows.Next() {
		var c Contribution
		if err := rows.Scan(&c.PlayerID, &c.Fragments, &c.Resources, &c.Tier); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *PgRepository) SetTier(ctx context.Context, spireID, playerID, tier string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE spire_contributions SET tier = $3 WHERE spire_id = $1 AND player_id = $2`, spireID, playerID, tier)
	return err
}
