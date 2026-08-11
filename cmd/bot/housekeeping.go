package main

import (
	"context"

	"github.com/ezra-game/server/pkg/geo"
)

// housekeeping exercises the breadth of player actions that aren't part of the
// per-tick combat/build decision: dailies, progression, economy, pets, spire,
// etc. It runs every housekeepEvery ticks, best-effort — each call logs success
// at info and expected rejections (locked features, nothing to claim) at debug.
// This is what makes the bots cover the *whole* API surface like a real player.
func (a *Agent) housekeeping(ctx context.Context, w World) {
	// Core placement happens in the ActPlace path (when the bot is properly on a
	// buildable cell). Here we just upgrade it once it exists.
	if a.placedCore && !a.didOnce("core_upgrade_done") {
		a.try("core_upgrade", a.cli.UpgradeCore(ctx))
	}
	if !a.didOnce("starter_pet") {
		a.try("claim_starter_pet", a.cli.ClaimStarterPet(ctx))
	}
	if !a.didOnce("skills") {
		// Tier-1 MVP skill nodes; the server rejects them when out of points.
		for _, id := range []string{"defender_1", "commander_1", "defender_2"} {
			a.try("unlock_skill:"+id, a.cli.UnlockSkill(ctx, id))
		}
	}
	if !a.didOnce("shop") {
		a.try("shop_add_crystals", a.cli.ShopAddCrystals(ctx, 100))
		if items, err := a.cli.ShopCatalog(ctx); err == nil && len(items) > 0 {
			a.try("shop_buy", a.cli.ShopBuy(ctx, items[0].ID))
		}
	}
	// Recurring dailies / economy / upkeep.
	a.try("streak_checkin", a.cli.StreakCheckin(ctx))
	a.claimReadyQuests(ctx)
	a.workPets(ctx)
	a.maintainTowers(ctx)
	a.try("recruit_survivor", a.cli.RecruitSurvivor(ctx, "fighter", w.curLat(), w.curLng()))
	a.try("spire_contribute", a.cli.SpireContribute(ctx, w.curLat(), w.curLng()))
	a.try("resonance_activate", a.cli.ResonanceActivate(ctx))

	// Symbionts can empower the infection's hives.
	if a.faction == "symbiont" {
		if hives, err := a.cli.GetHives(ctx); err == nil && len(hives) > 0 {
			a.try("empower_hive", a.cli.EmpowerHive(ctx, hives[0].ID))
		}
	}
}

// claimReadyQuests claims every quest whose progress is complete and unclaimed.
func (a *Agent) claimReadyQuests(ctx context.Context) {
	quests, err := a.cli.GetQuests(ctx)
	if err != nil {
		return
	}
	for _, q := range quests {
		if !q.Claimed && q.Progress >= q.Target {
			a.try("claim_quest", a.cli.ClaimQuest(ctx, q.ID))
		}
	}
}

// workPets claims the starter pet's output by sending idle pets on a search task.
func (a *Agent) workPets(ctx context.Context) {
	pets, err := a.cli.GetPets(ctx)
	if err != nil || len(pets) == 0 {
		return
	}
	a.try("send_pet", a.cli.SendPet(ctx, pets[0].ID, "search"))
}

// maintainTowers upgrades and repairs the bot's first owned tower.
func (a *Agent) maintainTowers(ctx context.Context) {
	towers, err := a.cli.TowersMine(ctx)
	if err != nil || len(towers) == 0 {
		return
	}
	a.try("tower_upgrade", a.cli.TowerUpgrade(ctx, towers[0].ID))
	a.try("tower_repair", a.cli.TowerRepair(ctx, towers[0].ID))
}

// try logs an action result: success at info (so a run shows the breadth that
// fired), expected rejections at debug (locked features, nothing to do).
func (a *Agent) try(name string, err error) {
	if err != nil {
		a.log.Debug("hk skip", "act", name, "error", err)
		return
	}
	a.log.Info("hk ok", "act", name)
}

// didOnce returns false the first time it sees a key, true afterwards.
func (a *Agent) didOnce(key string) bool {
	if a.once == nil {
		a.once = map[string]bool{}
	}
	if a.once[key] {
		return true
	}
	a.once[key] = true
	return false
}

// nearestBuildableCellID returns the buildable cell closest to the bot, for core
// placement (which domes the area — creating PvP targets organically). Nearest,
// because the server requires the core cell to be within range of the bot.
func nearestBuildableCellID(w World) string {
	best, bestD := "", 1e18
	lat, lng := w.Self.Position.Lat, w.Self.Position.Lng
	for i := range w.Cells {
		c := &w.Cells[i]
		if !isBuildable(c) {
			continue
		}
		if d := geo.Haversine(lat, lng, c.Lat, c.Lng); d < bestD {
			best, bestD = c.ID, d
		}
	}
	return best
}

// curLat/curLng read the bot's current position out of the world snapshot.
func (w World) curLat() float64 { return w.Self.Position.Lat }
func (w World) curLng() float64 { return w.Self.Position.Lng }
