package nest

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ezra-game/server/internal/canon"
)

// nestColumns is the canonical column list (order matches RowToStructByName's
// name matching; AuraRadiusM is db:"-" and excluded).
const nestColumns = `id, owner_id, cell_id, lat, lng, level, accrued_resonance,
	support_level, siege_hp, siege_state, collapse_at, siege_attacker_id,
	relocated_at, created_at, collapsed_at`

// Repository is the nest persistence port (kept an interface so the service
// stays testable without a live DB).
type Repository interface {
	Create(ctx context.Context, n *Nest) error
	GetLiveByOwner(ctx context.Context, ownerID string) (*Nest, error)
	GetByID(ctx context.Context, id string) (*Nest, error)
	HasEverOwned(ctx context.Context, ownerID string) (bool, error)
	UpdateLocation(ctx context.Context, n *Nest) error

	// AccrueTrickle adds each live nest's per-level trickle (scaled by its
	// support factor and the owner's Energist skill, T-845) into its
	// accrued_resonance buffer, capped. Set-based.
	AccrueTrickle(ctx context.Context, trickleByLevel [canon.NestMaxLevel + 1]float64, cap, energistPerPoint float64) (int64, error)
	// ApplyDecay lowers each live nest's support toward the floor (never below).
	ApplyDecay(ctx context.Context, decayPerTick, floor float64) (int64, error)
	// Feed restores a nest's support to max (T-834).
	Feed(ctx context.Context, id string, toSupport float64) error
	// CollectBuffer atomically zeroes a nest's trickle buffer and returns the
	// amount that was collected (T-833) — moved to the profile by the service.
	CollectBuffer(ctx context.Context, id string) (float64, error)

	// UpdateSiege persists a nest's siege fields (siege_hp/state/collapse_at/
	// attacker) after the service computes the state transition (ADR-N3-4).
	UpdateSiege(ctx context.Context, n *Nest) error
	// ApplyCollapses is the single-writer terminal step (ADR-N3-4 invariant 2):
	// nests past their collapse_at are marked collapsed (collapsed_at=now, buffer
	// zeroed — the Executable Loss). Returns the collapsed nest ids so the caller
	// can release their pocket cells. Set-based.
	ApplyCollapses(ctx context.Context) ([]string, error)

	// CellPlacementStatus reports whether a cell is domed and/or pierced (T-843).
	CellPlacementStatus(ctx context.Context, cellID string) (domed, pierced bool, err error)
	// AddPocketCell records that a nest holds a pierced pocket cell (T-843).
	AddPocketCell(ctx context.Context, nestID, cellID string) error
	// RefreshPocketCells pushes pierced_until forward for live nests' pocket
	// cells (T-843, ADR-N3-8). Set-based.
	RefreshPocketCells(ctx context.Context, holdSeconds int) (int64, error)
	// GetNearby returns live nests near a point (attacker-side map).
	GetNearby(ctx context.Context, lat, lng, radiusM float64) ([]Nest, error)
}

// PgRepository is the PostgreSQL/PostGIS implementation.
type PgRepository struct {
	db *pgxpool.Pool
}

func NewPgRepository(db *pgxpool.Pool) *PgRepository {
	return &PgRepository{db: db}
}

// Create inserts a new live nest. The partial UNIQUE index (owner_id) WHERE
// collapsed_at IS NULL enforces cap 1 live nest/player at the DB level.
func (r *PgRepository) Create(ctx context.Context, n *Nest) error {
	err := r.db.QueryRow(ctx, `
		INSERT INTO nests (owner_id, cell_id, lat, lng, geom, level, siege_hp)
		VALUES ($1, $2, $3, $4, ST_SetSRID(ST_MakePoint($4, $3), 4326), $5, $6)
		RETURNING id, created_at, accrued_resonance, support_level, siege_state`,
		n.OwnerID, n.CellID, n.Lat, n.Lng, n.Level, n.Config().SiegeHPMax,
	).Scan(&n.ID, &n.CreatedAt, &n.AccruedResonance, &n.SupportLevel, &n.SiegeState)
	if err != nil {
		return fmt.Errorf("create nest: %w", err)
	}
	n.SiegeHP = n.Config().SiegeHPMax
	n.decorate()
	return nil
}

// GetLiveByOwner returns the player's live nest, or (nil, nil) if none.
func (r *PgRepository) GetLiveByOwner(ctx context.Context, ownerID string) (*Nest, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+nestColumns+` FROM nests WHERE owner_id = $1 AND collapsed_at IS NULL`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	n, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[Nest])
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get live nest: %w", err)
	}
	n.decorate()
	return &n, nil
}

// GetByID returns a nest by id (live or collapsed).
func (r *PgRepository) GetByID(ctx context.Context, id string) (*Nest, error) {
	rows, err := r.db.Query(ctx, `SELECT `+nestColumns+` FROM nests WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	n, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[Nest])
	if err != nil {
		return nil, fmt.Errorf("nest not found: %w", err)
	}
	n.decorate()
	return &n, nil
}

// HasEverOwned reports whether the player has ANY nest row (live or collapsed).
// "First nest is free" and grandfathering (ADR-N3-11) read this: a prod
// Symbiont with no row gets a free nest with no retro-migration; a rebuild
// after collapse sees the history row and is no longer "first".
func (r *PgRepository) HasEverOwned(ctx context.Context, ownerID string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM nests WHERE owner_id = $1)`, ownerID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("has ever owned nest: %w", err)
	}
	return exists, nil
}

// UpdateLocation moves a nest to a new cell/point (relocation). Level, buffer
// and siege state are untouched — only position changes (mirror of RelocateCore).
func (r *PgRepository) UpdateLocation(ctx context.Context, n *Nest) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE nests
		SET cell_id = $2, lat = $3, lng = $4,
		    geom = ST_SetSRID(ST_MakePoint($4, $3), 4326),
		    relocated_at = now()
		WHERE id = $1 AND collapsed_at IS NULL`,
		n.ID, n.CellID, n.Lat, n.Lng)
	if err != nil {
		return fmt.Errorf("relocate nest: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("relocate nest: no live nest with id %s", n.ID)
	}
	return nil
}

// AccrueTrickle adds trickle to every live, non-collapsing nest's buffer. The
// per-tick amount scales with the nest's support factor (support/max, clamped
// to 1) — a decayed nest trickles less — and is capped at `cap`. Collapsing/
// collapsed nests get nothing (their aura is already muted). Set-based, one
// UPDATE (ADR-N3-9): the working set is bounded by cap 1-nest/player.
func (r *PgRepository) AccrueTrickle(ctx context.Context, t [canon.NestMaxLevel + 1]float64, cap, energistPerPoint float64) (int64, error) {
	// Join players to read the owner's Energist skill (T-845): a skill-tree
	// choice raises trickle, on top of the support factor. skills is JSONB;
	// COALESCE guards a missing/empty key. Working set is bounded by cap
	// 1-nest/player, so the join is tiny.
	tag, err := r.db.Exec(ctx, `
		UPDATE nests n SET accrued_resonance = LEAST($6::float8,
			n.accrued_resonance
			+ (CASE n.level WHEN 1 THEN $1::float8 WHEN 2 THEN $2::float8 WHEN 3 THEN $3::float8
							WHEN 4 THEN $4::float8 ELSE $5::float8 END)
			  * LEAST(1.0, n.support_level / $7::float8)
			  * (1.0 + $8::float8 * COALESCE((p.skills->>'energist')::int, 0)))
		FROM players p
		WHERE p.id = n.owner_id
		  AND n.collapsed_at IS NULL AND n.siege_state IN ('healthy','under_siege')`,
		t[1], t[2], t[3], t[4], t[5], cap, canon.NestSupportMax, energistPerPoint)
	if err != nil {
		return 0, fmt.Errorf("accrue nest trickle: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ApplyDecay lowers each live nest's support toward the floor (never below —
// neglect degrades output, not existence; US-N3-5). Set-based.
func (r *PgRepository) ApplyDecay(ctx context.Context, decayPerTick, floor float64) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE nests SET support_level = GREATEST($2::float8, support_level - $1::float8)
		WHERE collapsed_at IS NULL AND support_level > $2::float8`,
		decayPerTick, floor)
	if err != nil {
		return 0, fmt.Errorf("apply nest decay: %w", err)
	}
	return tag.RowsAffected(), nil
}

// Feed restores a live nest's support (T-834, semantics of hive.Empower).
func (r *PgRepository) Feed(ctx context.Context, id string, toSupport float64) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE nests SET support_level = $2 WHERE id = $1 AND collapsed_at IS NULL`,
		id, toSupport)
	if err != nil {
		return fmt.Errorf("feed nest: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("feed nest: no live nest with id %s", id)
	}
	return nil
}

// CollectBuffer atomically zeroes a nest's trickle buffer and returns what was
// in it (T-833). The service floors this to an int and grants it to the profile.
func (r *PgRepository) CollectBuffer(ctx context.Context, id string) (float64, error) {
	var collected float64
	// Capture the OLD buffer value in a CTE before the UPDATE zeroes it — a
	// plain RETURNING would give the new (0) value.
	err := r.db.QueryRow(ctx, `
		WITH old AS (
			SELECT accrued_resonance AS v FROM nests
			WHERE id = $1 AND collapsed_at IS NULL FOR UPDATE
		)
		UPDATE nests SET accrued_resonance = 0
		WHERE id = $1 AND collapsed_at IS NULL AND EXISTS (SELECT 1 FROM old)
		RETURNING (SELECT v FROM old)`, id).Scan(&collected)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("collect nest buffer: no live nest with id %s", id)
	}
	if err != nil {
		return 0, fmt.Errorf("collect nest buffer: %w", err)
	}
	return collected, nil
}

// UpdateSiege persists the siege fields after a service-computed transition.
func (r *PgRepository) UpdateSiege(ctx context.Context, n *Nest) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE nests
		SET siege_hp = $2, siege_state = $3, collapse_at = $4, siege_attacker_id = $5
		WHERE id = $1 AND collapsed_at IS NULL`,
		n.ID, n.SiegeHP, n.SiegeState, n.CollapseAt, n.SiegeAttackerID)
	if err != nil {
		return fmt.Errorf("update nest siege: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update nest siege: no live nest with id %s", n.ID)
	}
	return nil
}

// ApplyCollapses marks every live nest whose collapse timer has elapsed as
// collapsed (single-writer, called only by nest:tick — ADR-N3-4 invariant 2).
// The buffer is zeroed (Executable Loss). Returns collapsed nest ids.
func (r *PgRepository) ApplyCollapses(ctx context.Context) ([]string, error) {
	rows, err := r.db.Query(ctx, `
		UPDATE nests
		SET siege_state = 'collapsed', collapsed_at = now(), accrued_resonance = 0
		WHERE collapsed_at IS NULL
		  AND siege_state IN ('under_siege','collapsing')
		  AND collapse_at IS NOT NULL AND collapse_at <= now()
		RETURNING id`)
	if err != nil {
		return nil, fmt.Errorf("apply nest collapses: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan collapsed nest: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// CellPlacementStatus reports whether a cell is currently under a dome and/or
// currently pierced (T-843). A nest may not be placed under a foreign dome
// UNLESS the cell is a carved pocket (currently pierced).
func (r *PgRepository) CellPlacementStatus(ctx context.Context, cellID string) (domed, pierced bool, err error) {
	err = r.db.QueryRow(ctx, `
		SELECT
			EXISTS(SELECT 1 FROM domed_cells d WHERE d.cell_id = $1),
			EXISTS(SELECT 1 FROM cells c WHERE c.id = $1
			       AND c.pierced_until IS NOT NULL AND c.pierced_until > now())`,
		cellID).Scan(&domed, &pierced)
	if err != nil {
		return false, false, fmt.Errorf("cell placement status: %w", err)
	}
	return domed, pierced, nil
}

// AddPocketCell records that a nest holds a pierced pocket cell (T-843).
func (r *PgRepository) AddPocketCell(ctx context.Context, nestID, cellID string) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO nest_pocket_cells (nest_id, cell_id) VALUES ($1, $2)
		 ON CONFLICT (nest_id, cell_id) DO NOTHING`, nestID, cellID)
	if err != nil {
		return fmt.Errorf("add pocket cell: %w", err)
	}
	return nil
}

// RefreshPocketCells pushes pierced_until forward for every pocket cell held by
// a LIVE nest (T-843, ADR-N3-8). Collapsed nests are skipped, so their cells
// stop being refreshed and expire on their own (self-healing). Set-based.
func (r *PgRepository) RefreshPocketCells(ctx context.Context, holdSeconds int) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE cells c SET pierced_until = now() + make_interval(secs => $1)
		FROM nest_pocket_cells npc
		JOIN nests n ON n.id = npc.nest_id AND n.collapsed_at IS NULL
		WHERE c.id = npc.cell_id`, holdSeconds)
	if err != nil {
		return 0, fmt.Errorf("refresh pocket cells: %w", err)
	}
	return tag.RowsAffected(), nil
}

// GetNearby returns live nests within radiusM of a point (T-849, for the
// attacker-side map: humans must be able to find nests to assault).
func (r *PgRepository) GetNearby(ctx context.Context, lat, lng, radiusM float64) ([]Nest, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+nestColumns+` FROM nests
		 WHERE collapsed_at IS NULL
		   AND ST_DWithin(geom::geography, ST_SetSRID(ST_MakePoint($2, $1), 4326)::geography, $3)`,
		lat, lng, radiusM)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	nests, err := pgx.CollectRows(rows, pgx.RowToStructByName[Nest])
	if err != nil {
		return nil, fmt.Errorf("scan nearby nests: %w", err)
	}
	for i := range nests {
		nests[i].decorate()
	}
	return nests, nil
}
