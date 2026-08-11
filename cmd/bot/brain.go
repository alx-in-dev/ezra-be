package main

import (
	"context"
	"hash/fnv"
	"math/rand"
	"strings"

	"github.com/ezra-game/server/internal/cell"
	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/internal/pvp"
	"github.com/ezra-game/server/pkg/geo"
)

// World is the snapshot a brain reasons over each tick. It is everything the bot
// "sees": itself, the cells around it, and (for symbionts) nearby breach targets.
type World struct {
	Self       player.Player
	Cells      []cell.CellDTO
	PvpTargets []pvp.Target
}

// ActionKind enumerates the high-level intents a brain can return.
type ActionKind int

const (
	ActIdle       ActionKind = iota // do nothing this tick
	ActMoveTo                       // walk toward (Lat,Lng); the agent steps gradually
	ActPlace                        // build a tower at current location
	ActCapture                      // lockpick the tower with id TowerID
	ActAttackRift                   // start (and resolve) a battle against rift RiftID
	ActBreach                       // puncture an enemy dome on cell CellID (symbiont PvP)
)

// Action is a single decision. Reason is logged so swarm behaviour is legible.
type Action struct {
	Kind    ActionKind
	Lat     float64
	Lng     float64
	TowerID string
	RiftID  string
	CellID  string
	Reason  string
}

// Brain decides what a bot does given what it sees. Swap ScriptedBrain for
// LLMBrain (or any future impl) without touching the agent loop — this is the
// seam that lets the OpenRouter brain drop in behind the same contract.
type Brain interface {
	Name() string
	Decide(ctx context.Context, w World) Action
	// Observe feeds the result of the last action back to the brain so it can
	// learn from failures (e.g. blacklist a cell that returned too_close).
	Observe(action Action, err error)
}

// --- ScriptedBrain: a goal-driven state machine ---------------------------
//
// Goal: spread the bot's beacon network and pressure enemies. Each tick:
//  1. If standing near an enemy tower → lockpick it.
//  2. Else if standing on a free, buildable cell and we can afford it → build.
//  3. Else → walk toward the nearest interesting cell (free buildable, else any).
//
// "Near" is generous because PlaceTower with empty cell_id snaps to the nearest
// cell server-side, so the bot only needs to be roughly on top of a target.

const (
	placeMaterialsCost = 50 // conservative guard; server is authoritative
	captureRangeM      = 45 // within this distance we attempt a capture
	arriveRangeM       = 25 // close enough to act on a target cell
	roamReselectTicks  = 6  // pick a new roam target after this many ticks
	battleMinEnergy    = 25 // ⚡ to engage a (medium) rift; skip battle below this
)

type ScriptedBrain struct {
	selfID string // set by agent so the brain can tell own towers from enemies'

	// triedCapture remembers towers we already attempted, so a server-disabled
	// or failed capture isn't retried every tick (no log spam, no thrash).
	triedCapture map[string]bool

	// triedRifts remembers rifts we already fought, so a lost/retreated battle
	// isn't re-attempted in a tight loop.
	triedRifts map[string]bool

	// triedBreaches remembers domes we've already breached (each can only hold
	// one rift), so we don't spam already_breached.
	triedBreaches map[string]bool

	// failedPlaces blacklists grid points where a build failed (too_close /
	// no_power_source), so the bot stops re-targeting a doomed spot.
	failedPlaces map[string]bool

	// lastPlace records where we last tried to build, so Observe can blacklist
	// that exact spot if the placement failed.
	lastPlaceLat, lastPlaceLng float64

	// captureDisabled latches once the server reports capture is feature-locked,
	// so we stop attempting lockpicks entirely (kills the feature_locked spam).
	captureDisabled bool

	// rng disperses target choice so 10 bots don't all stampede the same dome /
	// rift (thundering herd). Seeded per-bot from the player id.
	rng *rand.Rand

	// Roam state: a wander destination the bot keeps until it arrives or the
	// counter expires. Keeps bots visibly moving when there's nothing to build.
	roamLat, roamLng float64
	roamTicks        int
}

// placeKey buckets a coordinate to ~12m so a blacklisted build spot also covers
// the immediate neighbourhood the server would snap to.
func placeKey(lat, lng float64) string {
	return geo.CellID(lat, lng)
}

func (b *ScriptedBrain) Name() string { return "scripted" }

func (b *ScriptedBrain) Decide(_ context.Context, w World) Action {
	b.selfID = w.Self.ID
	if b.triedCapture == nil {
		b.triedCapture = map[string]bool{}
		b.triedRifts = map[string]bool{}
		b.triedBreaches = map[string]bool{}
		b.failedPlaces = map[string]bool{}
		b.rng = rand.New(rand.NewSource(seedFromID(w.Self.ID)))
	}
	lat, lng := w.Self.Position.Lat, w.Self.Position.Lng

	// 0a) Symbiont with energy + an enemy dome to breach → breach it (opens a
	// rift, our signature PvP move). Energy-gated (200⚡) and dispersed across
	// bots so they don't all stampede the same dome.
	if w.Self.Energy >= pvp.BreachEnergyCost {
		if t := b.unbreachedTarget(w); t != nil {
			b.triedBreaches[t.CellID] = true
			return Action{Kind: ActBreach, CellID: t.CellID, Reason: "breach enemy dome"}
		}
	}

	// 0b) A rift in view we haven't fought yet → attack it (PvE, no proximity
	// needed), if we can afford the engagement cost.
	if w.Self.Energy >= battleMinEnergy {
		if rid := b.unfoughtRift(w); rid != "" {
			b.triedRifts[rid] = true
			return Action{Kind: ActAttackRift, RiftID: rid, Reason: "attack rift"}
		}
	}

	// 1) Enemy tower in reach we haven't tried yet → capture (once).
	if t := b.nearestEnemyTower(w, lat, lng); t != nil {
		d := geo.Haversine(lat, lng, t.Lat, t.Lng)
		if d <= captureRangeM {
			b.triedCapture[t.ID] = true
			return Action{Kind: ActCapture, TowerID: t.ID, Reason: "lockpick enemy tower nearby"}
		}
		return Action{Kind: ActMoveTo, Lat: t.Lat, Lng: t.Lng, Reason: "approach enemy tower"}
	}

	// 2) Standing on a buildable free cell + can afford → build.
	if w.Self.Materials >= placeMaterialsCost {
		if c := b.cellUnderfoot(w, lat, lng); c != nil && b.canBuild(c) {
			b.lastPlaceLat, b.lastPlaceLng = lat, lng
			return Action{Kind: ActPlace, Reason: "build tower on buildable cell"}
		}
		// Head for the nearest buildable free cell (one with a power source).
		if c := b.nearestBuildableCell(w, lat, lng); c != nil {
			return Action{Kind: ActMoveTo, Lat: c.Lat, Lng: c.Lng, Reason: "seek buildable cell"}
		}
	}

	// 3) Nothing to do (no materials / no buildable cell) → roam so the bot
	// keeps moving for proximity/crowd testing instead of freezing in place.
	return b.roam(w, lat, lng)
}

// isBuildable mirrors the server rule that a tower needs a power source: a road
// or building within range (no_power_source otherwise). Pre-filtering here
// avoids spamming doomed PlaceTower calls.
func isBuildable(c *cell.CellDTO) bool {
	return c.TowerID == nil && (c.HasNearbyRoad || c.HasNearbyBuilding)
}

// canBuild adds the runtime blacklist (spots a prior build failed at) on top of
// the static buildability rule.
func (b *ScriptedBrain) canBuild(c *cell.CellDTO) bool {
	return isBuildable(c) && !b.failedPlaces[placeKey(c.Lat, c.Lng)]
}

// Observe learns from the last action's result: blacklist a failed build spot,
// and latch capture-off once the server says the feature is locked.
func (b *ScriptedBrain) Observe(action Action, err error) {
	if err == nil {
		return
	}
	switch action.Kind {
	case ActPlace:
		if b.failedPlaces == nil {
			b.failedPlaces = map[string]bool{}
		}
		b.failedPlaces[placeKey(b.lastPlaceLat, b.lastPlaceLng)] = true
	case ActCapture:
		if strings.Contains(err.Error(), "feature_locked") {
			b.captureDisabled = true
		}
	}
}

// seedFromID derives a stable per-bot RNG seed from the player id so each bot
// considers targets in a different order.
func seedFromID(id string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(id))
	return int64(h.Sum64())
}

// unbreachedTarget returns a random enemy dome we haven't breached yet (random
// so concurrent bots spread across domes instead of colliding), or nil.
func (b *ScriptedBrain) unbreachedTarget(w World) *pvp.Target {
	var avail []int
	for i := range w.PvpTargets {
		if !b.triedBreaches[w.PvpTargets[i].CellID] {
			avail = append(avail, i)
		}
	}
	if len(avail) == 0 {
		return nil
	}
	return &w.PvpTargets[avail[b.rng.Intn(len(avail))]]
}

func (b *ScriptedBrain) roam(w World, lat, lng float64) Action {
	arrived := geo.Haversine(lat, lng, b.roamLat, b.roamLng) <= arriveRangeM
	if b.roamTicks <= 0 || arrived || (b.roamLat == 0 && b.roamLng == 0) {
		// Pick the farthest visible cell as a new wander target so the bot
		// actually traverses ground rather than jittering locally.
		var far *cell.CellDTO
		bestD := -1.0
		for i := range w.Cells {
			c := &w.Cells[i]
			if d := geo.Haversine(lat, lng, c.Lat, c.Lng); d > bestD {
				far, bestD = c, d
			}
		}
		if far == nil {
			return Action{Kind: ActIdle, Reason: "no cells in view"}
		}
		b.roamLat, b.roamLng, b.roamTicks = far.Lat, far.Lng, roamReselectTicks
	}
	b.roamTicks--
	return Action{Kind: ActMoveTo, Lat: b.roamLat, Lng: b.roamLng, Reason: "roam"}
}

func (b *ScriptedBrain) cellUnderfoot(w World, lat, lng float64) *cell.CellDTO {
	var best *cell.CellDTO
	bestD := float64(arriveRangeM)
	for i := range w.Cells {
		c := &w.Cells[i]
		if d := geo.Haversine(lat, lng, c.Lat, c.Lng); d <= bestD {
			best, bestD = c, d
		}
	}
	return best
}

func (b *ScriptedBrain) nearestBuildableCell(w World, lat, lng float64) *cell.CellDTO {
	var best *cell.CellDTO
	bestD := 1e18
	for i := range w.Cells {
		c := &w.Cells[i]
		if !b.canBuild(c) {
			continue
		}
		if d := geo.Haversine(lat, lng, c.Lat, c.Lng); d < bestD {
			best, bestD = c, d
		}
	}
	return best
}

// unfoughtRift returns a random rift visible on the map that this bot hasn't
// fought yet (random so bots spread across rifts), or "" if none.
func (b *ScriptedBrain) unfoughtRift(w World) string {
	var avail []string
	for i := range w.Cells {
		if rid := w.Cells[i].RiftID; rid != nil && !b.triedRifts[*rid] {
			avail = append(avail, *rid)
		}
	}
	if len(avail) == 0 {
		return ""
	}
	return avail[b.rng.Intn(len(avail))]
}

func (b *ScriptedBrain) nearestEnemyTower(w World, lat, lng float64) *cell.TowerSummary {
	if b.captureDisabled {
		return nil // server has capture feature-locked; don't bother
	}
	var best *cell.TowerSummary
	bestD := 1e18
	for i := range w.Cells {
		t := w.Cells[i].Tower
		if t == nil || t.OwnerID == b.selfID || b.triedCapture[t.ID] {
			continue
		}
		if d := geo.Haversine(lat, lng, t.Lat, t.Lng); d < bestD {
			best, bestD = t, d
		}
	}
	return best
}
