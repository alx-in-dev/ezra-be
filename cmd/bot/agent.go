package main

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/ezra-game/server/internal/battle"
	"github.com/ezra-game/server/internal/cell"
	"github.com/ezra-game/server/internal/pvp"
	"github.com/ezra-game/server/pkg/geo"
)

// Agent drives a single bot: authenticate, then loop {observe → decide → act}.
// The brain decides *intent*; the agent owns *execution* — crucially, it steps
// movement gradually so positions stay under the server's anti-cheat speed cap.
type Agent struct {
	login string
	cfg   Config
	cli   *Client
	brain Brain
	log   *slog.Logger

	// Last position we successfully reported, used to pace stepped movement.
	curLat, curLng float64

	// squadID is the bot's battle squad, lazily formed on first rift attack.
	squadID string

	// faction is "", "human", or "symbiont". Symbionts scan for breach targets.
	faction string

	// once gates one-time housekeeping actions; hkTick paces the periodic sweep.
	once        map[string]bool
	hkTick      int
	placedCore  bool            // core placement is retried across ticks until it lands
	builtTowers int             // towers built toward the base target
	failedBuild map[string]bool // cells where a build was rejected (too_close, ...)
}

const (
	squadFighters   = 8  // rift closure needs >=5; 8 wins medium rifts more often
	maxBattleRounds = 12 // safety cap so a stuck battle can't loop forever

	// Retreat when squad HP falls to <=1/3 of max and the enemy is still healthy.
	retreatHPNumerator   = 1
	retreatHPDenominator = 3

	// housekeepEvery runs the broad player-action sweep every N decision ticks.
	housekeepEvery = 8

	// Base-building target (brain-agnostic): a core plus this many beacons.
	maxBaseTowers = 3
	buildRangeM   = 35 // must be within the server's placement range of the cell
)

func NewAgent(login string, cfg Config, brain Brain, spawnLat, spawnLng float64) *Agent {
	return &Agent{
		login:   login,
		cfg:     cfg,
		cli:     NewClient(cfg.BaseURL),
		brain:   brain,
		log:     slog.With("bot", login),
		curLat:  spawnLat,
		curLng:  spawnLng,
		faction: cfg.Faction,
	}
}

// Run blocks until ctx is cancelled. Transient errors are logged, not fatal —
// one bot stumbling must not take down the swarm.
func (a *Agent) Run(ctx context.Context) {
	if err := a.cli.EnsureAuth(ctx, a.login, a.cfg.Password); err != nil {
		a.log.Error("auth failed, bot will not start", "error", err)
		return
	}
	a.log.Info("authenticated", "brain", a.brain.Name())

	// Declare allegiance if configured. May fail with not_contacted (the server
	// gates side choice behind closing a hive); that's fine — we log and go on.
	if a.faction != "" {
		if err := a.cli.ChooseFaction(ctx, a.faction); err != nil {
			a.log.Warn("faction choose failed", "side", a.faction, "error", err)
		} else {
			a.log.Info("faction chosen", "side", a.faction)
		}
	}

	// Plant the bot at its spawn point so the first decision has a real location.
	if err := a.cli.UpdatePosition(ctx, a.curLat, a.curLng); err != nil {
		a.log.Warn("initial position set failed", "error", err)
	}

	ticker := time.NewTicker(a.cfg.Tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			a.log.Info("stopping")
			return
		case <-ticker.C:
			a.step(ctx)
		}
	}
}

func (a *Agent) step(ctx context.Context) {
	self, err := a.cli.GetPlayer(ctx)
	if err != nil {
		a.log.Warn("get player failed", "error", err)
		return
	}
	// Trust the server's position as ground truth between ticks.
	if self.Position.Lat != 0 || self.Position.Lng != 0 {
		a.curLat, a.curLng = self.Position.Lat, self.Position.Lng
	}

	cells, err := a.cli.GetCells(ctx, a.curLat, a.curLng, 1.5)
	if err != nil {
		a.log.Warn("get cells failed", "error", err)
		return
	}

	world := World{Self: self, Cells: cells}
	// Symbionts scan for nearby enemy domes to breach.
	if a.faction == "symbiont" {
		if targets, terr := a.cli.PvpTargets(ctx, a.curLat, a.curLng); terr == nil {
			world.PvpTargets = targets
		}
	}

	// Base-building is the agent's job (brain-agnostic): establish a core + a few
	// beacons before handing tactical control to the brain. This guarantees even
	// an LLM bot builds a base — the brain then only drives combat/raiding.
	if a.tryEstablishBase(ctx, world) {
		a.hkTick++
		if a.hkTick%housekeepEvery == 0 {
			a.housekeeping(ctx, world)
		}
		return
	}

	action := a.brain.Decide(ctx, world)

	var actErr error
	switch action.Kind {
	case ActMoveTo:
		a.moveToward(ctx, action.Lat, action.Lng, action.Reason)
	case ActPlace:
		// First structure is the network Core (it domes the area — which makes
		// this bot a PvP target). We're standing on a buildable cell, so the
		// core cell is within range. After that, ActPlace builds towers.
		if !a.placedCore {
			if cid := nearestBuildableCellID(world); cid != "" && a.cli.PlaceCore(ctx, cid) == nil {
				a.placedCore = true
				a.log.Info("placed core", "lat", a.curLat, "lng", a.curLng)
				break
			}
		}
		if actErr = a.cli.PlaceTower(ctx, ""); actErr != nil {
			a.log.Warn("place tower failed", "reason", action.Reason, "error", actErr)
		} else {
			a.log.Info("placed tower", "reason", action.Reason, "lat", a.curLat, "lng", a.curLng)
		}
	case ActCapture:
		if actErr = a.cli.Lockpick(ctx, action.TowerID); actErr != nil {
			a.log.Warn("lockpick failed", "tower", action.TowerID, "error", actErr)
		} else {
			a.log.Info("lockpicked tower", "tower", action.TowerID, "reason", action.Reason)
		}
	case ActAttackRift:
		// Guard + target resolution (brain-agnostic). The LLM often hallucinates
		// ids — validate against the world and honour the intent with a real one.
		if self.Energy < battleMinEnergy {
			a.log.Debug("attack skipped: low energy", "energy", self.Energy)
			break
		}
		rid := resolveRift(action.RiftID, world)
		if rid == "" {
			a.log.Debug("attack skipped: no rift in view")
			break
		}
		a.fightRift(ctx, rid)
	case ActBreach:
		// Breach is symbiont-only, costs energy, and needs a *real* enemy dome.
		// The LLM may emit breach for a human or with a bogus cell id — guard the
		// rules and snap the target to an actual pvp_target instead of 403/400.
		switch {
		case a.faction != "symbiont":
			a.log.Debug("breach skipped: not a symbiont")
		case self.Energy < pvp.BreachEnergyCost:
			a.log.Debug("breach skipped: low energy", "energy", self.Energy)
		default:
			cellID := resolveBreachTarget(action.CellID, world)
			if cellID == "" {
				a.log.Debug("breach skipped: no enemy dome in range")
				break
			}
			res, err := a.cli.PvpBreach(ctx, cellID)
			if actErr = err; actErr != nil {
				a.log.Warn("breach failed", "cell", cellID, "error", actErr)
			} else {
				a.log.Info("breached dome", "cell", cellID, "rift", res.RiftID, "defender", res.DefenderName)
			}
		}
	case ActIdle:
		a.log.Debug("idle", "reason", action.Reason)
	}
	a.brain.Observe(action, actErr)

	// Periodically exercise the broad set of player actions (dailies, economy,
	// pets, spire, ...) so the bot covers the whole API, not just combat/build.
	a.hkTick++
	if a.hkTick%housekeepEvery == 0 {
		a.housekeeping(ctx, world)
	}
}

// fightRift forms a squad if needed, starts a rift battle, then plays rounds to
// completion. Tactic each round follows the server's weakness hint (the same
// signal the real UI surfaces), falling back to a plain attack.
func (a *Agent) fightRift(ctx context.Context, riftID string) {
	squadID, err := a.ensureSquad(ctx)
	if err != nil {
		a.log.Warn("cannot fight: no squad", "error", err)
		return
	}

	snap, err := a.cli.StartBattle(ctx, squadID, "rift", riftID)
	if err != nil {
		// Squad depleted below the fighter minimum (units died in prior fights)?
		// Drop the cached squad so the next attempt re-forms a full one.
		if strings.Contains(err.Error(), "insufficient_fighters") {
			a.squadID = ""
		}
		a.log.Warn("battle start failed", "rift", riftID, "error", err)
		return
	}
	a.log.Info("battle started", "rift", riftID, "enemy_hp", snap.EnemyHP, "weakness", snap.Enemy.Weakness)

	for round := 0; snap.Status == "ongoing" && round < maxBattleRounds; round++ {
		tactic := chooseTactic(snap)
		snap, err = a.cli.BattleAction(ctx, snap.BattleID, tactic)
		if err != nil {
			a.log.Warn("battle action failed", "battle", snap.BattleID, "tactic", tactic, "error", err)
			return
		}
		a.log.Debug("battle round", "tactic", tactic, "enemy_hp", snap.EnemyHP, "squad_hp", snap.SquadHP, "status", snap.Status)
	}
	a.log.Info("battle finished", "rift", riftID, "result", snap.Status, "rounds", snap.Round)
}

// chooseTactic picks the round tactic. It bails out (retreat) of a fight we're
// clearly losing — retreat keeps the fighters alive and refunds half the energy,
// so the squad survives for the next fight instead of being wiped (which forced
// a costly full re-recruit). Otherwise it plays the server's weakness hint.
func chooseTactic(snap battle.Snapshot) string {
	losing := snap.Squad.HPMax > 0 &&
		snap.SquadHP <= snap.Squad.HPMax*retreatHPNumerator/retreatHPDenominator &&
		snap.EnemyHP*4 > snap.Enemy.HPMax // enemy still above ~25% — not a fight we'll finish
	if losing {
		return "retreat"
	}
	if snap.Enemy.Weakness != "" {
		return snap.Enemy.Weakness
	}
	return "attack"
}

// tryEstablishBase drives core + beacon construction independent of the brain.
// Returns true if it issued a build (or a move toward a build site) this tick,
// in which case the caller skips the brain's tactical decision. Symbionts opt
// out — their job is raiding, not base-building.
func (a *Agent) tryEstablishBase(ctx context.Context, w World) bool {
	if a.faction == "symbiont" {
		return false
	}
	if a.failedBuild == nil {
		a.failedBuild = map[string]bool{}
	}

	needCore := !a.placedCore
	needTower := a.placedCore && a.builtTowers < maxBaseTowers && w.Self.Materials >= placeMaterialsCost
	if !needCore && !needTower {
		return false // base established — hand control to the brain
	}

	c := a.nearestBuildSite(w)
	if c == nil {
		return false // nothing buildable in view — let the brain move us around
	}
	if geo.Haversine(a.curLat, a.curLng, c.Lat, c.Lng) > buildRangeM {
		a.moveToward(ctx, c.Lat, c.Lng, "establish base")
		return true
	}

	if needCore {
		if err := a.cli.PlaceCore(ctx, c.ID); err != nil {
			if strings.Contains(err.Error(), "core_exists") {
				a.placedCore = true // already have a core (e.g. prior run) → build towers
			} else {
				a.failedBuild[c.ID] = true
				a.log.Debug("core place failed", "cell", c.ID, "error", err)
			}
		} else {
			a.placedCore = true
			a.log.Info("placed core", "cell", c.ID)
		}
		return true
	}
	if err := a.cli.PlaceTower(ctx, c.ID); err != nil {
		a.failedBuild[c.ID] = true
		a.log.Debug("tower place failed", "cell", c.ID, "error", err)
	} else {
		a.builtTowers++
		a.log.Info("placed tower", "cell", c.ID, "have", a.builtTowers)
	}
	return true
}

// nearestBuildSite returns the closest free, power-sourced cell we haven't
// already failed to build on.
func (a *Agent) nearestBuildSite(w World) *cell.CellDTO {
	var best *cell.CellDTO
	bestD := 1e18
	for i := range w.Cells {
		c := &w.Cells[i]
		if c.TowerID != nil || !(c.HasNearbyRoad || c.HasNearbyBuilding) || a.failedBuild[c.ID] {
			continue
		}
		if d := geo.Haversine(a.curLat, a.curLng, c.Lat, c.Lng); d < bestD {
			best, bestD = c, d
		}
	}
	return best
}

// resolveRift returns id if it's a rift visible in the world; otherwise the
// first visible rift (honour the "attack" intent despite a hallucinated id), or
// "" if there's nothing to attack.
func resolveRift(id string, w World) string {
	var first string
	for i := range w.Cells {
		if rid := w.Cells[i].RiftID; rid != nil {
			if *rid == id {
				return id
			}
			if first == "" {
				first = *rid
			}
		}
	}
	return first
}

// resolveBreachTarget returns cellID if it's a real nearby enemy dome; otherwise
// the first pvp target, or "" if there are none.
func resolveBreachTarget(cellID string, w World) string {
	for i := range w.PvpTargets {
		if w.PvpTargets[i].CellID == cellID {
			return cellID
		}
	}
	if len(w.PvpTargets) > 0 {
		return w.PvpTargets[0].CellID
	}
	return ""
}

// ensureSquad returns the bot's battle squad, forming one (recruiting fighters
// as needed) on first use. Cached for the bot's lifetime.
func (a *Agent) ensureSquad(ctx context.Context) (string, error) {
	if a.squadID != "" {
		return a.squadID, nil
	}

	// Reuse an existing squad with enough fighters if one already exists.
	if squads, err := a.cli.ListSquads(ctx); err == nil {
		for _, sq := range squads {
			if sq.FighterCount >= squadFighters {
				a.squadID = sq.ID
				return a.squadID, nil
			}
		}
	}

	// Gather idle (unassigned) fighters, recruiting more until we have enough.
	fighterIDs, err := a.idleFighters(ctx)
	if err != nil {
		return "", err
	}
	for len(fighterIDs) < squadFighters {
		u, err := a.cli.RecruitUnit(ctx, "fighter")
		if err != nil {
			return "", fmt.Errorf("recruit fighter (%d/%d): %w", len(fighterIDs), squadFighters, err)
		}
		fighterIDs = append(fighterIDs, u.ID)
		a.log.Debug("recruited fighter", "unit", u.ID, "have", len(fighterIDs))
	}

	sq, err := a.cli.CreateSquad(ctx, "strike", fighterIDs[:squadFighters])
	if err != nil {
		return "", fmt.Errorf("create squad: %w", err)
	}
	a.squadID = sq.ID
	a.log.Info("formed squad", "squad", sq.ID, "fighters", squadFighters)
	return a.squadID, nil
}

// idleFighters returns the IDs of the bot's unassigned fighter units.
func (a *Agent) idleFighters(ctx context.Context) ([]string, error) {
	units, err := a.cli.ListUnits(ctx)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, u := range units {
		if u.Type == "fighter" && u.SquadID == nil {
			ids = append(ids, u.ID)
		}
	}
	return ids, nil
}

// moveToward steps at most one tick's worth of distance toward the target and
// reports the new position. Linear interpolation in lat/lng is accurate enough
// at these scales (sub-kilometre steps).
func (a *Agent) moveToward(ctx context.Context, destLat, destLng float64, reason string) {
	maxStep := a.cfg.WalkSpeedMS * a.cfg.Tick.Seconds() // metres this tick
	dist := geo.Haversine(a.curLat, a.curLng, destLat, destLng)

	nextLat, nextLng := destLat, destLng
	if dist > maxStep {
		f := maxStep / dist
		nextLat = a.curLat + (destLat-a.curLat)*f
		nextLng = a.curLng + (destLng-a.curLng)*f
	}

	if math.IsNaN(nextLat) || math.IsNaN(nextLng) {
		return
	}
	if err := a.cli.UpdatePosition(ctx, nextLat, nextLng); err != nil {
		a.log.Warn("move failed", "reason", reason, "error", err)
		return
	}
	a.curLat, a.curLng = nextLat, nextLng
	a.log.Debug("moved", "reason", reason, "lat", nextLat, "lng", nextLng, "remaining_m", dist)
}
