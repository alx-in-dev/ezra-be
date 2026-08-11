package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// MapboxClient wraps the Mapbox Tilequery API for reading vector tile
// features at a given coordinate. Used by the seed script to classify cells
// as "has nearby building / road" for tower placement validation.
//
// Docs: https://docs.mapbox.com/api/maps/tilequery/
type MapboxClient struct {
	Token    string
	Tileset  string // default: "mapbox.mapbox-streets-v8"
	HTTPCli  *http.Client
	BaseURL  string // default: "https://api.mapbox.com"
}

// NewMapboxClient constructs a client with sensible defaults. Caller must
// ensure Token is set — an empty token will produce 401 on every request.
func NewMapboxClient(token string) *MapboxClient {
	return &MapboxClient{
		Token:   token,
		Tileset: "mapbox.mapbox-streets-v8",
		HTTPCli: &http.Client{
			Timeout: 15 * time.Second,
		},
		BaseURL: "https://api.mapbox.com",
	}
}

// TilequeryResult is the aggregated output of one Tilequery call, reduced to
// the flags that tower placement validation needs.
type TilequeryResult struct {
	HasNearbyBuilding bool
	HasNearbyRoad     bool
	NearestBuildingM  float64 // +Inf if no building found within radius
	NearestRoadM      float64 // +Inf if no road found within radius
	NearestRoadClass  string  // e.g. "residential", "primary"; empty if no road
}

// rawFeature is the minimal shape we need from the Mapbox Tilequery response.
// The full response has geometry + many properties; we only care about
// properties.tilequery.{distance, layer} and properties.class.
type rawFeature struct {
	Properties struct {
		Tilequery struct {
			Distance float64 `json:"distance"`
			Layer    string  `json:"layer"`
			Geometry string  `json:"geometry"`
		} `json:"tilequery"`
		Class string `json:"class"` // for roads: motorway/primary/residential/...
	} `json:"properties"`
}

type rawResponse struct {
	Type     string       `json:"type"`
	Features []rawFeature `json:"features"`
}

// TilequeryAt queries the Mapbox Tilequery API at the given lat/lng and returns
// aggregated flags for tower placement validation. Queries layers "building"
// and "road" simultaneously within `radiusM` meters.
//
// Mapbox free tier: 100k requests/month, 600 requests/minute. Seed should
// pace itself to avoid hitting the per-minute cap.
func (c *MapboxClient) TilequeryAt(ctx context.Context, lat, lng, radiusM float64) (*TilequeryResult, error) {
	if c.Token == "" {
		return nil, fmt.Errorf("mapbox token is not set (MAPBOX_TOKEN env)")
	}

	u := fmt.Sprintf("%s/v4/%s/tilequery/%.6f,%.6f.json", c.BaseURL, c.Tileset, lng, lat)
	q := url.Values{}
	q.Set("radius", fmt.Sprintf("%.0f", radiusM))
	q.Set("limit", "20")
	q.Set("layers", "building,road")
	q.Set("access_token", c.Token)
	u = u + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("build tilequery request: %w", err)
	}

	resp, err := c.HTTPCli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tilequery http call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tilequery http status %d", resp.StatusCode)
	}

	var body rawResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("tilequery decode: %w", err)
	}

	result := &TilequeryResult{
		NearestBuildingM: float64(1e9), // sentinel for "not found"
		NearestRoadM:     float64(1e9),
	}

	for _, f := range body.Features {
		tq := f.Properties.Tilequery
		switch tq.Layer {
		case "building":
			result.HasNearbyBuilding = true
			if tq.Distance < result.NearestBuildingM {
				result.NearestBuildingM = tq.Distance
			}
		case "road":
			result.HasNearbyRoad = true
			if tq.Distance < result.NearestRoadM {
				result.NearestRoadM = tq.Distance
				if f.Properties.Class != "" {
					result.NearestRoadClass = f.Properties.Class
				}
			}
		}
	}

	return result, nil
}
