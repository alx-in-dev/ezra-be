package canon

import "math"

// Beacon network / dome constants. See docs/feature/beacon_network_dome.md.
// Numbers are MVP starting points and intended to be tuned via balance_tables.md.
const (
	CoreMaxLevelMVP = 5

	// Core relocation (economy_and_shop.md): a premium *convenience*, not power
	// (anti-P2W — it never grants Ecap/area). The first relocation is FREE; each
	// subsequent one costs this many crystals. Gating keeps repositioning a
	// deliberate choice rather than a free teleport.
	CoreRelocationCostCrystals = 50

	// Energy throughput: Σ(beacon upkeep + link cost) must be ≤ Core Ecap.
	BeaconUpkeep    = 10 // per powered beacon
	LinkCostPer100m = 3  // link cost = ceil(len_m/100) * this

	// Minimum spacing between a new beacon and the nearest existing node.
	// TEMP 30m for desk testing — design value is 100m.
	NetworkMinSpacingM = 30.0
	// Free placement: how far the anchor cell (infection-grid attachment) may
	// be from the beacon's actual point. Farther means the spot is outside
	// the seeded world, where terrain flags are meaningless.
	TowerAnchorMaxDistanceM = 200.0

	// Domed cells are suppressed strongly (the dome blocks Spirits). Per-hour;
	// the infection job scales it to its tick interval.
	DomeSuppressionPerHour = 30.0

	// Infected cells gnaw nearby beacons. Below this infection level a beacon
	// takes no pressure damage (a domed beacon sits in suppressed cells → safe).
	BeaconPressureThreshold = 50.0
	// HP/hour per infection point above the threshold (at 100% → 25 HP/hour).
	BeaconPressureRate = 0.5

	// Defense race (beacon_network_dome.md §8). Warn the owner when a beacon
	// under pressure drops to/below this fraction of its max HP, so they can
	// run to repair it before it falls.
	BeaconLowHPWarnFraction = 0.3
	// Repair restores this many HP per energy point poured into the node.
	BeaconRepairHPPerEnergy = 2

	// Passive energy income per sealed-dome cell per hour (B3). Credited only
	// for closed-contour area (not radius/disk cells) so it never double-counts
	// the per-beacon passive income. beacon_network_dome.md §6.
	DomeAreaEnergyPerCellPerHour = 0.3

	// Beacon area capacity (energy pillar MVP, #7): the max beacons (any owner)
	// allowed within TowerOverloadRadiusM scales with the LOCAL electrical-grid
	// capacity, approximated by OSM building density (building-flagged cells in
	// the radius). Dense districts host more beacons; sparse areas fall to a
	// floor so solo play survives. This replaces the old flat overload cap at
	// placement. Capacity is computed on-the-fly only on rare paths (placement,
	// single-tower detail) — never per-tower on map polls (would be N²; a
	// precomputed table is the future refinement).
	BeaconCapacityMin     = 3    // floor: solo in the sticks can still build a few
	BeaconCapacityPerCell = 1.0  // capacity per building-flagged cell in radius
	BeaconCapacityHardCap = 12   // absolute perf ceiling (falloff/income worker bound)

	// Soft start (audit C4): a brand-new player's first Core instantly knocks
	// the cells under its dome down to this infection level so the very first
	// beacon VISIBLY works. The lone Core's 30%/h suppression would otherwise
	// take ~40 min to drop a 100% pocket below the 50% pressure threshold —
	// a dead first session. One-time, only fires on first Core placement.
	SoftStartInfection = 20.0
)

// BeaconPressureDamagePerHour returns the HP a beacon loses per hour from the
// infection level of the cell it stands on. Domed beacons (suppressed cell,
// infection ≤ threshold) take zero — neglect is what eats your network.
func BeaconPressureDamagePerHour(infection float64) float64 {
	over := infection - BeaconPressureThreshold
	if over <= 0 {
		return 0
	}
	return over * BeaconPressureRate
}

// Energy-disk radii (Perimeter ZeroLayerRadius port): each node projects an
// energy disk; the field/dome is the union of POWERED nodes' disks. The disk
// is the VISIBLE zone only — transmission reach is ConnectionRadius below.
const (
	CoreEnergyRadiusM   = 160.0
	BeaconEnergyRadiusM = 110.0
)

// ConnectionRadius (Perimeter ConnectionRadius port, decoupled from the energy
// disk like Frame=300/Transmitter=500 vs ZeroLayer=200/100 in the original):
// a new node joins the network when it stands within the ConnectionRadius of
// an already-CONNECTED node, so upgraded nodes reach out farther and form
// thin relay branches with no field around the line itself.
var (
	// Core level also buys reach: the higher the Core, the farther from it
	// the first ring of beacons can stand. (Level already buys Ecap = how
	// many beacons the network feeds — see CoreEcapByLevel.)
	CoreConnRadiusByLevel = map[int]float64{1: 300, 2: 375, 3: 450, 4: 525, 5: 600}
	// Beacon level buys reach: upgrade a frontier beacon to push the network
	// deeper ("вглубь или вширь" choice per upgrade).
	BeaconConnRadiusByLevel = map[int]float64{1: 250, 2: 400, 3: 600}
)

// NodeConnectionRadius returns how far a connected node reaches to attach new
// nodes, by kind and level (clamped to level 1 when the level is unknown).
func NodeConnectionRadius(isCore bool, level int) float64 {
	if isCore {
		if r, ok := CoreConnRadiusByLevel[level]; ok {
			return r
		}
		return CoreConnRadiusByLevel[1]
	}
	if r, ok := BeaconConnRadiusByLevel[level]; ok {
		return r
	}
	return BeaconConnRadiusByLevel[1]
}

// NodeEnergyRadius returns a node's energy-disk radius.
func NodeEnergyRadius(isCore bool) float64 {
	if isCore {
		return CoreEnergyRadiusM
	}
	return BeaconEnergyRadiusM
}

// CoreEcapByLevel is the energy budget a Core of a given level can sustain.
var CoreEcapByLevel = map[int]int{1: 100, 2: 180, 3: 320, 4: 560, 5: 960}

// CoreUpgradeCost maps the TARGET core level to its energy/materials cost.
var CoreUpgradeCost = map[int]ResourceCost{
	2: {Energy: 200, Materials: 40},
	3: {Energy: 500, Materials: 100},
	4: {Energy: 1200, Materials: 240},
	5: {Energy: 3000, Materials: 600},
}

// ResourceCost is a simple energy/materials pair.
type ResourceCost struct {
	Energy    int
	Materials int
}

// BeaconAreaCapacity maps the local OSM building density (count of
// building-flagged cells within the overload radius) to the max number of
// beacons that area's grid can power. Clamped to [BeaconCapacityMin,
// BeaconCapacityHardCap]. See the energy-pillar constants above.
func BeaconAreaCapacity(buildingCells int) int {
	return BeaconAreaCapacityWith(buildingCells, 0)
}

// BeaconAreaCapacityWith adds a station capacity bonus (T-777 power plants) on
// top of the OSM-density base before clamping. The HARD_CAP still bounds the
// total, so stations let a sparse area catch up but never overtake a dense
// centre for free (anti-geography-nullification).
func BeaconAreaCapacityWith(buildingCells, stationBonus int) int {
	c := BeaconCapacityMin + int(math.Round(BeaconCapacityPerCell*float64(buildingCells))) + stationBonus
	if c < BeaconCapacityMin {
		c = BeaconCapacityMin
	}
	if c > BeaconCapacityHardCap {
		c = BeaconCapacityHardCap
	}
	return c
}

// CoreEcap returns the energy capacity for a Core level (clamped to L1).
func CoreEcap(level int) int {
	if e, ok := CoreEcapByLevel[level]; ok {
		return e
	}
	return CoreEcapByLevel[1]
}

// LinkCost returns the energy-throughput cost of a link of the given length.
func LinkCost(lengthM float64) int {
	n := int(math.Ceil(lengthM / 100.0))
	if n < 1 {
		n = 1
	}
	return n * LinkCostPer100m
}
