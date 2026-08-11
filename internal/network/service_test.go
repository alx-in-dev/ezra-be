package network

import (
	"context"
	"testing"
	"time"

	"github.com/ezra-game/server/internal/canon"
)

// fakeRepo is an in-memory Repository for testing the graph logic without a DB.
type fakeRepo struct {
	core        *Core
	nodes       []Node
	cleared     bool
	inserted    bool
	domedCount  int
	polyCount   int
	polyRings   [][]LatLng
	polyInvoked bool
}

func (f *fakeRepo) GetCore(_ context.Context, _ string) (*Core, error)            { return f.core, nil }
func (f *fakeRepo) CreateCore(_ context.Context, _ *Core) error                   { return nil }
func (f *fakeRepo) UpdateCore(_ context.Context, _ *Core) error                   { return nil }
func (f *fakeRepo) GetNodes(_ context.Context, _ string) ([]Node, error)          { return f.nodes, nil }
func (f *fakeRepo) ClearDomed(_ context.Context, _ string) error                  { f.cleared = true; return nil }
func (f *fakeRepo) CountDomed(_ context.Context, _ string) (int, error)           { return f.domedCount, nil }
func (f *fakeRepo) NearestCellID(_ context.Context, _, _ float64) (string, error) { return "", nil }
func (f *fakeRepo) PlayersWithNodesInRadius(_ context.Context, _, _, _ float64) ([]string, error) {
	return nil, nil
}
func (f *fakeRepo) InsertDomedByDisks(_ context.Context, _ string, _, _, _ []float64) (int, error) {
	f.inserted = true
	return f.domedCount, nil
}
func (f *fakeRepo) InsertDomedByPolygons(_ context.Context, _ string, rings [][]LatLng) (int, error) {
	f.polyInvoked = true
	f.polyRings = rings
	return f.polyCount, nil
}

func newSvc(r Repository) *Service { return NewService(r, nil, nil, nil, nil) }

// The first Core relocation is free (RelocatedAt nil) and every later one costs
// crystals; GetNetwork surfaces the next cost so the client can confirm.
func TestNextRelocationCost(t *testing.T) {
	ctx := context.Background()

	free := &fakeRepo{core: &Core{ID: "c1", PlayerID: "p1", Level: 1}}
	st, err := newSvc(free).GetNetwork(ctx, "p1")
	if err != nil {
		t.Fatalf("GetNetwork (free): %v", err)
	}
	if st.NextRelocationCost != 0 {
		t.Fatalf("first relocation must be free, got cost %d", st.NextRelocationCost)
	}

	moved := time.Now()
	paid := &fakeRepo{core: &Core{ID: "c1", PlayerID: "p1", Level: 1, RelocatedAt: &moved}}
	st, err = newSvc(paid).GetNetwork(ctx, "p1")
	if err != nil {
		t.Fatalf("GetNetwork (paid): %v", err)
	}
	if st.NextRelocationCost != canon.CoreRelocationCostCrystals {
		t.Fatalf("after first move want cost %d, got %d",
			canon.CoreRelocationCostCrystals, st.NextRelocationCost)
	}
}

// fakeSeeder records on-demand region seeding requests.
type fakeSeeder struct {
	calls            int
	lastLat, lastLng float64
}

func (f *fakeSeeder) EnsureRegion(_ context.Context, lat, lng float64) error {
	f.calls++
	f.lastLat, f.lastLng = lat, lng
	return nil
}

func TestNearestCellID_SeedsRegionFirst(t *testing.T) {
	repo := &fakeRepo{}
	seeder := &fakeSeeder{}
	svc := NewService(repo, nil, nil, nil, seeder)

	if _, err := svc.NearestCellID(context.Background(), 53.2, 50.18); err != nil {
		t.Fatalf("NearestCellID: %v", err)
	}
	if seeder.calls != 1 {
		t.Fatalf("expected region seeded once before lookup, got %d calls", seeder.calls)
	}
	if seeder.lastLat != 53.2 || seeder.lastLng != 50.18 {
		t.Errorf("seeded wrong point: %f,%f", seeder.lastLat, seeder.lastLng)
	}
}

func TestRecompute_NoCore_NoDome(t *testing.T) {
	repo := &fakeRepo{core: nil, nodes: []Node{{ID: "a", Lat: 1, Lng: 1}}}
	st, err := newSvc(repo).GetNetwork(context.Background(), "p1")
	if err != nil {
		t.Fatalf("GetNetwork: %v", err)
	}
	if st.DomeSealed {
		t.Errorf("expected no dome without a core")
	}
	if !repo.cleared {
		t.Errorf("expected domed cells cleared when no core")
	}
	if st.Nodes[0].Powered {
		t.Errorf("nodes should be unpowered without a core")
	}
}

func TestRecompute_OverlappingDisks_FormField(t *testing.T) {
	// core + 2 beacons ~55m away (disks radii 140/90 overlap) → all powered,
	// energy field (domed cells) persisted.
	repo := &fakeRepo{
		core:       &Core{ID: "core", PlayerID: "p1", Lat: 0, Lng: 0, Level: 1},
		nodes:      []Node{{ID: "core", IsCore: true, Lat: 0, Lng: 0}, {ID: "a", Lat: 0.0005, Lng: 0}, {ID: "b", Lat: 0, Lng: 0.0005}},
		domedCount: 6,
	}
	st, err := newSvc(repo).GetNetwork(context.Background(), "p1")
	if err != nil {
		t.Fatalf("GetNetwork: %v", err)
	}
	for _, n := range st.Nodes {
		if !n.Powered {
			t.Errorf("overlapping node %s should be powered", n.ID)
		}
		if n.RadiusM <= 0 {
			t.Errorf("node %s should report an energy radius", n.ID)
		}
	}
	if !repo.inserted {
		t.Errorf("domed disks should be persisted")
	}
	if !st.DomeSealed || st.DomedCellCount != 6 {
		t.Errorf("expected active field with 6 domed cells, got sealed=%v count=%d", st.DomeSealed, st.DomedCellCount)
	}
}

func TestRecompute_FarBeacon_NotPowered(t *testing.T) {
	// A beacon ~1.1km away: its disk does not overlap the Core's → unpowered.
	repo := &fakeRepo{
		core:  &Core{ID: "core", PlayerID: "p1", Lat: 0, Lng: 0, Level: 1},
		nodes: []Node{{ID: "core", IsCore: true, Lat: 0, Lng: 0}, {ID: "far", Lat: 0.01, Lng: 0}},
	}
	st, err := newSvc(repo).GetNetwork(context.Background(), "p1")
	if err != nil {
		t.Fatalf("GetNetwork: %v", err)
	}
	for _, n := range st.Nodes {
		if n.ID == "far" && n.Powered {
			t.Errorf("far beacon must be unpowered (disks don't overlap)")
		}
		if n.ID == "core" && !n.Powered {
			t.Errorf("core must be powered")
		}
	}
}

func TestRecompute_PerimeterLinks_ClosestParentChain(t *testing.T) {
	// core(0,0) — a ~166m north (overlaps core: 140+90=230m) — b ~111m beyond a
	// (overlaps a: 90+90=180m, but NOT core: 277m > 230m). Perimeter rule:
	// a hangs off the core, b hangs off a → a clean 2-edge chain, no mesh.
	repo := &fakeRepo{
		core: &Core{ID: "core", PlayerID: "p1", Lat: 0, Lng: 0, Level: 1},
		nodes: []Node{
			{ID: "core", IsCore: true, Lat: 0, Lng: 0},
			{ID: "a", Lat: 0.0015, Lng: 0},
			{ID: "b", Lat: 0.0025, Lng: 0},
		},
	}
	st, err := newSvc(repo).GetNetwork(context.Background(), "p1")
	if err != nil {
		t.Fatalf("GetNetwork: %v", err)
	}
	if len(st.Links) != 2 {
		t.Fatalf("expected 2 derived links (tree), got %d: %+v", len(st.Links), st.Links)
	}
	want := map[string]string{"a": "core", "b": "a"} // ToID → FromID
	for _, l := range st.Links {
		if want[l.ToID] != l.FromID {
			t.Errorf("link %s should hang off %s, got %s", l.ToID, want[l.ToID], l.FromID)
		}
		if l.LengthM <= 0 || l.Cost <= 0 {
			t.Errorf("link %s→%s must carry length and cost, got %v/%v", l.FromID, l.ToID, l.LengthM, l.Cost)
		}
	}
}

func TestRecompute_ConnectionRadius_GrowsWithLevel(t *testing.T) {
	// b1 sits ~166m from the L1 core (reach 250m → connected). b2 sits ~333m
	// beyond b1: an L1 beacon (reach 200m) cannot pull it, an L3 one
	// (reach 450m) can — upgrading a frontier beacon extends the network.
	nodes := func(b1Level int) []Node {
		return []Node{
			{ID: "core", IsCore: true, Lat: 0, Lng: 0, Level: 1},
			{ID: "b1", Lat: 0.0015, Lng: 0, Level: b1Level},
			{ID: "b2", Lat: 0.0045, Lng: 0, Level: 1},
		}
	}

	for _, tc := range []struct {
		level   int
		powered bool
	}{
		{1, false},
		{3, true},
	} {
		repo := &fakeRepo{
			core:  &Core{ID: "core", PlayerID: "p1", Lat: 0, Lng: 0, Level: 1},
			nodes: nodes(tc.level),
		}
		st, err := newSvc(repo).GetNetwork(context.Background(), "p1")
		if err != nil {
			t.Fatalf("GetNetwork: %v", err)
		}
		for _, n := range st.Nodes {
			if n.ID == "b2" && n.Powered != tc.powered {
				t.Errorf("b1 level %d: b2 powered = %v, want %v", tc.level, n.Powered, tc.powered)
			}
		}
	}
}

func TestRecompute_ClosedRing_SealsDome(t *testing.T) {
	// A ~150m square (core + 3 beacons, all non-collinear and within reach):
	// the network encloses a block → sealed dome with an area polygon.
	repo := &fakeRepo{
		core: &Core{ID: "core", PlayerID: "p1", Lat: 0, Lng: 0, Level: 1},
		nodes: []Node{
			{ID: "core", IsCore: true, Lat: 0, Lng: 0, Level: 1},
			{ID: "a", Lat: 0.00135, Lng: 0, Level: 1},
			{ID: "b", Lat: 0.00135, Lng: 0.00135, Level: 1},
			{ID: "c", Lat: 0, Lng: 0.00135, Level: 1},
		},
		polyCount: 12,
	}
	st, err := newSvc(repo).GetNetwork(context.Background(), "p1")
	if err != nil {
		t.Fatalf("GetNetwork: %v", err)
	}
	if !st.DomeSealed {
		t.Errorf("a closed ring must seal the dome")
	}
	if !repo.polyInvoked || len(repo.polyRings) == 0 {
		t.Errorf("sealed dome must persist area polygons, invoked=%v rings=%d", repo.polyInvoked, len(repo.polyRings))
	}
	if len(st.DomePolygon) < 3 {
		t.Errorf("sealed dome must expose an outline polygon, got %d vertices", len(st.DomePolygon))
	}
	// domed count = disks (0 here) + polygon area cells (12 from fake)
	if st.DomedCellCount != 12 {
		t.Errorf("expected 12 domed cells (area), got %d", st.DomedCellCount)
	}
}

func TestRecompute_CollinearRelay_NoSeal(t *testing.T) {
	// Three collinear nodes (a relay line, not a ring) enclose ~0 m² → no dome.
	repo := &fakeRepo{
		core: &Core{ID: "core", PlayerID: "p1", Lat: 0, Lng: 0, Level: 1},
		nodes: []Node{
			{ID: "core", IsCore: true, Lat: 0, Lng: 0, Level: 1},
			{ID: "a", Lat: 0.0015, Lng: 0, Level: 1},
			{ID: "b", Lat: 0.0025, Lng: 0, Level: 1},
		},
	}
	st, err := newSvc(repo).GetNetwork(context.Background(), "p1")
	if err != nil {
		t.Fatalf("GetNetwork: %v", err)
	}
	if st.DomeSealed {
		t.Errorf("a collinear relay line must not seal a dome")
	}
	if repo.polyInvoked {
		t.Errorf("no contours → InsertDomedByPolygons must not be called")
	}
	if len(st.DomePolygon) != 0 {
		t.Errorf("expected no dome polygon for a relay line")
	}
}

func TestFindContours_Square(t *testing.T) {
	nodes := []Node{
		{ID: "core", IsCore: true, Lat: 0, Lng: 0, Level: 1},
		{ID: "a", Lat: 0.00135, Lng: 0, Level: 1},
		{ID: "b", Lat: 0.00135, Lng: 0.00135, Level: 1},
		{ID: "c", Lat: 0, Lng: 0.00135, Level: 1},
	}
	powered, depth, parent := connectNodes(nodes, "core")
	contours := findContours(nodes, powered, depth, parent)
	if len(contours) == 0 {
		t.Fatalf("expected at least one contour for a square network")
	}
	if a := polygonAreaM2(largestContour(contours)); a < MinContourAreaM2 {
		t.Errorf("largest contour area should be a real block, got %.1f m²", a)
	}
}

func TestRecompute_Brownout_WhenOverEcap(t *testing.T) {
	// Core L1 Ecap=100; 12 beacons in a chain → upkeep 120 > 100 → brownout.
	nodes := []Node{{ID: "core", IsCore: true}}
	for i := 1; i <= 12; i++ {
		id := string(rune('a' + i - 1)) // a..l
		nodes = append(nodes, Node{ID: id, Lat: float64(i) * 0.001})
	}
	repo := &fakeRepo{core: &Core{ID: "core", PlayerID: "p1", Level: 1}, nodes: nodes}
	st, err := newSvc(repo).GetNetwork(context.Background(), "p1")
	if err != nil {
		t.Fatalf("GetNetwork: %v", err)
	}
	if st.EcapUsed <= st.EcapMax {
		t.Fatalf("expected used (%d) > ecap (%d)", st.EcapUsed, st.EcapMax)
	}
	brownouts := 0
	for _, n := range st.Nodes {
		if n.Brownout {
			brownouts++
		}
	}
	if brownouts == 0 {
		t.Errorf("expected some nodes in brownout when over budget")
	}
}
