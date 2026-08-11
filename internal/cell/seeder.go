package cell

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/singleflight"

	"github.com/ezra-game/server/internal/platform"
)

// Seeder lazily populates a 0.1° grid region around a given point with OSM
// data from Overpass and the 50m cell grid. Idempotent: subsequent calls
// for the same region are O(1) no-ops thanks to the seeded_regions table.
//
// Thread-safety: concurrent EnsureRegion calls for the same region key are
// deduplicated via singleflight — only one goroutine actually talks to
// Overpass + writes rows, the rest wait and return its result.
type Seeder struct {
	db       *pgxpool.Pool
	overpass *platform.OverpassClient
	sf       singleflight.Group
}

// NewSeeder constructs a Seeder. The caller owns the db pool and overpass
// client lifetime.
func NewSeeder(db *pgxpool.Pool, overpass *platform.OverpassClient) *Seeder {
	return &Seeder{db: db, overpass: overpass}
}

const (
	// Fixed grid resolution: 0.1° in lat and lng. At Kaliningrad latitude
	// this gives ≈11km × 6km per region cell — close enough to the
	// requested "10km default area" and gives stable region keys.
	gridStepDeg = 0.1

	// Cell grid step inside a region (meters). Matches existing schema.
	cellStepM = 50.0

	// Tower placement power-source radius, must match
	// canon.TowerPlacementMaxDistanceM = 30m.
	powerRadiusM = 30.0

	// Initial infection range for freshly generated cells (decision 8.1).
	initialInfectionMin = 95.0
	initialInfectionMax = 100.0

	// Bulk insert batch size. PostgreSQL parameter limit is ~65k, our
	// cell insert has ~9 params per row, so 1000 rows per flush is safe.
	cellInsertBatchSize    = 1000
	featureInsertBatchSize = 500
)

// EnsureRegion seeds the 0.1° grid region containing (lat, lng) if it hasn't
// been seeded already. Safe to call from HTTP handlers — blocks the caller
// until the region is ready (or returns immediately if already seeded).
//
// First call per region: ~3-10 seconds (Overpass + SQL writes).
// Subsequent calls: O(1) cache check on seeded_regions.
//
// Uses a detached context for the actual seed work so a client disconnect
// doesn't abort a half-completed transaction. The caller still sees the
// original ctx's cancellation through the returned error if they cancel
// before seed completes, but the seed itself runs to commit.
func (s *Seeder) EnsureRegion(ctx context.Context, lat, lng float64) error {
	key := regionKey(lat, lng)

	_, err, _ := s.sf.Do(key, func() (any, error) {
		// Fast path: check seeded_regions. Most calls hit this after the
		// first player for a region has already triggered the seed.
		already, err := s.isSeeded(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("check seeded_regions: %w", err)
		}
		if already {
			return nil, nil
		}
		// Detach from the request context so a client disconnect doesn't
		// tear down an in-flight seed transaction. Cap at 2 minutes to
		// prevent runaway (Overpass timeout + tx).
		seedCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		defer cancel()

		// Retry on deadlock — the infection recalc worker runs concurrently
		// and can conflict with our bulk UPDATE cells on flag population.
		// PostgreSQL's deadlock resolution picks a victim transaction; we
		// retry the whole seedRegion (idempotent thanks to ON CONFLICT DO
		// NOTHING on feature and cell inserts). Exponential backoff.
		const maxAttempts = 4
		for attempt := 0; attempt < maxAttempts; attempt++ {
			if err = s.seedRegion(seedCtx, key, lat, lng); err == nil {
				return nil, nil
			}
			if !isDeadlockError(err) || attempt == maxAttempts-1 {
				return nil, err
			}
			backoff := time.Duration(100*(1<<attempt)) * time.Millisecond
			slog.Warn("seed region: deadlock, retrying",
				"key", key, "attempt", attempt+1, "backoff_ms", backoff.Milliseconds())
			select {
			case <-time.After(backoff):
			case <-seedCtx.Done():
				return nil, seedCtx.Err()
			}
		}
		return nil, err
	})
	return err
}

// isDeadlockError returns true for PostgreSQL SQLSTATE 40P01 (deadlock_detected).
// Transient: the victim transaction is aborted but can be retried safely.
func isDeadlockError(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "40P01"
	}
	return false
}

// regionKey maps any point to a stable region identifier. Two calls with
// (lat, lng) in the same 0.1° grid cell return the same key.
func regionKey(lat, lng float64) string {
	return fmt.Sprintf("grid_%d_%d",
		int(math.Floor(lat*10)),
		int(math.Floor(lng*10)))
}

// bboxForKey is the inverse of regionKey: returns the 0.1° square the key
// refers to.
func bboxForKey(key string) (platform.Bbox, error) {
	var latIdx, lngIdx int
	if _, err := fmt.Sscanf(key, "grid_%d_%d", &latIdx, &lngIdx); err != nil {
		return platform.Bbox{}, fmt.Errorf("invalid region key %q: %w", key, err)
	}
	south := float64(latIdx) / 10
	north := south + gridStepDeg
	west := float64(lngIdx) / 10
	east := west + gridStepDeg
	return platform.Bbox{South: south, West: west, North: north, East: east}, nil
}

func (s *Seeder) isSeeded(ctx context.Context, key string) (bool, error) {
	var found bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM seeded_regions WHERE key = $1)`, key).Scan(&found)
	return found, err
}

// seedRegion is the actual work: fetch OSM, insert features, generate cells,
// populate power-source flags, mark region as seeded. Runs in a single
// transaction so partial failures don't leave the region half-seeded.
func (s *Seeder) seedRegion(ctx context.Context, key string, lat, lng float64) error {
	bbox, err := bboxForKey(key)
	if err != nil {
		return err
	}

	started := time.Now()
	slog.Info("seed region: starting",
		"key", key,
		"bbox", fmt.Sprintf("[%f,%f,%f,%f]", bbox.South, bbox.West, bbox.North, bbox.East))

	// 1. Overpass query — single round-trip for the whole region.
	features, err := s.overpass.FetchBuildingsAndRoads(ctx, bbox)
	if err != nil {
		return fmt.Errorf("overpass fetch: %w", err)
	}
	slog.Info("seed region: overpass done",
		"key", key,
		"features", len(features),
		"elapsed_ms", time.Since(started).Milliseconds())

	// 2. Compute cell grid for the bbox.
	coords := computeCellGrid(bbox)

	// 3. Transaction: features → cells → flags → seeded_regions.
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := bulkInsertFeatures(ctx, tx, features); err != nil {
		return fmt.Errorf("insert features: %w", err)
	}

	if err := bulkInsertCells(ctx, tx, coords); err != nil {
		return fmt.Errorf("insert cells: %w", err)
	}

	// 4. Populate power-source flags and terrain classification via PostGIS
	// spatial join. has_nearby_* within 30m, terrain = tight 5m match.
	bboxWKT := fmt.Sprintf(
		"POLYGON((%f %f, %f %f, %f %f, %f %f, %f %f))",
		bbox.West, bbox.South,
		bbox.East, bbox.South,
		bbox.East, bbox.North,
		bbox.West, bbox.North,
		bbox.West, bbox.South,
	)
	// NOTE on distances: we use ST_DWithin in degrees (not meters via
	// ::geography) so the GiST index on osm_features.geom is actually hit.
	// Approximate conversion at Kaliningrad latitude: 30m ≈ 0.00045°. This
	// gives ±20m worst-case error on a 50m cell grid — acceptable for MVP
	// (tower placement has its own haversine check for player→cell).
	//
	// nearest_road_class is deliberately NOT populated here; it's a
	// cosmetic field and computing it required an ORDER BY ST_Distance
	// per cell (~100x slower). Deferred to a future batch job.
	const (
		powerRadiusDeg   = 0.00045 // ≈30m at 54° latitude
		terrainRadiusDeg = 0.00010 // ≈7m at 54° latitude (tight match for primary class)
	)
	_, err = tx.Exec(ctx, `
		UPDATE cells c SET
			has_nearby_building = EXISTS (
				SELECT 1 FROM osm_features f
				WHERE f.type = 'building'
				  AND ST_DWithin(f.geom, c.geom, $1)
			),
			has_nearby_road = EXISTS (
				SELECT 1 FROM osm_features f
				WHERE f.type = 'road'
				  AND ST_DWithin(f.geom, c.geom, $1)
			),
			terrain = CASE
				WHEN EXISTS (
					SELECT 1 FROM osm_features f
					WHERE f.type = 'building'
					  AND ST_DWithin(f.geom, c.geom, $2)
				) THEN 'building'
				WHEN EXISTS (
					SELECT 1 FROM osm_features f
					WHERE f.type = 'road'
					  AND ST_DWithin(f.geom, c.geom, $2)
				) THEN 'road'
				ELSE 'open'
			END
		WHERE ST_Within(c.geom, ST_GeomFromText($3, 4326))
	`, powerRadiusDeg, terrainRadiusDeg, bboxWKT)
	if err != nil {
		return fmt.Errorf("update cell flags: %w", err)
	}

	// 5. Mark region as seeded so subsequent requests fast-path.
	_, err = tx.Exec(ctx, `
		INSERT INTO seeded_regions (key, center_lat, center_lng, bbox, cell_count, feature_count)
		VALUES ($1, $2, $3, ST_GeomFromText($4, 4326), $5, $6)
		ON CONFLICT (key) DO UPDATE SET
			bbox = EXCLUDED.bbox,
			cell_count = EXCLUDED.cell_count,
			feature_count = EXCLUDED.feature_count,
			seeded_at = NOW()
	`,
		key,
		(bbox.South+bbox.North)/2, (bbox.West+bbox.East)/2,
		bboxWKT,
		len(coords),
		len(features),
	)
	if err != nil {
		return fmt.Errorf("mark seeded: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	slog.Info("seed region: done",
		"key", key,
		"cells", len(coords),
		"features", len(features),
		"elapsed_ms", time.Since(started).Milliseconds())
	return nil
}

// gridCoord is one cell in the 50m grid we're about to INSERT.
//   - swLat, swLng: south-west corner of the 50m cell, used as the stable
//     ID derivation source.
//   - centerLat, centerLng: geographic center of the cell (swLat + halfStep,
//     swLng + halfStep). This is what we store in cells.lat/lng and use for
//     haversine checks, rendering, and any "where is this cell" query.
// Decision: cell (lat, lng) semantic = CENTER (C-02 from ai/review.md
// 2026-04-11). The ID keeps SW-corner coords so it doesn't drift if the
// center semantic changes.
type gridCoord struct {
	id        string
	swLat     float64
	swLng     float64
	centerLat float64
	centerLng float64
}

// computeCellGrid enumerates all cell coordinates inside a bbox at the fixed
// 50m step. At Kaliningrad latitude a 0.1° × 0.1° bbox yields ~222 × 128 =
// ~28k cells.
//
// Grid anchor is GLOBAL (math.Floor of lat/stepLat from 0), not per-region
// (W-02 from ai/review.md). This way adjacent regions produce grids that
// align on the shared boundary — no ~25m gap between them. stepLng is
// still computed from the region mid-latitude because cos(lat) changes
// slowly and a globally-fixed stepLng would elongate cells at high
// latitudes; the practical drift across a 0.1° region is <0.03% in our
// latitude range.
func computeCellGrid(b platform.Bbox) []gridCoord {
	latMid := (b.South + b.North) / 2
	stepLat := cellStepM / 111000.0
	stepLng := cellStepM / (111000.0 * math.Cos(latMid*math.Pi/180))

	// Anchor to multiples of stepLat/stepLng from zero (global origin).
	// Use Floor + skip-below rather than Ceil so every region inside the
	// same latitude band lands on the same cell IDs.
	firstLatIdx := int(math.Floor(b.South / stepLat))
	firstLngIdx := int(math.Floor(b.West / stepLng))
	startLat := float64(firstLatIdx) * stepLat
	startLng := float64(firstLngIdx) * stepLng

	halfStepLat := stepLat / 2
	halfStepLng := stepLng / 2

	var out []gridCoord
	for lat := startLat; lat < b.North; lat += stepLat {
		if lat < b.South {
			continue // anchored grid start may fall below bbox, skip
		}
		for lng := startLng; lng < b.East; lng += stepLng {
			if lng < b.West {
				continue
			}
			out = append(out, gridCoord{
				id:        fmt.Sprintf("%.5f_%.5f", lat, lng),
				swLat:     lat,
				swLng:     lng,
				centerLat: lat + halfStepLat,
				centerLng: lng + halfStepLng,
			})
		}
	}
	return out
}

// bulkInsertFeatures inserts OSM features in batches. Uses WKT geometry +
// ST_GeomFromText so we can stream via the query builder. ON CONFLICT DO
// NOTHING handles the case where neighbouring regions overlap on OSM ways.
//
// NOTE: parameter numbering is driven by `placed`, not the loop index `i`.
// If a feature produces an empty WKT (skipped) we don't increment `placed`,
// so the $N placeholders and the args slice stay in sync. Using `i` would
// leave gaps in parameter numbering and desync the args count, which is a
// silent landmine if the input ever starts producing empty geometries.
func bulkInsertFeatures(ctx context.Context, tx pgx.Tx, features []platform.OsmFeature) error {
	for start := 0; start < len(features); start += featureInsertBatchSize {
		end := start + featureInsertBatchSize
		if end > len(features) {
			end = len(features)
		}
		batch := features[start:end]

		var sb strings.Builder
		sb.WriteString("INSERT INTO osm_features (osm_id, type, road_class, geom) VALUES ")
		args := make([]any, 0, len(batch)*4)
		placed := 0
		for _, f := range batch {
			wkt := f.BuildWKT()
			if wkt == "" {
				continue
			}
			if placed > 0 {
				sb.WriteString(",")
			}
			base := placed * 4
			fmt.Fprintf(&sb, "($%d,$%d,$%d,ST_GeomFromText($%d,4326))",
				base+1, base+2, base+3, base+4)
			var roadClass any
			if f.RoadClass != "" {
				roadClass = f.RoadClass
			}
			args = append(args, f.OsmID, f.Type, roadClass, wkt)
			placed++
		}
		if placed == 0 {
			// Nothing to insert in this batch — all geometries were empty.
			continue
		}
		sb.WriteString(" ON CONFLICT (osm_id, type) DO NOTHING")

		if _, err := tx.Exec(ctx, sb.String(), args...); err != nil {
			return fmt.Errorf("batch [%d..%d]: %w", start, end, err)
		}
	}
	return nil
}

// bulkInsertCells creates missing cells with initial infection in rupture band.
// Existing cells keep their infection/tower_id/rift_id — the later UPDATE in
// seedRegion refreshes only the Mapbox-derived fields (terrain + flags).
//
// Uses parameterized placeholders instead of string interpolation. All
// current fields are numeric / hash-derived strings and can't be attacked,
// but keeping the SQL construction safe-by-default prevents a regression
// if a user-controlled column (owner_name, note) is added later.
func bulkInsertCells(ctx context.Context, tx pgx.Tx, coords []gridCoord) error {
	for start := 0; start < len(coords); start += cellInsertBatchSize {
		end := start + cellInsertBatchSize
		if end > len(coords) {
			end = len(coords)
		}
		batch := coords[start:end]

		var sb strings.Builder
		sb.WriteString("INSERT INTO cells (id, lat, lng, infection, terrain, geom) VALUES ")
		// 4 args per row: id, centerLat, centerLng, infection.
		// ST_MakePoint reuses $centerLat/$centerLng indices, no extra args.
		args := make([]any, 0, len(batch)*4)
		for i, c := range batch {
			if i > 0 {
				sb.WriteString(",")
			}
			infection := initialInfectionMin + rand.Float64()*(initialInfectionMax-initialInfectionMin)
			base := i * 4
			fmt.Fprintf(&sb,
				"($%d, $%d, $%d, $%d, 'open', ST_SetSRID(ST_MakePoint($%d, $%d), 4326))",
				base+1, base+2, base+3, base+4, base+3, base+2)
			// cells.lat/lng store the GEOGRAPHIC CENTER of the 50m cell
			// (decision C-02), NOT the SW corner. Haversine and overlay
			// rendering both expect center semantics.
			args = append(args, c.id, c.centerLat, c.centerLng, infection)
		}
		sb.WriteString(" ON CONFLICT (id) DO NOTHING")

		if _, err := tx.Exec(ctx, sb.String(), args...); err != nil {
			return fmt.Errorf("batch [%d..%d]: %w", start, end, err)
		}
	}
	return nil
}
