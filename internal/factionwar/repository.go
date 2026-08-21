package factionwar

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	AddPoints(ctx context.Context, playerID, season string, delta int) (int, error)
	GetPoints(ctx context.Context, playerID, season string) (int, error)
	TopBySeason(ctx context.Context, season string, limit int) ([]Standing, error)
	RegionBalance(ctx context.Context, lat, lng, radiusM float64) (RegionBalance, error)
	IsSettled(ctx context.Context, season string) (bool, error)
	MarkSettled(ctx context.Context, season string) error
	RecordReward(ctx context.Context, playerID, season string, rank, points, crystals int) error
	UnsettledSeasons(ctx context.Context, exceptSeason string) ([]string, error)
	LastReward(ctx context.Context, playerID string) (*SeasonReward, error)
	// SeasonScoreCount reports how many players have scored in a season — 0 means
	// the season is fresh (post-wipe), the between-seasons window (T-826).
	SeasonScoreCount(ctx context.Context, season string) (int, error)
}

type PgRepository struct {
	db *pgxpool.Pool
}

func NewPgRepository(db *pgxpool.Pool) *PgRepository {
	return &PgRepository{db: db}
}

func (r *PgRepository) AddPoints(ctx context.Context, playerID, season string, delta int) (int, error) {
	var total int
	err := r.db.QueryRow(ctx, `
		INSERT INTO faction_scores (player_id, season, points, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (player_id, season)
		DO UPDATE SET points = faction_scores.points + EXCLUDED.points, updated_at = now()
		RETURNING points`,
		playerID, season, delta).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("add faction points: %w", err)
	}
	return total, nil
}

func (r *PgRepository) GetPoints(ctx context.Context, playerID, season string) (int, error) {
	var points int
	err := r.db.QueryRow(ctx,
		`SELECT COALESCE((SELECT points FROM faction_scores WHERE player_id=$1 AND season=$2), 0)`,
		playerID, season).Scan(&points)
	return points, err
}

func (r *PgRepository) TopBySeason(ctx context.Context, season string, limit int) ([]Standing, error) {
	rows, err := r.db.Query(ctx, `
		SELECT fs.player_id, COALESCE(p.username, 'Игрок'), fs.points
		FROM faction_scores fs JOIN players p ON p.id = fs.player_id
		WHERE fs.season = $1 AND fs.points > 0
		ORDER BY fs.points DESC LIMIT $2`,
		season, limit)
	if err != nil {
		return nil, fmt.Errorf("top by season: %w", err)
	}
	defer rows.Close()
	var out []Standing
	for rows.Next() {
		var s Standing
		if err := rows.Scan(&s.PlayerID, &s.Username, &s.Points); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *PgRepository) IsSettled(ctx context.Context, season string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM faction_season_settlements WHERE season = $1)`, season).Scan(&exists)
	return exists, err
}

func (r *PgRepository) MarkSettled(ctx context.Context, season string) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO faction_season_settlements (season) VALUES ($1) ON CONFLICT (season) DO NOTHING`, season)
	return err
}

func (r *PgRepository) RecordReward(ctx context.Context, playerID, season string, rank, points, crystals int) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO faction_season_rewards (player_id, season, rank, points, crystals)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (player_id, season) DO NOTHING`,
		playerID, season, rank, points, crystals)
	return err
}

// UnsettledSeasons returns past seasons that have scores but no settlement yet.
func (r *PgRepository) UnsettledSeasons(ctx context.Context, exceptSeason string) ([]string, error) {
	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT fs.season FROM faction_scores fs
		WHERE fs.season <> $1
		  AND NOT EXISTS (SELECT 1 FROM faction_season_settlements s WHERE s.season = fs.season)`,
		exceptSeason)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *PgRepository) SeasonScoreCount(ctx context.Context, season string) (int, error) {
	var n int
	err := r.db.QueryRow(ctx,
		`SELECT count(*) FROM faction_scores WHERE season = $1`, season).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("season score count: %w", err)
	}
	return n, nil
}

func (r *PgRepository) LastReward(ctx context.Context, playerID string) (*SeasonReward, error) {
	var sr SeasonReward
	err := r.db.QueryRow(ctx, `
		SELECT season, rank, points, crystals FROM faction_season_rewards
		WHERE player_id = $1 ORDER BY claimed_at DESC LIMIT 1`, playerID).
		Scan(&sr.Season, &sr.Rank, &sr.Points, &sr.Crystals)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &sr, nil
}

// Source-aura radii for the N1 territory score. Mirror of infection's
// hiveRadiusL1/L2/L3 and maxRiftSearchM (and hive.LevelConfig.RadiusM) — kept as
// local literals to avoid an infection/hive → factionwar import edge, the same
// workaround wave 1 used. A cell is Symbiont territory when it sits inside one
// of these reaches of an open source (or is currently pierced).
const (
	auraHiveRadiusL1 = 200.0
	auraHiveRadiusL2 = 350.0
	auraHiveRadiusL3 = 500.0
	auraRiftRadiusM  = 200.0

	// Nest aura radii by level (N3, T-832). A live Nest projects Symbiont
	// territory just like a hive/rift. Local literals to avoid a canon/nest
	// import edge (same pattern as the hive radii) — keep in sync with
	// canon.NestConfig / nest.LevelConfig / infection's nestRadiusL*.
	auraNestRadiusL1 = 200.0
	auraNestRadiusL2 = 275.0
	auraNestRadiusL3 = 350.0
	auraNestRadiusL4 = 425.0
	auraNestRadiusL5 = 500.0
)

func (r *PgRepository) RegionBalance(ctx context.Context, lat, lng, radiusM float64) (RegionBalance, error) {
	var b RegionBalance
	// N1 (T-822): score projected territory, not ambient infection. Per region
	// cell we resolve domed / pierced / in-source-aura once, then reduce:
	//   Human    = domed AND NOT pierced  (its aura minus Symbiont-carved holes)
	//   Symbiont = aura OR pierced        (source reach plus pierced cells, deduped)
	// A pierced cell is by definition a domed cell (pierce = TTL bypass on a
	// domed cell), so excluding pierced from Human is what stops one cell scoring
	// for BOTH sides. Ambient clean/infected counts are carried as diagnostics.
	err := r.db.QueryRow(ctx, `
		WITH pt AS (SELECT ST_SetSRID(ST_MakePoint($2::float8, $1::float8), 4326)::geography AS g),
		region AS (
			SELECT
				(EXISTS (SELECT 1 FROM domed_cells d WHERE d.cell_id = c.id)) AS domed,
				(c.pierced_until IS NOT NULL AND c.pierced_until > now()) AS pierced,
				(c.infection <= 20) AS clean,
				(c.infection > 50) AS infected,
				(EXISTS (
					SELECT 1 FROM hives h WHERE h.closed_at IS NULL
					  AND ST_DWithin(c.geom::geography, h.geom::geography,
							CASE h.level WHEN 1 THEN $4::float8 WHEN 2 THEN $5::float8 ELSE $6::float8 END)
				) OR EXISTS (
					SELECT 1 FROM rifts rf JOIN cells rc ON rc.id = rf.cell_id
					WHERE rf.closed_at IS NULL AND ST_DWithin(c.geom::geography, rc.geom::geography, $7::float8)
				) OR EXISTS (
					-- T-832: a live Nest extends Symbiont aura (boolean OR — no double
					-- count with hives/rifts/pierced, ADR-N3-2).
					SELECT 1 FROM nests n WHERE n.collapsed_at IS NULL
					  AND ST_DWithin(c.geom::geography, n.geom::geography,
							CASE n.level WHEN 1 THEN $8::float8 WHEN 2 THEN $9::float8
										 WHEN 3 THEN $10::float8 WHEN 4 THEN $11::float8
										 ELSE $12::float8 END)
				)) AS aura
			FROM cells c, pt
			WHERE ST_DWithin(c.geom::geography, pt.g, $3)
		)
		SELECT
			count(*) FILTER (WHERE domed),
			count(*) FILTER (WHERE clean),
			count(*) FILTER (WHERE infected),
			count(*) FILTER (WHERE aura),
			count(*) FILTER (WHERE pierced),
			count(*) FILTER (WHERE domed AND NOT pierced),
			count(*) FILTER (WHERE aura OR pierced),
			(SELECT count(*) FROM hives h, pt WHERE h.closed_at IS NULL AND ST_DWithin(h.geom::geography, pt.g, $3)),
			(SELECT count(*) FROM rifts rf JOIN cells rc ON rc.id = rf.cell_id, pt
				WHERE rf.closed_at IS NULL AND ST_DWithin(rc.geom::geography, pt.g, $3))
		FROM region`,
		lat, lng, radiusM, auraHiveRadiusL1, auraHiveRadiusL2, auraHiveRadiusL3, auraRiftRadiusM,
		auraNestRadiusL1, auraNestRadiusL2, auraNestRadiusL3, auraNestRadiusL4, auraNestRadiusL5).
		Scan(&b.DomedCells, &b.CleanCells, &b.InfectedCells, &b.AuraCells, &b.PiercedCells,
			&b.HumanRaw, &b.SymbiontRaw, &b.OpenHives, &b.OpenRifts)
	if err != nil {
		return b, fmt.Errorf("region balance: %w", err)
	}
	total := b.HumanRaw + b.SymbiontRaw
	if total > 0 {
		b.HumanPct = int(float64(b.HumanRaw) * 100.0 / float64(total))
		b.SymbiontPct = 100 - b.HumanPct
	} else {
		b.HumanPct = 50
		b.SymbiontPct = 50
	}
	return b, nil
}
