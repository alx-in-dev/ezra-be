package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OverpassClient queries the Overpass API for OSM features in a bbox.
// Used by cell.Seeder to populate osm_features and derive
// has_nearby_building / has_nearby_road flags for tower placement validation.
//
// Overpass is public and free; the main tradeoff is throughput (~1-2 complex
// queries per minute sustained) and accuracy of the OSM extract. For lazy
// per-region seeding that's perfect — one query per new 0.1° grid cell, then
// never again.
//
// The client tries Endpoints[0] first and falls back to the remaining ones
// on 5xx / timeout / network error. Public endpoints go under load, so
// having 2-3 mirrors is important for reliability.
type OverpassClient struct {
	Endpoints []string
	HTTPCli   *http.Client
}

// DefaultOverpassEndpoints is the public mirror list in preference order.
// Source: https://wiki.openstreetmap.org/wiki/Overpass_API#Public_Overpass_API_instances
var DefaultOverpassEndpoints = []string{
	"https://overpass-api.de/api/interpreter",
	"https://overpass.kumi.systems/api/interpreter",
	"https://lz4.overpass-api.de/api/interpreter",
}

// NewOverpassClient builds a client with the given endpoint list. If
// endpoints is empty, DefaultOverpassEndpoints is used.
func NewOverpassClient(endpoints []string) *OverpassClient {
	if len(endpoints) == 0 {
		endpoints = DefaultOverpassEndpoints
	}
	return &OverpassClient{
		Endpoints: endpoints,
		HTTPCli: &http.Client{
			Timeout: 90 * time.Second,
		},
	}
}

// Bbox is a geographic bounding box in decimal degrees (WGS84).
type Bbox struct {
	South, West, North, East float64
}

// OsmFeature is a single building or road extracted from Overpass, normalized
// to the shape the rest of the server expects.
//
// Geometry is the flattened ordered list of (lat, lng) coordinates.
//   - For buildings: the outer polygon ring (first + last point coincide).
//   - For roads: the polyline vertices in order.
//
// The Seeder converts this into PostGIS WKT at INSERT time.
type OsmFeature struct {
	OsmID     int64
	Type      string // "building" | "road"
	RoadClass string // only for roads: "motorway"/"primary"/"residential"/...
	Geometry  []LatLng
}

// LatLng is a single point in WGS84.
type LatLng struct {
	Lat float64
	Lng float64
}

// FetchBuildingsAndRoads runs one Overpass query for the given bbox and
// returns all buildings (any `building=*`) and roads with a power-source-
// plausible highway class. Internal paths, tracks, footways etc. are excluded
// because they don't imply infrastructure-backed electricity (decision 8.2).
func (c *OverpassClient) FetchBuildingsAndRoads(ctx context.Context, b Bbox) ([]OsmFeature, error) {
	// Highway classes that imply plausible electric infrastructure.
	// Excluded: path, track, footway, cycleway, steps, bridleway, busway,
	// raceway, construction, proposed.
	highwayFilter := strings.Join([]string{
		"motorway", "trunk", "primary", "secondary", "tertiary",
		"unclassified", "residential", "living_street", "service",
	}, "|")

	q := fmt.Sprintf(`[out:json][timeout:60];
(
  way["building"](%f,%f,%f,%f);
  way["highway"~"^(%s)$"](%f,%f,%f,%f);
);
out geom;`,
		b.South, b.West, b.North, b.East,
		highwayFilter,
		b.South, b.West, b.North, b.East,
	)

	body, err := c.postWithFallback(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("overpass post: %w", err)
	}

	return parseOverpassResponse(body)
}

// postWithFallback sends the query to each endpoint in order until one
// succeeds. Network errors and 5xx responses trigger a fallback; 4xx
// responses abort immediately (those are our bugs, not server issues).
func (c *OverpassClient) postWithFallback(ctx context.Context, query string) ([]byte, error) {
	var lastErr error
	for i, ep := range c.Endpoints {
		body, err := c.postOnce(ctx, ep, query)
		if err == nil {
			if i > 0 {
				slog.Info("overpass: recovered on fallback endpoint", "endpoint", ep, "attempt", i+1)
			}
			return body, nil
		}
		// Permanent error (4xx) — no point retrying other endpoints.
		if errors.Is(err, errOverpassPermanent) {
			return nil, err
		}
		slog.Warn("overpass endpoint failed, trying next", "endpoint", ep, "error", err)
		lastErr = err
	}
	return nil, fmt.Errorf("all overpass endpoints failed: %w", lastErr)
}

var errOverpassPermanent = errors.New("overpass permanent error (4xx)")

func (c *OverpassClient) postOnce(ctx context.Context, endpoint, query string) ([]byte, error) {
	form := url.Values{"data": {query}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Public Overpass instances reject requests with the default Go
	// net/http User-Agent — they require a "meaningful" UA to enable
	// per-app rate limiting and abuse control. Spec: https://wiki.
	// openstreetmap.org/wiki/Overpass_API#Public_Overpass_API_instances
	// Contact must be a real mailbox: Overpass operators will reach out
	// before throttling or banning a UA rather than silently ignore.
	req.Header.Set("User-Agent", "ezra-game-server/1.0 (contact: it.leha@gmail.com)")

	resp, err := c.HTTPCli.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, readErr
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return body, nil
	}
	// 429 Too Many Requests is transient — retry on another endpoint (or
	// the same one after backoff). Treating it as permanent aborts the
	// fallback chain, which is wrong.
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("rate limited (429) body=%s", truncate(body, 200))
	}
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		// Likely bad query syntax — same across all endpoints.
		return nil, fmt.Errorf("%w: status %d body=%s", errOverpassPermanent, resp.StatusCode, truncate(body, 200))
	}
	return nil, fmt.Errorf("transient status %d body=%s", resp.StatusCode, truncate(body, 200))
}

// overpassResponse is the minimal subset of the Overpass JSON schema we need.
// Ways have `id`, `tags`, and a `geometry` array of {lat, lon} nodes.
type overpassResponse struct {
	Elements []overpassElement `json:"elements"`
}

type overpassElement struct {
	Type     string           `json:"type"`
	ID       int64            `json:"id"`
	Tags     map[string]string `json:"tags"`
	Geometry []overpassGeomPoint   `json:"geometry"`
}

type overpassGeomPoint struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

func parseOverpassResponse(body []byte) ([]OsmFeature, error) {
	var raw overpassResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode overpass response: %w", err)
	}

	out := make([]OsmFeature, 0, len(raw.Elements))
	for _, el := range raw.Elements {
		if el.Type != "way" || len(el.Geometry) < 2 {
			continue
		}

		var featType, roadClass string
		if _, isBuilding := el.Tags["building"]; isBuilding {
			featType = "building"
		} else if hw, isHighway := el.Tags["highway"]; isHighway {
			featType = "road"
			roadClass = hw
		} else {
			continue
		}

		geom := make([]LatLng, len(el.Geometry))
		for i, p := range el.Geometry {
			geom[i] = LatLng{Lat: p.Lat, Lng: p.Lon}
		}

		out = append(out, OsmFeature{
			OsmID:     el.ID,
			Type:      featType,
			RoadClass: roadClass,
			Geometry:  geom,
		})
	}
	return out, nil
}

// BuildWKT returns the PostGIS WKT representation of a feature's geometry.
// Buildings become POLYGON (outer ring only), roads become LINESTRING.
//
// For buildings we rely on the OSM convention that `out geom;` returns the
// way's ordered node list; if the first and last nodes aren't equal we close
// the ring ourselves.
func (f OsmFeature) BuildWKT() string {
	if len(f.Geometry) == 0 {
		return ""
	}
	points := make([]string, 0, len(f.Geometry)+1)
	for _, p := range f.Geometry {
		points = append(points, fmt.Sprintf("%f %f", p.Lng, p.Lat))
	}
	if f.Type == "building" {
		// Close the ring if the way isn't already closed.
		if f.Geometry[0] != f.Geometry[len(f.Geometry)-1] {
			points = append(points, fmt.Sprintf("%f %f", f.Geometry[0].Lng, f.Geometry[0].Lat))
		}
		return "POLYGON((" + strings.Join(points, ", ") + "))"
	}
	return "LINESTRING(" + strings.Join(points, ", ") + ")"
}

func truncate(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n]) + "..."
	}
	return string(b)
}
