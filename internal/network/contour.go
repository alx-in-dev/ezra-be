package network

import (
	"math"
	"sort"

	"github.com/ezra-game/server/internal/canon"
	"github.com/ezra-game/server/pkg/geo"
)

// MinContourAreaM2 is the smallest enclosed area (m²) that counts as a sealed
// dome. It rejects degenerate "rings" — three collinear relay beacons enclose
// ~0 m², which is a line, not a dome. Tunable per beacon_network_dome.md §13
// ("минимальное число узлов / макс. длина ребра кольца").
const MinContourAreaM2 = 1.0

// findContours returns the simple polygons enclosed by the player's POWERED
// beacon network — the sealed domes from beacon_network_dome.md §6.
//
// connectNodes already produced a spanning tree (parent) rooted at the Core, so
// every proximity edge between two powered nodes that is NOT a tree edge is a
// "chord" closing exactly one fundamental cycle: the chord plus the unique tree
// path between its endpoints. Each fundamental cycle is a ring → polygon. This
// is bounded (at most E−V+1 cycles) and needs no general cycle enumeration.
//
// Degenerate (near-zero area, e.g. collinear relay lines) and self-intersecting
// rings are dropped so the PostGIS ST_Contains downstream only ever sees simple
// polygons.
func findContours(nodes []Node, powered map[string]bool, depth map[string]int, parent map[string]parentEdge) [][]LatLng {
	pos := make(map[string]Node)
	ids := make([]string, 0, len(nodes))
	for _, n := range nodes {
		if powered[n.ID] {
			pos[n.ID] = n
			ids = append(ids, n.ID)
		}
	}
	sort.Strings(ids) // deterministic chord iteration → stable output

	treeEdge := make(map[[2]string]bool, len(parent))
	for child, pe := range parent {
		treeEdge[edgeKey(child, pe.id)] = true
	}

	seen := make(map[string]bool)
	var contours [][]LatLng
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			u, v := ids[i], ids[j]
			if treeEdge[edgeKey(u, v)] {
				continue // already a network link, not a chord
			}
			nu, nv := pos[u], pos[v]
			d := geo.Haversine(nu.Lat, nu.Lng, nv.Lat, nv.Lng)
			reach := math.Max(
				canon.NodeConnectionRadius(nu.IsCore, nu.Level),
				canon.NodeConnectionRadius(nv.IsCore, nv.Level),
			)
			if d >= reach {
				continue // too far to be a link → no ring through here
			}

			ringIDs := fundamentalCycle(u, v, depth, parent)
			if len(ringIDs) < 3 {
				continue
			}
			key := cycleKey(ringIDs)
			if seen[key] {
				continue
			}
			seen[key] = true

			poly := make([]LatLng, 0, len(ringIDs))
			for _, id := range ringIDs {
				n := pos[id]
				poly = append(poly, LatLng{Lat: n.Lat, Lng: n.Lng})
			}
			if polygonAreaM2(poly) < MinContourAreaM2 {
				continue // collinear / degenerate: a line, not a dome
			}
			if !isSimplePolygon(poly) {
				continue // self-intersecting: ST_Contains would be invalid
			}
			contours = append(contours, poly)
		}
	}
	return contours
}

// fundamentalCycle returns the node-id ring of the cycle closed by the chord
// (u,v): the tree path u→LCA→v. The chord itself (v back to u) closes the ring.
func fundamentalCycle(u, v string, depth map[string]int, parent map[string]parentEdge) []string {
	a, b := u, v
	var up, vp []string
	for depth[a] > depth[b] {
		up = append(up, a)
		a = parent[a].id
	}
	for depth[b] > depth[a] {
		vp = append(vp, b)
		b = parent[b].id
	}
	for a != b {
		up = append(up, a)
		a = parent[a].id
		vp = append(vp, b)
		b = parent[b].id
	}
	// a == b == LCA. Ring: u..(below LCA), LCA, then v's side reversed.
	ring := make([]string, 0, len(up)+len(vp)+1)
	ring = append(ring, up...)
	ring = append(ring, a)
	for k := len(vp) - 1; k >= 0; k-- {
		ring = append(ring, vp[k])
	}
	return ring
}

// edgeKey returns an unordered key for a pair of node ids.
func edgeKey(a, b string) [2]string {
	if a > b {
		a, b = b, a
	}
	return [2]string{a, b}
}

// cycleKey returns an order-independent key for a set of ring node ids so the
// same ring discovered from different chords is only domed once.
func cycleKey(ids []string) string {
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)
	out := ""
	for _, id := range sorted {
		out += id + "|"
	}
	return out
}

// polygonAreaM2 returns the absolute area of a lat/lng ring in square metres
// using an equirectangular projection around the ring centroid — accurate
// enough at city scale to size and rank domes.
func polygonAreaM2(p []LatLng) float64 {
	if len(p) < 3 {
		return 0
	}
	var latSum float64
	for _, v := range p {
		latSum += v.Lat
	}
	latRef := latSum / float64(len(p))
	const mPerDegLat = 111320.0
	mPerDegLng := mPerDegLat * math.Cos(latRef*math.Pi/180)

	x := func(v LatLng) float64 { return v.Lng * mPerDegLng }
	y := func(v LatLng) float64 { return v.Lat * mPerDegLat }

	var area float64
	for i := 0; i < len(p); i++ {
		j := (i + 1) % len(p)
		area += x(p[i])*y(p[j]) - x(p[j])*y(p[i])
	}
	return math.Abs(area) / 2
}

// isSimplePolygon reports whether the closed ring has no self-intersections
// (no two non-adjacent edges cross). Treats lat/lng as planar (fine at city
// scale).
func isSimplePolygon(p []LatLng) bool {
	n := len(p)
	if n < 3 {
		return false
	}
	for i := 0; i < n; i++ {
		a1, a2 := p[i], p[(i+1)%n]
		for j := i + 1; j < n; j++ {
			// skip adjacent edges (they share a vertex)
			if j == i {
				continue
			}
			if (j+1)%n == i || j == (i+1)%n {
				continue
			}
			b1, b2 := p[j], p[(j+1)%n]
			if segmentsIntersect(a1, a2, b1, b2) {
				return false
			}
		}
	}
	return true
}

func segmentsIntersect(p1, p2, p3, p4 LatLng) bool {
	d1 := orientation(p3, p4, p1)
	d2 := orientation(p3, p4, p2)
	d3 := orientation(p1, p2, p3)
	d4 := orientation(p1, p2, p4)
	if ((d1 > 0 && d2 < 0) || (d1 < 0 && d2 > 0)) &&
		((d3 > 0 && d4 < 0) || (d3 < 0 && d4 > 0)) {
		return true
	}
	return false
}

// orientation returns the signed cross product (a→b) × (a→c); >0 ccw, <0 cw,
// 0 collinear.
func orientation(a, b, c LatLng) float64 {
	return (b.Lng-a.Lng)*(c.Lat-a.Lat) - (b.Lat-a.Lat)*(c.Lng-a.Lng)
}

// largestContour returns the polygon with the greatest area (the dome the
// client should outline), or nil when there are none.
func largestContour(contours [][]LatLng) []LatLng {
	var best []LatLng
	var bestArea float64
	for _, c := range contours {
		if a := polygonAreaM2(c); a > bestArea {
			bestArea, best = a, c
		}
	}
	return best
}
