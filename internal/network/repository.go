package network

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository persists the beacon network (cores, domed cells). Links are NOT
// stored — recompute re-derives them from node positions every time
// (Perimeter model: the graph itself is never persisted).
type Repository interface {
	GetCore(ctx context.Context, playerID string) (*Core, error)
	CreateCore(ctx context.Context, c *Core) error
	UpdateCore(ctx context.Context, c *Core) error
	GetNodes(ctx context.Context, playerID string) ([]Node, error)
	PlayersWithNodesInRadius(ctx context.Context, lat, lng, radiusM float64) ([]string, error)
	ClearDomed(ctx context.Context, playerID string) error
	CountDomed(ctx context.Context, playerID string) (int, error)
	NearestCellID(ctx context.Context, lat, lng float64) (string, error)
	InsertDomedByDisks(ctx context.Context, playerID string, lngs, lats, radii []float64) (int, error)
	InsertDomedByPolygons(ctx context.Context, playerID string, rings [][]LatLng) (int, error)
	SpiritPressuredIDs(ctx context.Context, playerID string) ([]string, error)
}

// PgRepository is the PostgreSQL/PostGIS implementation.
type PgRepository struct {
	db *pgxpool.Pool
}

// NewPgRepository creates a network repository.
func NewPgRepository(db *pgxpool.Pool) *PgRepository {
	return &PgRepository{db: db}
}

func (r *PgRepository) GetCore(ctx context.Context, playerID string) (*Core, error) {
	const q = `SELECT id::text, player_id::text, cell_id, lat, lng, level, created_at, relocated_at
		FROM cores WHERE player_id = $1`
	var c Core
	err := r.db.QueryRow(ctx, q, playerID).Scan(
		&c.ID, &c.PlayerID, &c.CellID, &c.Lat, &c.Lng, &c.Level, &c.CreatedAt, &c.RelocatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get core: %w", err)
	}
	return &c, nil
}

func (r *PgRepository) CreateCore(ctx context.Context, c *Core) error {
	const q = `INSERT INTO cores (player_id, cell_id, lat, lng, geom, level)
		VALUES ($1, $2, $3, $4, ST_SetSRID(ST_MakePoint($4, $3), 4326), $5)
		RETURNING id::text, created_at`
	return r.db.QueryRow(ctx, q, c.PlayerID, c.CellID, c.Lat, c.Lng, c.Level).Scan(&c.ID, &c.CreatedAt)
}

func (r *PgRepository) UpdateCore(ctx context.Context, c *Core) error {
	const q = `UPDATE cores SET cell_id = $2, lat = $3, lng = $4,
		geom = ST_SetSRID(ST_MakePoint($4, $3), 4326), level = $5, relocated_at = NOW()
		WHERE id = $1`
	_, err := r.db.Exec(ctx, q, c.ID, c.CellID, c.Lat, c.Lng, c.Level)
	return err
}

func (r *PgRepository) GetNodes(ctx context.Context, playerID string) ([]Node, error) {
	const q = `
		SELECT id::text, lat, lng, true AS is_core, level, NULL::timestamptz AS legacy_since FROM cores WHERE player_id = $1
		UNION ALL
		SELECT id::text, lat, lng, false AS is_core, level, legacy_since FROM towers WHERE owner_id = $1`
	rows, err := r.db.Query(ctx, q, playerID)
	if err != nil {
		return nil, fmt.Errorf("get nodes: %w", err)
	}
	defer rows.Close()
	var nodes []Node
	for rows.Next() {
		var n Node
		if err := rows.Scan(&n.ID, &n.Lat, &n.Lng, &n.IsCore, &n.Level, &n.LegacySince); err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

// PlayersWithNodesInRadius returns the distinct ids of players who own a Core
// or tower within radiusM of (lat,lng). Drives the merged regional field
// overlay (GET /map/fields) — the caller resolves each player's powered disks.
func (r *PgRepository) PlayersWithNodesInRadius(ctx context.Context, lat, lng, radiusM float64) ([]string, error) {
	const q = `
		SELECT DISTINCT pid FROM (
			SELECT player_id::text AS pid FROM cores
			 WHERE ST_DWithin(geom::geography, ST_SetSRID(ST_MakePoint($2, $1), 4326)::geography, $3)
			UNION
			SELECT owner_id::text AS pid FROM towers
			 WHERE ST_DWithin(geom::geography, ST_SetSRID(ST_MakePoint($2, $1), 4326)::geography, $3)
		) q`
	rows, err := r.db.Query(ctx, q, lat, lng, radiusM)
	if err != nil {
		return nil, fmt.Errorf("players with nodes in radius: %w", err)
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

func (r *PgRepository) ClearDomed(ctx context.Context, playerID string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM domed_cells WHERE player_id = $1`, playerID)
	return err
}

// InsertDomedByDisks marks every cell inside the union of the supplied energy
// disks (lng[i], lat[i], radius[i]) as domed for the player (Perimeter field).
func (r *PgRepository) InsertDomedByDisks(ctx context.Context, playerID string, lngs, lats, radii []float64) (int, error) {
	if len(lngs) == 0 {
		return 0, nil
	}
	const q = `
		INSERT INTO domed_cells (player_id, cell_id, sealed)
		SELECT DISTINCT $1::uuid, c.id, false
		FROM cells c
		JOIN unnest($2::float8[], $3::float8[], $4::float8[]) AS n(lng, lat, r)
		  ON ST_DWithin(c.geom::geography, ST_SetSRID(ST_MakePoint(n.lng, n.lat), 4326)::geography, n.r)
		ON CONFLICT (player_id, cell_id) DO NOTHING`
	tag, err := r.db.Exec(ctx, q, playerID, lngs, lats, radii)
	if err != nil {
		return 0, fmt.Errorf("insert domed by disks: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// InsertDomedByPolygons marks every cell whose geometry is contained in any of
// the supplied closed rings (sealed-dome area, beacon_network_dome.md §6) as
// domed. Each ring is a simple polygon validated upstream by findContours, so
// ST_Contains is always well-defined. Returns the number of newly domed cells
// (ON CONFLICT dedupes against cells already domed by the disk field).
func (r *PgRepository) InsertDomedByPolygons(ctx context.Context, playerID string, rings [][]LatLng) (int, error) {
	const q = `
		INSERT INTO domed_cells (player_id, cell_id, sealed)
		SELECT DISTINCT $1::uuid, c.id, true
		FROM cells c
		WHERE ST_Contains(ST_GeomFromText($2, 4326), c.geom)
		ON CONFLICT (player_id, cell_id) DO UPDATE SET sealed = true`
	total := 0
	for _, ring := range rings {
		wkt := polygonWKT(ring)
		if wkt == "" {
			continue
		}
		tag, err := r.db.Exec(ctx, q, playerID, wkt)
		if err != nil {
			return total, fmt.Errorf("insert domed by polygon: %w", err)
		}
		total += int(tag.RowsAffected())
	}
	return total, nil
}

// polygonWKT renders a closed ring as a PostGIS POLYGON WKT (lng lat order,
// first vertex repeated to close the ring). Returns "" for under-specified
// rings.
func polygonWKT(ring []LatLng) string {
	if len(ring) < 3 {
		return ""
	}
	var b strings.Builder
	b.WriteString("POLYGON((")
	for i, v := range ring {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%.7f %.7f", v.Lng, v.Lat)
	}
	// close the ring
	fmt.Fprintf(&b, ", %.7f %.7f", ring[0].Lng, ring[0].Lat)
	b.WriteString("))")
	return b.String()
}

// SetTowerBrownout persists recompute's brownout verdict on the owner's
// towers so SQL consumers (infection suppression, income worker) can read it.
// An empty ids slice clears every flag. Off the Repository contract —
// consumed via an optional interface.
func (r *PgRepository) SetTowerBrownout(ctx context.Context, playerID string, brownoutIDs []string) error {
	if brownoutIDs == nil {
		brownoutIDs = []string{}
	}
	_, err := r.db.Exec(ctx,
		`UPDATE towers SET brownout = (id::text = ANY($2)) WHERE owner_id = $1`,
		playerID, brownoutIDs)
	if err != nil {
		return fmt.Errorf("set tower brownout: %w", err)
	}
	return nil
}

// SealedCellCountsByPlayer returns, per player, how many of their domed cells
// are sealed (closed-contour area). Drives dome area income (B3). Symbionts
// are excluded: their human-era network is legacy infrastructure and pays the
// ex-owner nothing (canon legacy_human_network.md).
func (r *PgRepository) SealedCellCountsByPlayer(ctx context.Context) (map[string]int, error) {
	const q = `SELECT dc.player_id::text, COUNT(*) FROM domed_cells dc
		WHERE dc.sealed = true
		  AND NOT EXISTS (
			SELECT 1 FROM player_factions pf
			WHERE pf.player_id = dc.player_id AND pf.faction = 'symbiont'
		  )
		GROUP BY dc.player_id`
	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("sealed cell counts: %w", err)
	}
	defer rows.Close()
	out := make(map[string]int)
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

func (r *PgRepository) CountDomed(ctx context.Context, playerID string) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM domed_cells WHERE player_id = $1`, playerID).Scan(&n)
	return n, err
}

// NearestCellID returns the id of the cell geographically closest to (lat,lng),
// using the GiST KNN operator. Used for "place where you stand" so the server
// chooses the cell from its own authoritative player position.
func (r *PgRepository) NearestCellID(ctx context.Context, lat, lng float64) (string, error) {
	var id string
	err := r.db.QueryRow(ctx,
		`SELECT id FROM cells ORDER BY geom <-> ST_SetSRID(ST_MakePoint($1, $2), 4326) LIMIT 1`,
		lng, lat).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("nearest cell: %w", err)
	}
	return id, nil
}

// SpiritPressuredIDs returns the player's tower ids currently under N2 spirit
// pressure (spirit_pressure_until > now()) — recompute drops these from the dome
// so infection creeps in around a spirit-pressured perimeter beacon (T-864).
func (r *PgRepository) SpiritPressuredIDs(ctx context.Context, playerID string) ([]string, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id::text FROM towers
		 WHERE owner_id = $1 AND spirit_pressure_until IS NOT NULL AND spirit_pressure_until > now()`,
		playerID)
	if err != nil {
		return nil, fmt.Errorf("spirit-pressured ids: %w", err)
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
