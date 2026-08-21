package network

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"time"

	"github.com/ezra-game/server/internal/canon"
	"github.com/ezra-game/server/internal/cell"
	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/pkg/geo"
	"github.com/ezra-game/server/pkg/httputil"
)

// RegionSeeder lazily populates the 0.1° grid region around a point with the
// cell grid (Overpass-driven). Placement seeds on demand so a player can plant
// a Core/beacon in a region the map hasn't loaded yet (softlock S1). Defined
// as an interface so the network service stays nil-safe in tests.
type RegionSeeder interface {
	EnsureRegion(ctx context.Context, lat, lng float64) error
}

// Service implements beacon-network / dome logic. See
// docs/feature/beacon_network_dome.md.
type Service struct {
	repo      Repository
	cells     cell.Repository
	players   player.Repository
	resources *player.ResourceService
	seeder    RegionSeeder
}

// NewService creates a network service.
func NewService(repo Repository, cells cell.Repository, players player.Repository, resources *player.ResourceService, seeder RegionSeeder) *Service {
	return &Service{repo: repo, cells: cells, players: players, resources: resources, seeder: seeder}
}

// ensureRegion best-effort seeds the region around (lat,lng) so the cell grid
// exists before a nearest-cell lookup. Errors are logged, not fatal: a partial
// or failed seed just falls back to whatever cells already exist.
//
// (0,0) is treated as "no GPS fix yet" (the player row's zero-value default
// before their first position update), never a real place to seed — without
// this guard, placing a Core/beacon in that window kicks off an ~80s Overpass
// fetch for Null Island that stalls every other request until it times out.
// Mirrors the client's own LocationService.IsCoordinateUsable.
func (s *Service) ensureRegion(ctx context.Context, lat, lng float64) {
	if s.seeder == nil {
		return
	}
	if lat == 0 && lng == 0 {
		slog.Warn("network: ensure region skipped, no position yet (0,0)")
		return
	}
	if err := s.seeder.EnsureRegion(ctx, lat, lng); err != nil {
		slog.Warn("network: ensure region failed", "lat", lat, "lng", lng, "error", err)
	}
}

// PlaceCore plants the player's personal Core (one per player) at the
// player's exact position (free placement); the nearest cell is only the
// infection-grid anchor. An explicit cellID keeps the legacy snap-to-cell.
func (s *Service) PlaceCore(ctx context.Context, playerID, cellID string) (*Core, error) {
	existing, err := s.repo.GetCore(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("get core: %w", err)
	}
	if existing != nil {
		return nil, httputil.NewBadRequest("core_exists", "you already have a Core; use relocate")
	}

	lat, lng, cellID, err := s.resolvePlacementPoint(ctx, playerID, cellID)
	if err != nil {
		return nil, err
	}

	core := &Core{PlayerID: playerID, CellID: cellID, Lat: lat, Lng: lng, Level: 1}
	if err := s.repo.CreateCore(ctx, core); err != nil {
		return nil, fmt.Errorf("create core: %w", err)
	}

	// Soft start (audit C4): clear the infection pocket under the first Core's
	// dome so the very first beacon visibly works. Best-effort and one-time —
	// PlaceCore only succeeds when the player has no Core yet.
	if clearer, ok := s.cells.(infectionClearer); ok {
		if n, cerr := clearer.ClearInfectionInRadius(ctx, lat, lng, canon.CoreEnergyRadiusM, canon.SoftStartInfection); cerr != nil {
			slog.Warn("soft start: clear infection failed", "player_id", playerID, "error", cerr)
		} else if n > 0 {
			slog.Info("soft start: cleared infection pocket", "player_id", playerID, "cells", n)
		}
	}

	if _, err := s.recompute(ctx, playerID); err != nil {
		slog.Error("network recompute after core placement failed", "player_id", playerID, "error", err)
	}
	slog.Info("core placed", "player_id", playerID, "cell_id", cellID)
	return core, nil
}

// infectionClearer is optionally implemented by the cell repository to support
// the soft-start pocket: knock infection down under a new player's first Core.
type infectionClearer interface {
	ClearInfectionInRadius(ctx context.Context, lat, lng, radiusM, target float64) (int64, error)
}

// crystalSpender is optionally implemented by the player repository so Core
// relocation can charge crystals atomically (paid past the first free move).
type crystalSpender interface {
	SpendCrystals(ctx context.Context, playerID string, amount int) (bool, error)
}

// RelocateCore moves the player's Core to a new cell (monetised convenience;
// payment/cooldown gating is a v1.0 follow-up — see economy_and_shop.md).
func (s *Service) RelocateCore(ctx context.Context, playerID, cellID string) (*Core, error) {
	core, err := s.repo.GetCore(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("get core: %w", err)
	}
	if core == nil {
		return nil, httputil.NewBadRequest("no_core", "no Core to relocate; place one first")
	}

	lat, lng, cellID, err := s.resolvePlacementPoint(ctx, playerID, cellID)
	if err != nil {
		return nil, err
	}

	// The first relocation is free; every later one costs crystals (premium
	// convenience, never power — economy_and_shop.md). RelocatedAt is nil until
	// the first move, so it doubles as the "has relocated before" flag. Charge
	// only after the target point validates, so a bad target never bills.
	if core.RelocatedAt != nil && canon.CoreRelocationCostCrystals > 0 {
		if spender, ok := s.players.(crystalSpender); ok {
			paid, sErr := spender.SpendCrystals(ctx, playerID, canon.CoreRelocationCostCrystals)
			if sErr != nil {
				return nil, fmt.Errorf("charge relocation: %w", sErr)
			}
			if !paid {
				return nil, httputil.NewBadRequest("insufficient_crystals",
					fmt.Sprintf("перенос Ядра стоит %d 💎", canon.CoreRelocationCostCrystals))
			}
		}
	}

	core.CellID = cellID
	core.Lat = lat
	core.Lng = lng
	if err := s.repo.UpdateCore(ctx, core); err != nil {
		return nil, fmt.Errorf("update core: %w", err)
	}
	if _, err := s.recompute(ctx, playerID); err != nil {
		slog.Error("network recompute after core relocation failed", "player_id", playerID, "error", err)
	}
	slog.Info("core relocated", "player_id", playerID, "cell_id", cellID)
	return core, nil
}

// UpgradeCore raises the Core level (more energy capacity), charging resources.
func (s *Service) UpgradeCore(ctx context.Context, playerID string) (*Core, error) {
	core, err := s.repo.GetCore(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("get core: %w", err)
	}
	if core == nil {
		return nil, httputil.NewBadRequest("no_core", "no Core to upgrade; place one first")
	}
	next := core.Level + 1
	cost, ok := canon.CoreUpgradeCost[next]
	if !ok {
		return nil, httputil.NewBadRequest("max_level", "Core is already at max level")
	}
	if s.resources != nil {
		if _, err := s.resources.Spend(ctx, playerID, cost.Energy, cost.Materials); err != nil {
			return nil, err
		}
	}
	core.Level = next
	if err := s.repo.UpdateCore(ctx, core); err != nil {
		return nil, fmt.Errorf("update core: %w", err)
	}
	if _, err := s.recompute(ctx, playerID); err != nil {
		slog.Error("network recompute after core upgrade failed", "player_id", playerID, "error", err)
	}
	slog.Info("core upgraded", "player_id", playerID, "level", next)
	return core, nil
}

// GetNetwork returns the player's current computed network state.
func (s *Service) GetNetwork(ctx context.Context, playerID string) (*State, error) {
	return s.recompute(ctx, playerID)
}

// RegionFields returns the powered energy disks of EVERY player whose network
// reaches the (lat,lng,radiusM) view, with ownership stripped. The client
// unions them into one continuous field so neighbouring/allied domes render as
// a single shield — the visual half of Meta-Dome. Suppression already merges
// server-side (a domed cell decays regardless of owner), so this is purely
// cosmetic and needs no alliance system.
//
// Read-only: powered connectivity is computed in memory via connectNodes (the
// same pure function recompute uses) — it does NOT write domed_cells, so map
// polling can't trigger per-neighbour recompute side effects.
func (s *Service) RegionFields(ctx context.Context, lat, lng, radiusM float64) ([]FieldDisk, error) {
	// Expand the scan so a node just outside the view whose disk still reaches
	// into it is included (largest possible disk is a level-3 beacon's).
	scanR := radiusM + canon.MaxEnergyRadiusM
	players, err := s.repo.PlayersWithNodesInRadius(ctx, lat, lng, scanR)
	if err != nil {
		return nil, fmt.Errorf("players in radius: %w", err)
	}
	var disks []FieldDisk
	for _, pid := range players {
		nodes, err := s.repo.GetNodes(ctx, pid)
		if err != nil || len(nodes) == 0 {
			continue
		}
		nodes, _ = activeNodes(nodes, time.Now()) // R4-4a: dead legacy beacons don't render
		coreID := ""
		for _, n := range nodes {
			if n.IsCore {
				coreID = n.ID
				break
			}
		}
		if coreID == "" {
			continue // no Core → no rooted/powered network
		}
		powered, _, _ := connectNodes(nodes, coreID)
		for _, n := range nodes {
			if !powered[n.ID] {
				continue
			}
			rad := canon.NodeEnergyRadius(n.IsCore, n.Level)
			if geo.Haversine(lat, lng, n.Lat, n.Lng) <= radiusM+rad {
				disks = append(disks, FieldDisk{Lat: n.Lat, Lng: n.Lng, RadiusM: rad})
			}
		}
	}
	return disks, nil
}

// Recompute re-derives the whole network from node positions (Perimeter
// model: nothing about the graph is stored). The tower service calls it
// after a beacon is placed or removed.
func (s *Service) Recompute(ctx context.Context, playerID string) error {
	_, err := s.recompute(ctx, playerID)
	return err
}

// CanConnect reports whether a new beacon placed at (lat,lng) would join the
// player's POWERED network: it must stand within the ConnectionRadius of some
// connected node. hasNetwork is false when there is no Core yet (then
// placement is unrestricted — the first node bootstraps the network).
func (s *Service) CanConnect(ctx context.Context, playerID string, lat, lng float64) (bool, bool, error) {
	core, err := s.repo.GetCore(ctx, playerID)
	if err != nil {
		return false, false, err
	}
	if core == nil {
		return false, false, nil
	}
	nodes, err := s.repo.GetNodes(ctx, playerID)
	if err != nil {
		return false, true, err
	}
	nodes, _ = activeNodes(nodes, time.Now()) // R4-4a: can't anchor to a dead beacon
	powered, _, _ := connectNodes(nodes, core.ID)
	for _, n := range nodes {
		if !powered[n.ID] {
			continue
		}
		if geo.Haversine(lat, lng, n.Lat, n.Lng) < canon.NodeConnectionRadius(n.IsCore, n.Level) {
			return true, true, nil
		}
	}
	return false, true, nil
}

// resolvePlacementPoint returns where a Core lands: with an empty cellID the
// player's exact position (free placement) anchored to the nearest cell;
// with an explicit cellID the legacy cell-centre snap (≤30m walk-up check).
// ResolvePlayerPlacement resolves a placement cell at the player's current
// server-side position (seeding the region if needed) — used by other features
// (N3 nest onboarding) that place at "where I stand" without a client cell id.
func (s *Service) ResolvePlayerPlacement(ctx context.Context, playerID string) (float64, float64, string, error) {
	return s.resolvePlacementPoint(ctx, playerID, "")
}

func (s *Service) resolvePlacementPoint(ctx context.Context, playerID, cellID string) (float64, float64, string, error) {
	if cellID != "" {
		c, err := s.locateCell(ctx, playerID, cellID)
		if err != nil {
			return 0, 0, "", err
		}
		return c.Lat, c.Lng, cellID, nil
	}

	p, err := s.players.GetByID(ctx, playerID)
	if err != nil {
		return 0, 0, "", fmt.Errorf("get player: %w", err)
	}
	// Seed the region on demand so a Core can be planted even where the map
	// hasn't been polled yet (softlock S1).
	s.ensureRegion(ctx, p.Position.Lat, p.Position.Lng)
	id, err := s.repo.NearestCellID(ctx, p.Position.Lat, p.Position.Lng)
	if err != nil {
		return 0, 0, "", err
	}
	if id == "" {
		return 0, 0, "", httputil.NewBadRequest("no_cell", "рядом нет клетки")
	}
	c, err := s.cells.GetByID(ctx, id)
	if err != nil || c == nil {
		return 0, 0, "", httputil.NewNotFound("not_found", "cell not found")
	}
	if geo.Haversine(p.Position.Lat, p.Position.Lng, c.Lat, c.Lng) > canon.TowerAnchorMaxDistanceM {
		return 0, 0, "", httputil.NewBadRequest("no_cell", "слишком далеко от размеченной зоны")
	}
	slog.Info("core placement point (server-side player position)",
		"player_id", playerID,
		"lat", p.Position.Lat, "lng", p.Position.Lng,
		"position_age_s", time.Since(p.Position.UpdatedAt).Seconds())
	return p.Position.Lat, p.Position.Lng, id, nil
}

func (s *Service) locateCell(ctx context.Context, playerID, cellID string) (*cell.Cell, error) {
	p, err := s.players.GetByID(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("get player: %w", err)
	}
	c, err := s.cells.GetByID(ctx, cellID)
	if err != nil || c == nil {
		return nil, httputil.NewNotFound("not_found", "cell not found")
	}
	if geo.Haversine(p.Position.Lat, p.Position.Lng, c.Lat, c.Lng) > canon.TowerPlacementMaxDistanceM {
		return nil, httputil.NewBadRequest("too_far", "must be within range of the cell")
	}
	return c, nil
}

// NearestCellID returns the cell nearest to (lat,lng) — server-authoritative
// "place where you stand" selection. Exposed for the tower placement hook.
// Seeds the region first so beacons can be placed in regions the map hasn't
// loaded yet (softlock S1).
func (s *Service) NearestCellID(ctx context.Context, lat, lng float64) (string, error) {
	s.ensureRegion(ctx, lat, lng)
	return s.repo.NearestCellID(ctx, lat, lng)
}

// recompute rebuilds connectivity, links, energy budget and the dome from
// node positions alone (Perimeter model: the graph is never stored, only
// re-derived), persists the domed-cell set, and returns the network state.
func (s *Service) recompute(ctx context.Context, playerID string) (*State, error) {
	core, err := s.repo.GetCore(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("get core: %w", err)
	}

	nodes, err := s.repo.GetNodes(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("get nodes: %w", err)
	}
	// R4-4a: legacy beacons that have gone offline drop out of the network
	// entirely (no power, no dome); survivors carry their legacy state.
	nodes, legacy := activeNodes(nodes, time.Now())

	state := &State{}
	if core != nil {
		state.Core = core
		state.EcapMax = canon.CoreEcap(core.Level)
		// First relocation is free (RelocatedAt still nil); after that it costs.
		if core.RelocatedAt != nil {
			state.NextRelocationCost = canon.CoreRelocationCostCrystals
		}
	}

	// No core → no rooted network; clear any stale dome.
	if core == nil {
		_ = s.repo.ClearDomed(ctx, playerID)
		state.Nodes = nodeStatuses(nodes, nil, nil, legacy)
		return state, nil
	}

	// Connectivity is geometric (Perimeter port): each node attaches to the
	// closest already-connected node whose ConnectionRadius (by kind+level)
	// covers it — that parent edge is the visible energy line. The energy
	// disks below are only the field/dome footprint.
	powered, depth, parent := connectNodes(nodes, core.ID)
	state.Links = parentLinks(playerID, parent)

	// Energy: Σ(upkeep + link throughput) ≤ Ecap; over budget → brownout the
	// deepest nodes. Link cost used to be display-only (canon says it shares
	// the budget), which made Ecap nearly cosmetic for stretched networks.
	nodeCost := map[string]int{}
	used := 0
	for id := range powered {
		if id == core.ID {
			continue
		}
		cost := canon.BeaconUpkeep
		if pe, ok := parent[id]; ok {
			cost += canon.LinkCost(pe.distM)
		}
		nodeCost[id] = cost
		used += cost
	}
	state.EcapUsed = used
	brownout := computeBrownout(powered, depth, core.ID, nodeCost, used, state.EcapMax)
	// Persist the flag so SQL consumers (infection suppression, income) see it.
	if setter, ok := s.repo.(interface {
		SetTowerBrownout(ctx context.Context, playerID string, brownoutIDs []string) error
	}); ok {
		ids := make([]string, 0, len(brownout))
		for id := range brownout {
			ids = append(ids, id)
		}
		if err := setter.SetTowerBrownout(ctx, playerID, ids); err != nil {
			slog.Error("persist brownout failed", "player_id", playerID, "error", err)
		}
	}

	// Field (radius_only): union of the powered nodes' energy disks — partial
	// suppression around every beacon, always on (beacon_network_dome.md §6).
	if err := s.repo.ClearDomed(ctx, playerID); err != nil {
		slog.Error("clear domed failed", "player_id", playerID, "error", err)
	}
	// N2 (T-864): a spirit-pressured perimeter beacon drops from the dome for the
	// wave's duration, so the sealed area shrinks and infection creeps in — the
	// cascade — without any spirit entering the dome. Energy-brownout behaviour is
	// unchanged; only spirit pressure carves the dome.
	pressured := map[string]bool{}
	if ids, err := s.repo.SpiritPressuredIDs(ctx, playerID); err == nil {
		for _, id := range ids {
			pressured[id] = true
		}
	}
	domePowered := powered
	if len(pressured) > 0 {
		domePowered = make(map[string]bool, len(powered))
		for id, ok := range powered {
			domePowered[id] = ok && !pressured[id]
		}
	}

	var lngs, lats, radii []float64
	for _, n := range nodes {
		if domePowered[n.ID] {
			lngs = append(lngs, n.Lng)
			lats = append(lats, n.Lat)
			radii = append(radii, canon.NodeEnergyRadius(n.IsCore, n.Level))
		}
	}
	domed := 0
	if count, err := s.repo.InsertDomedByDisks(ctx, playerID, lngs, lats, radii); err != nil {
		slog.Error("insert domed by disks failed", "player_id", playerID, "error", err)
	} else {
		domed += count
	}

	// Sealed dome(s): closed contours of the network enclose whole blocks; the
	// area inside (ST_Contains) is domed even where no disk reaches it. A real
	// ring — not just any disk field — is what "sealed" means.
	contours := findContours(nodes, domePowered, depth, parent)
	if len(contours) > 0 {
		if count, err := s.repo.InsertDomedByPolygons(ctx, playerID, contours); err != nil {
			slog.Error("insert domed by polygons failed", "player_id", playerID, "error", err)
		} else {
			domed += count
		}
		state.DomeSealed = true
		state.DomePolygon = largestContour(contours)
	}

	state.DomedCellCount = domed
	state.Nodes = nodeStatuses(nodes, powered, brownout, legacy)
	return state, nil
}

// parentEdge is a node's attachment to the network: the closest connected
// node at the moment the node joined, and the distance to it.
type parentEdge struct {
	id    string
	distM float64
}

// connectNodes computes the powered set the way Perimeter's ConnectCoreOp
// does: starting from the Core, every unconnected node attaches to the
// CLOSEST already-connected node whose ConnectionRadius covers it (the
// EXISTING node's reach, like `d < sqr((*i)->attr()->ConnectionRadius)` in
// the original — so upgraded nodes pull farther), and immediately becomes
// part of the connected set so the wave propagates outward. Returns the
// powered set, the hop depth from the Core and each node's single parent
// edge. The energy disk is NOT involved here — it is only the visible
// field/dome footprint.
func connectNodes(nodes []Node, coreID string) (map[string]bool, map[string]int, map[string]parentEdge) {
	powered := map[string]bool{coreID: true}
	depth := map[string]int{coreID: 0}
	parent := map[string]parentEdge{}
	for {
		progressed := false
		for _, n := range nodes {
			if powered[n.ID] {
				continue
			}
			best := parentEdge{distM: math.MaxFloat64}
			for _, c := range nodes {
				if !powered[c.ID] {
					continue
				}
				d := geo.Haversine(n.Lat, n.Lng, c.Lat, c.Lng)
				if d < canon.NodeConnectionRadius(c.IsCore, c.Level) && d < best.distM {
					best = parentEdge{id: c.ID, distM: d}
				}
			}
			if best.id != "" {
				powered[n.ID] = true
				depth[n.ID] = depth[best.id] + 1
				parent[n.ID] = best
				progressed = true
			}
		}
		if !progressed {
			break
		}
	}
	return powered, depth, parent
}

// parentLinks converts the parent edges into the wire-format link list
// (deterministic order for stable API responses).
func parentLinks(playerID string, parent map[string]parentEdge) []Link {
	links := make([]Link, 0, len(parent))
	for id, pe := range parent {
		links = append(links, Link{
			PlayerID: playerID,
			FromID:   pe.id,
			ToID:     id,
			LengthM:  pe.distM,
			Cost:     canon.LinkCost(pe.distM),
		})
	}
	sort.Slice(links, func(i, j int) bool { return links[i].ToID < links[j].ToID })
	return links
}

// computeBrownout marks the deepest powered nodes as brownout until the running
// energy cost (upkeep + link throughput per node) is within the Core budget.
func computeBrownout(powered map[string]bool, depth map[string]int, coreID string, nodeCost map[string]int, used, ecap int) map[string]bool {
	brownout := map[string]bool{}
	if ecap <= 0 || used <= ecap {
		return brownout
	}
	type nd struct {
		id    string
		depth int
	}
	var list []nd
	for id := range powered {
		if id == coreID {
			continue
		}
		list = append(list, nd{id, depth[id]})
	}
	// Deepest first — they are the most expendable tail of the network.
	sort.Slice(list, func(i, j int) bool { return list[i].depth > list[j].depth })
	over := used - ecap
	for _, n := range list {
		if over <= 0 {
			break
		}
		brownout[n.id] = true
		cost := nodeCost[n.id]
		if cost <= 0 {
			cost = canon.BeaconUpkeep
		}
		over -= cost
	}
	return brownout
}

func nodeStatuses(nodes []Node, powered, brownout map[string]bool, legacy map[string]string) []NodeStatus {
	out := make([]NodeStatus, 0, len(nodes))
	for _, n := range nodes {
		st := ""
		if legacy != nil {
			st = legacy[n.ID]
		}
		out = append(out, NodeStatus{
			ID:          n.ID,
			Lat:         n.Lat,
			Lng:         n.Lng,
			IsCore:      n.IsCore,
			Powered:     powered != nil && powered[n.ID],
			Brownout:    brownout != nil && brownout[n.ID],
			RadiusM:     canon.NodeEnergyRadius(n.IsCore, n.Level),
			LegacyState: st,
		})
	}
	return out
}

// activeNodes drops legacy beacons that have gone fully offline (dead structures
// that no longer power or dome) and returns the kept nodes plus the per-node
// legacy state (legacy_stable/legacy_fading) for the survivors (R4-4a). Active
// human-owned nodes (LegacySince nil) pass through unchanged.
func activeNodes(nodes []Node, now time.Time) ([]Node, map[string]string) {
	kept := make([]Node, 0, len(nodes))
	state := make(map[string]string)
	for _, n := range nodes {
		if n.LegacySince == nil {
			kept = append(kept, n)
			continue
		}
		st := canon.LegacyState(n.Level, now.Sub(*n.LegacySince))
		if st == canon.LegacyOffline {
			continue // dead structure → excluded from the network graph & field
		}
		state[n.ID] = st
		kept = append(kept, n)
	}
	return kept, state
}
