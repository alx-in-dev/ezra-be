package battle

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"time"

	"github.com/ezra-game/server/internal/canon"
	"github.com/ezra-game/server/internal/factionwar"
	"github.com/ezra-game/server/internal/hive"
	"github.com/ezra-game/server/internal/item"
	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/internal/rift"
	"github.com/ezra-game/server/internal/unit"
	"github.com/ezra-game/server/pkg/httputil"
)

// Combat tuning. Base tactic multipliers (the dials between damage and risk),
// then spirit weaknesses (C3) and unit roles (C4) layer on top. Kept here next
// to the combat logic — same place the original literals lived.
const (
	attackDealtMult = 1.3
	defendTakenMult = 0.6
	energyDealtMult = 1.5
	energyTakenMult = 1.15

	// C3 — spirit weaknesses (docs/02_core_gameplay.md §2.4). The matching
	// tactic is super-effective against that share of the enemy: an energy
	// burst tears through a swarm of small spirits; a defensive formation
	// blunts a humanoid's heavy blows.
	energyVsSwarmDealtBonus  = 0.4
	defendVsHumanoidTakenCut = 0.3

	// C4 — unit roles in battle (docs/feature/battle_system.md "what's
	// missing"). Scouts add first-strike damage, engineers fortify (reduce
	// damage taken), medics offset damage with field medicine. Capped so a
	// stacked squad can't trivialise combat.
	scoutDealtPerUnit         = 0.05
	scoutDealtCap             = 0.5
	engineerMitigationPerUnit = 0.05
	engineerMitigationCap     = 0.5
	medicHealPerUnit          = 5
)

// SquadUnit holds pre-fetched unit stats for battle calculations.
type SquadUnit struct {
	ID   string
	Type string
	ATK  int
	HP   int
}

// SquadProvider retrieves units assigned to a squad.
type SquadProvider interface {
	GetSquadOwner(ctx context.Context, squadID string) (string, error)
	GetSquadUnits(ctx context.Context, squadID string) ([]SquadUnit, error)
	StartMission(ctx context.Context, squadID, targetType, targetID string) error
	FinishMission(ctx context.Context, squadID string) error
	MarkUnitsLost(ctx context.Context, unitIDs []string) error
}

// RiftProvider returns enemy data and can close a completed rift.
type RiftProvider interface {
	GetByID(ctx context.Context, id string) (*rift.Rift, error)
	Close(ctx context.Context, id string) error
}

type ResourceRewarder interface {
	Add(ctx context.Context, playerID string, energy, materials int) (*player.Player, error)
	Spend(ctx context.Context, playerID string, energy, materials int) (*player.Player, error)
}

type ProgressionRewarder interface {
	AddXP(ctx context.Context, playerID string, amount int) (*player.Player, error)
}

type PlayerProvider interface {
	GetByID(ctx context.Context, id string) (*player.Player, error)
	Update(ctx context.Context, p *player.Player) error
}

// UnitProgressor awards battle XP to a squad's units (D3). Optional dependency
// (set via SetUnitProgressor); nil in unit tests, which skip unit leveling.
type UnitProgressor interface {
	AwardSquadBattleXP(ctx context.Context, playerID, squadID string, xpEach int) ([]unit.UnitLevelUp, error)
}

// ItemGranter grants inventory items as epic loot (D5). Optional dependency
// (set via SetItemGranter); nil in unit tests.
type ItemGranter interface {
	Grant(ctx context.Context, playerID, itemType, variant string, delta int) (int, error)
}

// Discoverer records bestiary discoveries (spirit/rift types) when a battle is
// engaged (T-660). Optional (set via SetDiscoverer); nil in unit tests.
type Discoverer interface {
	Discover(ctx context.Context, playerID string, keys []string) error
}

// BlueprintRoller is optionally implemented by the inventory service to do a
// pity-aware blueprint-fragment roll (audit C2). When the wired ItemGranter
// also satisfies this, the battle service defers the rare-drop decision to it;
// otherwise it falls back to a flat per-tier roll.
type BlueprintRoller interface {
	RollBlueprintFragment(ctx context.Context, playerID string, base, roll float64) (bool, error)
}

// HiveProvider lets a squad assault a Symbiont hive (E1 deepening). Optional
// dependency (set via SetHiveProvider); nil in unit tests.
type HiveProvider interface {
	GetByID(ctx context.Context, id string) (*hive.Hive, error)
	OnAssaultVictory(ctx context.Context, hiveID string) (closed bool, err error)
}

// FactionScorer credits Human faction-war points for victories (E2 slice).
// Optional dependency (set via SetFactionScorer).
type FactionScorer interface {
	AwardHuman(ctx context.Context, playerID string, points int) error
}

// FactionContacter records the player's "Contact with Эзра" (E3), unlocking the
// faction-side choice. Optional dependency.
type FactionContacter interface {
	MarkContacted(ctx context.Context, playerID string) error
	// EnterTemporarySymbiont flips the player into a temporary symbiont
	// (faction=symbiont, chosen=false) and seeds the Resonance roster, for the
	// onboarding symbiont tutorial. The final faction choice confirms or reverts
	// it. See docs/feature/symbiont_onboarding.md.
	EnterTemporarySymbiont(ctx context.Context, playerID string) error
}

// RiftDailyLimiter caps how many rifts of a tier a player can close per day
// (anti-farm / anti-inflation — docs/06_economy.md §6.3). ClosuresToday is read
// at engagement to gate Start; RecordClosure is called on victory so only
// completed farms count. Optional dependency (SetRiftLimiter); nil → no cap.
type RiftDailyLimiter interface {
	ClosuresToday(ctx context.Context, playerID, riftType string) (int, error)
	RecordClosure(ctx context.Context, playerID, riftType string) error
}

// UnitBattleXPShare is the fraction of a battle's player-XP each surviving unit
// earns toward its own level (D3 — keeps unit progress in step with the
// encounter tier without a separate balance table).
const UnitBattleXPShare = 0.12

// blueprintDropChance is the per-rift-tier chance to drop a Spire blueprint
// fragment (megastructure_spire.md: 0.5% baseline, scaling with tier).
var blueprintDropChance = map[string]float64{
	"minor":    0.005,
	"medium":   0.02,
	"critical": 0.05,
}

// RoundTimeout is the maximum time between rounds before auto-retreat.
const RoundTimeout = 60 * time.Second

// Service implements battle business logic.
type Service struct {
	battles     Repository
	rifts       RiftProvider
	squads      SquadProvider
	resources   ResourceRewarder
	progression ProgressionRewarder
	players     PlayerProvider
	units       UnitProgressor
	inventory   ItemGranter
	discoverer  Discoverer
	hives       HiveProvider
	faction     FactionScorer
	contacter   FactionContacter
	riftLimiter RiftDailyLimiter
	quests      interface {
		UpdateProgress(ctx context.Context, playerID, eventType string) error
	}
	// factionCheck gates battle entry to Humans (T-800). nil-safe: without it
	// the gate is skipped. Distinct from faction (FactionScorer, which awards
	// war points and stays even for Symbionts' own scoring path).
	factionCheck FactionChecker
}

// FactionChecker reports a player's faction for the human-toolkit exclusion
// gate (T-800: Symbionts don't fight normal PvE battles).
type FactionChecker interface {
	IsSymbiont(ctx context.Context, playerID string) (bool, error)
}

// SetFactionChecker wires the optional faction gate. Kept off the
// constructor so existing callers/tests stay unchanged.
func (s *Service) SetFactionChecker(f FactionChecker) {
	s.factionCheck = f
}

// SetRiftLimiter wires the optional daily rift-closure cap (anti-inflation).
// Kept off the constructor so existing callers/tests stay unchanged.
func (s *Service) SetRiftLimiter(l RiftDailyLimiter) { s.riftLimiter = l }

// SetHiveProvider wires the optional hive-assault dependency (E1 deepening).
func (s *Service) SetHiveProvider(h HiveProvider) { s.hives = h }

// SetFactionScorer wires the optional faction-war scorer (E2 slice).
func (s *Service) SetFactionScorer(f FactionScorer) { s.faction = f }

// SetFactionContacter wires the optional E3 Contact trigger.
func (s *Service) SetFactionContacter(c FactionContacter) { s.contacter = c }

// SetUnitProgressor wires the optional unit-leveling dependency (D3). Kept off
// the constructor so existing callers/tests stay unchanged.
func (s *Service) SetUnitProgressor(u UnitProgressor) {
	s.units = u
}

// SetItemGranter wires the optional epic-loot inventory dependency (D5).
func (s *Service) SetItemGranter(g ItemGranter) {
	s.inventory = g
}

// SetDiscoverer wires the optional bestiary discovery hook (T-660).
func (s *Service) SetDiscoverer(d Discoverer) {
	s.discoverer = d
}

// recordDiscoveries marks the encounter's spirit types (and rift tier) as
// discovered for the bestiary. Best-effort, called on battle engagement.
func (s *Service) recordDiscoveries(ctx context.Context, playerID string, b *Battle) {
	if s.discoverer == nil {
		return
	}
	spirits, encType, _, err := s.loadEncounter(ctx, b)
	if err != nil {
		return
	}
	seen := map[string]bool{}
	keys := make([]string, 0, 4)
	for _, sp := range spirits {
		k := "spirit:" + sp.Type
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	if b.TargetType == "rift" && encType != "" {
		keys = append(keys, "rift:"+encType)
	}
	if err := s.discoverer.Discover(ctx, playerID, keys); err != nil {
		slog.Warn("bestiary discover failed", "player_id", playerID, "error", err)
	}
}

// rollEpicSpectrum picks a resonance spectrum for a shard-boss drop. Varied by
// the battle id so it isn't constant (scripts can't use rand seeds here).
func rollEpicSpectrum(battleID string) string {
	spectra := item.ResonanceSpectra
	if len(spectra) == 0 {
		return ""
	}
	sum := 0
	for _, c := range battleID {
		sum += int(c)
	}
	return spectra[(sum+rand.Intn(len(spectra)))%len(spectra)]
}

// NewService creates a new battle service.
func NewService(battles Repository, rifts RiftProvider, squads SquadProvider, resources ResourceRewarder, progression ProgressionRewarder, players PlayerProvider, quests interface {
	UpdateProgress(ctx context.Context, playerID, eventType string) error
}) *Service {
	return &Service{
		battles:     battles,
		rifts:       rifts,
		squads:      squads,
		resources:   resources,
		progression: progression,
		players:     players,
		quests:      quests,
	}
}

// Start initiates a new battle for a player's squad against a target.
func (s *Service) Start(ctx context.Context, playerID, squadID, targetType, targetID string) (*Battle, error) {
	if s.factionCheck != nil {
		if isSymbiont, err := s.factionCheck.IsSymbiont(ctx, playerID); err == nil && isSymbiont {
			return nil, httputil.NewForbidden("symbiont_no_human_toolkit", "симбионт не может вести обычный бой")
		}
	}
	// Validate squad ownership
	ownerID, err := s.squads.GetSquadOwner(ctx, squadID)
	if err != nil {
		return nil, httputil.NewNotFound("not_found", "squad not found")
	}
	if ownerID != playerID {
		return nil, httputil.NewForbidden("not_owner", "not your squad")
	}

	units, err := s.squads.GetSquadUnits(ctx, squadID)
	if err != nil {
		return nil, fmt.Errorf("get squad units: %w", err)
	}
	switch targetType {
	case "rift":
		fighterCount := 0
		for _, u := range units {
			if u.Type == "fighter" {
				fighterCount++
			}
		}
		if fighterCount < 5 {
			return nil, httputil.NewBadRequest("insufficient_fighters", "rift closure requires at least 5 fighters")
		}
		// Daily farm cap (anti-inflation, docs/06_economy.md §6.3): refuse to
		// engage another rift of this tier once today's closures hit the cap.
		// Checked before charging energy so a capped player isn't billed.
		if s.riftLimiter != nil {
			r, err := s.rifts.GetByID(ctx, targetID)
			if err != nil || r == nil {
				return nil, httputil.NewNotFound("not_found", "rift not found")
			}
			if capN := canon.RiftDailyCap(r.Type); capN > 0 {
				if used, lerr := s.riftLimiter.ClosuresToday(ctx, playerID, r.Type); lerr == nil && used >= capN {
					return nil, httputil.NewBadRequest("rift_daily_cap",
						fmt.Sprintf("дневной лимит разломов «%s» исчерпан (%d/%d) — возвращайся завтра", r.Type, used, capN))
				}
			}
		}
	case "hive":
		if s.hives == nil {
			return nil, httputil.NewBadRequest("feature_locked", "hive assault is unavailable")
		}
		fighterCount := 0
		for _, u := range units {
			if u.Type == "fighter" {
				fighterCount++
			}
		}
		// Assaulting the infection root is the hardest fight — demand a full squad.
		if fighterCount < 5 {
			return nil, httputil.NewBadRequest("insufficient_fighters", "штурм Очага требует минимум 5 бойцов")
		}
		if _, err := s.hives.GetByID(ctx, targetID); err != nil {
			return nil, httputil.NewNotFound("not_found", "hive not found")
		}
	case "tutorial":
		if len(units) < 2 {
			return nil, httputil.NewBadRequest("insufficient_units", "tutorial battle requires at least 2 units")
		}
		if s.players == nil {
			return nil, httputil.NewBadRequest("feature_locked", "tutorial battle is unavailable")
		}
		p, err := s.players.GetByID(ctx, playerID)
		if err != nil {
			return nil, fmt.Errorf("get player: %w", err)
		}
		if p.OnboardingStep != canon.OnboardingTutorialBattle {
			return nil, httputil.NewBadRequest("feature_locked", "tutorial battle is not available right now")
		}
	default:
		return nil, httputil.NewBadRequest("invalid_target", "target_type must be 'rift', 'hive' or 'tutorial'")
	}

	b := &Battle{
		AttackerID: playerID,
		SquadID:    squadID,
		TargetType: targetType,
		TargetID:   targetID,
		Rounds:     []Round{},
		Status:     canon.BattleStateAwaitingTactic,
	}

	// Engagement cost (balance_tables.md): charged up front to enter combat,
	// forfeited on defeat, half-refunded on retreat. Tutorial battles are free.
	// Skipped when no resource service is wired (unit tests).
	cost := 0
	if s.resources != nil {
		cost = s.encounterEnergyCost(ctx, targetType, targetID)
		if cost > 0 {
			if _, err := s.resources.Spend(ctx, playerID, cost, 0); err != nil {
				return nil, err
			}
		}
	}

	if err := s.squads.StartMission(ctx, squadID, targetType, targetID); err != nil {
		s.refundEnergy(ctx, playerID, cost)
		if appErr, ok := err.(*httputil.AppError); ok {
			return nil, appErr
		}
		return nil, fmt.Errorf("start squad mission: %w", err)
	}
	if err := s.battles.Create(ctx, b); err != nil {
		_ = s.squads.FinishMission(ctx, squadID)
		s.refundEnergy(ctx, playerID, cost)
		return nil, fmt.Errorf("create battle: %w", err)
	}
	s.recordDiscoveries(ctx, playerID, b) // T-660 bestiary: log what you fought
	return b, nil
}

// HiveAssaultEnergyCost is the ⚡ price to launch one assault on a hive — the
// priciest engagement, befitting an attack on the infection root.
const HiveAssaultEnergyCost = 150

// encounterEnergyCost is the ⚡ price to engage the target. Rifts cost by type;
// hive assaults are a flat premium; tutorial is free.
func (s *Service) encounterEnergyCost(ctx context.Context, targetType, targetID string) int {
	if targetType == "hive" {
		return HiveAssaultEnergyCost
	}
	if targetType != "rift" {
		return 0
	}
	r, err := s.rifts.GetByID(ctx, targetID)
	if err != nil || r == nil {
		return canon.BattleEnergyCost("minor")
	}
	return canon.BattleEnergyCost(r.Type)
}

// refundEnergy returns energy to the player (best-effort) — used to roll back
// the engagement charge when a battle fails to start, and for retreat refunds.
func (s *Service) refundEnergy(ctx context.Context, playerID string, energy int) {
	if energy <= 0 || s.resources == nil {
		return
	}
	if _, err := s.resources.Add(ctx, playerID, energy, 0); err != nil {
		slog.Error("battle energy refund failed", "player_id", playerID, "energy", energy, "error", err)
	}
}

func tutorialEncounter(targetID string) ([]Spirit, string, map[string]any) {
	spirits := []Spirit{
		SpiritConfigs["small"],
		SpiritConfigs["small"],
	}
	loot := map[string]any{
		"energy":    40,
		"materials": 8,
		"xp":        40,
	}
	if targetID == "" {
		targetID = "starter"
	}
	return spirits, "tutorial", loot
}

func (s *Service) loadEncounter(ctx context.Context, b *Battle) ([]Spirit, string, map[string]any, error) {
	switch b.TargetType {
	case "tutorial":
		spirits, encounterType, loot := tutorialEncounter(b.TargetID)
		return spirits, encounterType, loot, nil
	case "rift":
		r, err := s.rifts.GetByID(ctx, b.TargetID)
		if err != nil {
			return nil, "", nil, fmt.Errorf("get rift: %w", err)
		}
		spirits := generateSpirits(r)
		s.scaleRiftToPlayer(ctx, b.AttackerID, spirits) // E4: gentle player-level scaling
		return spirits, r.Type, buildRiftLoot(r), nil
	case "hive":
		h, err := s.hives.GetByID(ctx, b.TargetID)
		if err != nil {
			return nil, "", nil, fmt.Errorf("get hive: %w", err)
		}
		return hiveDefenders(h.Level), "hive", hiveLoot(h.Level), nil
	default:
		return nil, "", nil, httputil.NewBadRequest("invalid_target", "unsupported battle target")
	}
}

// hiveDefenders builds the Symbiont garrison guarding a hive — tougher than any
// rift: at least one shard boss anchoring humanoids and a swarm, scaling with
// hive level (a second boss at L3).
func hiveDefenders(level int) []Spirit {
	if level < 1 {
		level = 1
	}
	spirits := []Spirit{SpiritConfigs["shard"]}
	for i := 0; i < 1+level; i++ {
		spirits = append(spirits, SpiritConfigs["humanoid"])
	}
	for i := 0; i < 4+2*level; i++ {
		spirits = append(spirits, SpiritConfigs["small"])
	}
	if level >= 3 {
		spirits = append(spirits, SpiritConfigs["shard"])
	}
	return spirits
}

// hiveLoot is the per-assault reward. "tier":"hive" marks the encounter so
// onBattleResolved routes the assault to the hive lifecycle.
func hiveLoot(level int) map[string]any {
	return map[string]any{
		"energy":    400 + 200*level,
		"materials": 80 + 40*level,
		"xp":        500,
		"tier":      "hive",
	}
}

func (s *Service) completeTutorialBattle(ctx context.Context, playerID string) (*player.Player, error) {
	if s.players == nil {
		return nil, nil
	}
	p, err := s.players.GetByID(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("get player: %w", err)
	}
	if p.OnboardingStep == canon.OnboardingTutorialBattle {
		p.OnboardingStep = canon.OnboardingPetUnlockStep
		if err := s.players.Update(ctx, p); err != nil {
			return nil, fmt.Errorf("update player onboarding: %w", err)
		}
	}
	return p, nil
}

// completeContactRift fires the onboarding "Contact with Эзра" beat. When the
// player closes a rift while on contact_step, they experience Contact and turn
// into a temporary symbiont, advancing into the symbiont tutorial. Best-effort:
// any failure is logged but never fails the battle resolution. No-op otherwise.
func (s *Service) completeContactRift(ctx context.Context, playerID string) {
	if s.players == nil || s.contacter == nil {
		return
	}
	p, err := s.players.GetByID(ctx, playerID)
	if err != nil || p == nil || p.OnboardingStep != canon.OnboardingContactStep {
		return
	}
	if err := s.contacter.MarkContacted(ctx, playerID); err != nil {
		slog.Error("contact: mark contacted failed", "player_id", playerID, "error", err)
		return
	}
	if err := s.contacter.EnterTemporarySymbiont(ctx, playerID); err != nil {
		slog.Error("contact: enter temporary symbiont failed", "player_id", playerID, "error", err)
		return
	}
	p.OnboardingStep = canon.OnboardingSymbiontIntro
	if err := s.players.Update(ctx, p); err != nil {
		slog.Error("contact: advance onboarding failed", "player_id", playerID, "error", err)
	}
}

// scaleRiftToPlayer applies E4 player-level difficulty scaling to a rift's
// spirits in place (HP/ATK). No-op when no player provider is wired (unit
// tests) or the player can't be loaded — the base roster then stands.
func (s *Service) scaleRiftToPlayer(ctx context.Context, playerID string, spirits []Spirit) {
	if s.players == nil || playerID == "" {
		return
	}
	p, err := s.players.GetByID(ctx, playerID)
	if err != nil || p == nil {
		return
	}
	factor := canon.RiftDifficultyScale(p.Level)
	if factor == 1.0 {
		return
	}
	for i := range spirits {
		if hp := int(math.Round(float64(spirits[i].HP) * factor)); hp > 0 {
			spirits[i].HP = hp
		} else {
			spirits[i].HP = 1
		}
		if atk := int(math.Round(float64(spirits[i].ATK) * factor)); atk > 0 {
			spirits[i].ATK = atk
		} else {
			spirits[i].ATK = 1
		}
	}
}

// generateSpirits creates a deterministic enemy roster from persisted rift data.
func generateSpirits(r *rift.Rift) []Spirit {
	cfg, ok := rift.TypeConfigs[r.Type]
	if !ok {
		cfg = rift.TypeConfigs["minor"]
	}

	count := r.SpiritsCount
	if count <= 0 {
		count = cfg.MinSpirits
	}
	spirits := make([]Spirit, 0, count)

	switch r.Type {
	case "critical":
		// Swarm + shard (III boss): 1 shard + 1 humanoid lieutenant + rest small.
		spirits = append(spirits, SpiritConfigs["shard"])
		if count > 1 {
			spirits = append(spirits, SpiritConfigs["humanoid"])
		}
		for i := 2; i < count; i++ {
			spirits = append(spirits, SpiritConfigs["small"])
		}
	case "medium":
		// 1 humanoid + rest small
		spirits = append(spirits, SpiritConfigs["humanoid"])
		for i := 1; i < count; i++ {
			spirits = append(spirits, SpiritConfigs["small"])
		}
	default:
		// all small
		for i := 0; i < count; i++ {
			spirits = append(spirits, SpiritConfigs["small"])
		}
	}

	// Intensity hardens the roster: an aged / entity-amplified / empowered
	// rift fights harder. Intensity was written by three systems (expansion,
	// Empower, scout/absorber entities) and read by none — this is where it
	// lands. 1 → ×1.0, 100 → ×1.5 on HP/ATK.
	if mult := riftIntensityMultiplier(r.Intensity); mult > 1.0 {
		for i := range spirits {
			spirits[i].HP = int(float64(spirits[i].HP) * mult)
			spirits[i].ATK = int(float64(spirits[i].ATK) * mult)
		}
	}
	return spirits
}

// riftIntensityMultiplier maps rift intensity (1..100) to a combat/infection
// scale of ×1.0..×1.5.
func riftIntensityMultiplier(intensity int) float64 {
	if intensity < 1 {
		intensity = 1
	}
	if intensity > 100 {
		intensity = 100
	}
	return 1.0 + float64(intensity-1)/198.0
}

// computeEnemyStats returns total HP and ATK for spirits.
func computeEnemyStats(spirits []Spirit) (totalHP, totalATK int) {
	for _, sp := range spirits {
		totalHP += sp.HP
		totalATK += sp.ATK
	}
	return
}

// computeSquadStats returns total HP and ATK for squad units.
func computeSquadStats(units []SquadUnit) (totalHP, totalATK int) {
	for _, u := range units {
		totalHP += u.HP
		totalATK += u.ATK
	}
	return
}

// enemyTypeShares returns the fraction of total enemy HP made up of small
// (swarm) and humanoid spirits — drives the C3 weakness bonuses.
func enemyTypeShares(spirits []Spirit) (smallShare, humanoidShare float64) {
	var small, humanoid, total float64
	for _, sp := range spirits {
		switch sp.Type {
		case "humanoid", "shard":
			// The shard boss is elite armour, not swarm — anti-humanoid tactics
			// bite it (C3 weakness), swarm-clear tactics don't.
			humanoid += float64(sp.HP)
		default:
			small += float64(sp.HP)
		}
		total += float64(sp.HP)
	}
	if total == 0 {
		return 0, 0
	}
	return small / total, humanoid / total
}

// recommendedTactic names the super-effective tactic against the enemy mix, or
// "" when neither share is dominant enough to matter (M1 combat-depth hint).
// Mirrors the C3 bonus structure: energy shreds a swarm, defend blunts
// humanoid/shard heavy hitters. The threshold keeps the hint honest — a roughly
// even mix gives no advice rather than a misleading one.
func recommendedTactic(swarmShare, humanoidShare float64) string {
	const hintThreshold = 0.34
	if swarmShare >= humanoidShare && swarmShare >= hintThreshold {
		return canon.BattleTacticEnergy
	}
	if humanoidShare > swarmShare && humanoidShare >= hintThreshold {
		return canon.BattleTacticDefend
	}
	return ""
}

// countRoles tallies the non-fighter support roles in a squad (C4).
func countRoles(units []SquadUnit) (scouts, engineers, medics int) {
	for _, u := range units {
		switch u.Type {
		case "scout":
			scouts++
		case "engineer":
			engineers++
		case "medic":
			medics++
		}
	}
	return
}

// combatDamage computes one round's damage dealt and taken from the tactic,
// pooled ATK, enemy composition (C3 weaknesses) and squad composition (C4
// roles). Returns non-negative integers.
func combatDamage(tactic string, squadATK, enemyATK int, spirits []Spirit, units []SquadUnit) (dealt, taken int) {
	dealtMult, takenMult := 1.0, 1.0
	switch tactic {
	case canon.BattleTacticAttack:
		dealtMult = attackDealtMult
	case canon.BattleTacticDefend:
		takenMult = defendTakenMult
	case canon.BattleTacticEnergy:
		dealtMult, takenMult = energyDealtMult, energyTakenMult
	}

	// C3: super-effective tactics by enemy composition.
	smallShare, humanoidShare := enemyTypeShares(spirits)
	if tactic == canon.BattleTacticEnergy {
		dealtMult *= 1 + energyVsSwarmDealtBonus*smallShare
	}
	if tactic == canon.BattleTacticDefend {
		takenMult *= 1 - defendVsHumanoidTakenCut*humanoidShare
	}

	// C4: unit roles.
	scouts, engineers, medics := countRoles(units)
	dealtMult *= 1 + math.Min(scoutDealtCap, scoutDealtPerUnit*float64(scouts))
	takenMult *= 1 - math.Min(engineerMitigationCap, engineerMitigationPerUnit*float64(engineers))

	dealtF := float64(squadATK) * dealtMult
	takenF := float64(enemyATK)*takenMult - float64(medicHealPerUnit*medics)
	if takenF < 0 {
		takenF = 0
	}
	return int(math.Round(dealtF)), int(math.Round(takenF))
}

func battleStatusForClient(state string) string {
	switch state {
	case canon.BattleStateResolvedVictory:
		return "win"
	case canon.BattleStateResolvedDefeat:
		return "loss"
	case canon.BattleStateResolvedRetreat:
		return "retreat"
	default:
		return "ongoing"
	}
}

// riftLootRange holds the canonical loot bounds per rift type from
// docs/feature/balance_tables.md "Rift rewards". Energy/materials are rolled
// within [min,max]; XP is a fixed canon value.
//
// Deferred (no key/item system yet): the rare drop column — 5% key for type I,
// 20% key/rare-unit for type II. Tracked in ai/tasks.md Stage 2.1 alongside the
// streak rare-unit/cosmetic rewards; add here once a key/item store exists.
type riftLootRange struct {
	energyMin, energyMax       int
	materialsMin, materialsMax int
	xp                         int
}

var riftLootRanges = map[string]riftLootRange{
	"minor":    {100, 150, 8, 20, 100},
	"medium":   {220, 360, 25, 60, 250},
	"critical": {700, 1100, 100, 220, 600},
}

func buildRiftLoot(r *rift.Rift) map[string]any {
	lr, ok := riftLootRanges[r.Type]
	if !ok {
		lr = riftLootRanges["minor"]
	}
	return map[string]any{
		"energy":    rollRange(lr.energyMin, lr.energyMax),
		"materials": rollRange(lr.materialsMin, lr.materialsMax),
		"xp":        lr.xp,
		"tier":      r.Type,
	}
}

// rollRange returns a uniform int in [min,max]. Mirrors the pet-loot pattern
// (internal/pet/service.go) so reward variability is consistent across systems.
func rollRange(min, max int) int {
	if max <= min {
		return min
	}
	return min + rand.Intn(max-min+1)
}

func cumulativeUnitsLost(units []SquadUnit, remainingHP int) int {
	totalHP := 0
	for _, u := range units {
		totalHP += u.HP
	}
	damage := totalHP - remainingHP
	if damage <= 0 {
		return 0
	}

	lost := 0
	for _, u := range units {
		if damage < u.HP {
			break
		}
		damage -= u.HP
		lost++
	}
	return lost
}

func lostUnitIDs(units []SquadUnit, lostCount int) []string {
	if lostCount <= 0 {
		return nil
	}
	if lostCount > len(units) {
		lostCount = len(units)
	}
	unitIDs := make([]string, 0, lostCount)
	for i := 0; i < lostCount; i++ {
		unitIDs = append(unitIDs, units[i].ID)
	}
	return unitIDs
}

// ExecuteRound processes one combat round with the chosen tactic.
func (s *Service) ExecuteRound(ctx context.Context, battleID, tactic string) (*Battle, error) {
	b, err := s.battles.GetByID(ctx, battleID)
	if err != nil {
		return nil, httputil.NewNotFound("not_found", "battle not found")
	}
	if canon.IsBattleResolved(b.Status) {
		return nil, httputil.NewBadRequest("battle_ended", "battle is not ongoing")
	}

	// T-115: timeout check — if last update was more than 60s ago, auto retreat
	if time.Since(b.UpdatedAt) > RoundTimeout {
		b.Status = canon.BattleStateResolvedDefeat
		_ = s.squads.FinishMission(ctx, b.SquadID)
		if err := s.battles.Update(ctx, b); err != nil {
			return nil, fmt.Errorf("update battle timeout: %w", err)
		}
		return b, nil
	}

	// Validate tactic
	if _, ok := canon.BattleTacticsMVP[tactic]; !ok {
		return nil, httputil.NewBadRequest("invalid_tactic", "tactic must be 'attack', 'defend', 'energy', or 'retreat'")
	}

	// Retreat: end battle, no unit losses, refund half the engagement cost.
	if tactic == canon.BattleTacticRetreat {
		round := Round{
			Number: len(b.Rounds) + 1,
			Tactic: canon.BattleTacticRetreat,
		}
		b.Rounds = append(b.Rounds, round)
		b.Status = canon.BattleStateResolvedRetreat
		_ = s.squads.FinishMission(ctx, b.SquadID)
		if s.resources != nil {
			cost := s.encounterEnergyCost(ctx, b.TargetType, b.TargetID)
			refund := cost * canon.BattleRetreatRefundNumerator / canon.BattleRetreatRefundDenominator
			s.refundEnergy(ctx, b.AttackerID, refund)
		}
		if err := s.battles.Update(ctx, b); err != nil {
			return nil, fmt.Errorf("update battle retreat: %w", err)
		}
		return b, nil
	}

	// Get squad units
	units, err := s.squads.GetSquadUnits(ctx, b.SquadID)
	if err != nil {
		return nil, fmt.Errorf("get squad units: %w", err)
	}
	if len(units) == 0 {
		b.Status = canon.BattleStateResolvedDefeat
		_ = s.squads.FinishMission(ctx, b.SquadID)
		if err := s.battles.Update(ctx, b); err != nil {
			return nil, fmt.Errorf("update battle no units: %w", err)
		}
		return b, nil
	}

	spirits, _, loot, err := s.loadEncounter(ctx, b)
	if err != nil {
		return nil, err
	}
	enemyTotalHP, enemyTotalATK := computeEnemyStats(spirits)
	squadTotalHP, squadTotalATK := computeSquadStats(units)

	// Apply previous round damage to running HP
	for _, rd := range b.Rounds {
		enemyTotalHP -= rd.DmgDealt
		squadTotalHP -= rd.DmgTaken
	}
	if enemyTotalHP < 0 {
		enemyTotalHP = 0
	}
	if squadTotalHP < 0 {
		squadTotalHP = 0
	}
	prevSquadHP := squadTotalHP

	// Calculate this round's damage. Base tactic multipliers, then spirit
	// weaknesses (C3) and unit roles (C4) layer on top.
	dmgDealt, dmgTaken := combatDamage(tactic, squadTotalATK, enemyTotalATK, spirits, units)

	// Apply damage
	enemyTotalHP -= dmgDealt
	squadTotalHP -= dmgTaken
	if enemyTotalHP < 0 {
		enemyTotalHP = 0
	}
	if squadTotalHP < 0 {
		squadTotalHP = 0
	}
	totalLost := cumulativeUnitsLost(units, squadTotalHP)
	prevLost := cumulativeUnitsLost(units, prevSquadHP)

	round := Round{
		Number:    len(b.Rounds) + 1,
		Tactic:    tactic,
		DmgDealt:  dmgDealt,
		DmgTaken:  dmgTaken,
		UnitsLost: totalLost - prevLost,
		EnemyHP:   enemyTotalHP,
		SquadHP:   squadTotalHP,
	}

	// Check win/loss
	if enemyTotalHP <= 0 {
		b.Status = canon.BattleStateResolvedVictory
	} else if squadTotalHP <= 0 {
		b.Status = canon.BattleStateResolvedDefeat
	} else {
		b.Status = canon.BattleStateAwaitingTactic
	}

	b.Rounds = append(b.Rounds, round)
	if round.UnitsLost > 0 {
		if err := s.squads.MarkUnitsLost(ctx, lostUnitIDs(units, totalLost)); err != nil {
			return nil, fmt.Errorf("mark units lost: %w", err)
		}
	}
	if err := s.onBattleResolved(ctx, b, loot); err != nil {
		return nil, err
	}
	if err := s.battles.Update(ctx, b); err != nil {
		return nil, fmt.Errorf("update battle round: %w", err)
	}
	return b, nil
}

// onBattleResolved runs the end-of-battle side effects (finish mission, and on
// victory: close rift, grant loot/XP, tutorial/quest progress). No-op while the
// battle is still ongoing. Shared by ExecuteRound and Overcharge.
func (s *Service) onBattleResolved(ctx context.Context, b *Battle, loot map[string]any) error {
	if !canon.IsBattleResolved(b.Status) {
		return nil
	}
	if err := s.squads.FinishMission(ctx, b.SquadID); err != nil {
		return fmt.Errorf("finish squad mission: %w", err)
	}
	if b.Status != canon.BattleStateResolvedVictory {
		return nil
	}
	if b.TargetType == "rift" {
		if err := s.rifts.Close(ctx, b.TargetID); err != nil {
			return fmt.Errorf("close rift: %w", err)
		}
		// Anti-inflation: count this closure toward the player's daily cap for
		// the tier (only victories count). Best-effort — a limiter error never
		// fails the battle. tier comes from the encounter loot snapshot.
		if s.riftLimiter != nil {
			if tier, _ := loot["tier"].(string); tier != "" {
				_ = s.riftLimiter.RecordClosure(ctx, b.AttackerID, tier)
			}
		}
		// E2: closing a rift scores Human faction-war points by tier.
		if s.faction != nil {
			tier, _ := loot["tier"].(string)
			if pts := factionwar.PointsForRiftClose(tier); pts > 0 {
				_ = s.faction.AwardHuman(ctx, b.AttackerID, pts)
			}
		}
		// Spire: rare blueprint-fragment drop from rifts (megastructure_spire.md).
		// Pity-aware when the inventory supports it (audit C2): a long dry streak
		// ramps the chance to a guaranteed drop, so the megastructure gate can't
		// stall on bad luck. Falls back to a flat roll otherwise.
		if s.inventory != nil {
			tier, _ := loot["tier"].(string)
			if base := blueprintDropChance[tier]; base > 0 {
				if roller, ok := s.inventory.(BlueprintRoller); ok {
					if dropped, err := roller.RollBlueprintFragment(ctx, b.AttackerID, base, rand.Float64()); err != nil {
						slog.Error("blueprint pity roll failed", "battle_id", b.ID, "error", err)
					} else if dropped {
						loot["blueprint_fragment"] = 1
					}
				} else if rand.Float64() < base {
					if _, err := s.inventory.Grant(ctx, b.AttackerID, item.TypeBlueprintFragment, "", 1); err == nil {
						loot["blueprint_fragment"] = 1
					}
				}
			}
		}
	}
	if s.resources != nil {
		if _, err := s.resources.Add(ctx, b.AttackerID, loot["energy"].(int), loot["materials"].(int)); err != nil {
			return fmt.Errorf("grant battle resources: %w", err)
		}
	}
	battleXP, _ := loot["xp"].(int)
	if s.progression != nil {
		if _, err := s.progression.AddXP(ctx, b.AttackerID, battleXP); err != nil {
			return fmt.Errorf("grant battle xp: %w", err)
		}
	}
	if s.units != nil {
		unitXP := int(math.Round(float64(battleXP) * UnitBattleXPShare))
		ups, err := s.units.AwardSquadBattleXP(ctx, b.AttackerID, b.SquadID, unitXP)
		if err != nil {
			// Unit XP is a reward side-effect; don't fail the whole resolution.
			slog.Error("award squad battle xp failed", "battle_id", b.ID, "squad_id", b.SquadID, "error", err)
		} else {
			b.UnitLevelUps = ups
		}
	}
	// Onboarding Contact beat: closing a rift while on contact_step is "Contact
	// with Эзра" — the player turns into a temporary symbiont and enters the
	// symbiont tutorial. See docs/feature/symbiont_onboarding.md.
	if b.TargetType == "rift" {
		s.completeContactRift(ctx, b.AttackerID)
	}
	// D5: critical-rift (III) shard bosses drop an epic resonance signature.
	if s.inventory != nil && b.TargetType == "rift" {
		if tier, _ := loot["tier"].(string); tier == "critical" {
			spectrum := rollEpicSpectrum(b.ID)
			if _, err := s.inventory.Grant(ctx, b.AttackerID, item.TypeResonanceSignature, spectrum, 1); err != nil {
				slog.Error("grant epic signature failed", "battle_id", b.ID, "error", err)
			} else {
				epic := map[string]any{
					"item_type":   item.TypeResonanceSignature,
					"variant":     spectrum,
					"name":        "Резонансная сигнатура · " + item.SpectrumRu(spectrum),
					"spectrum_ru": item.SpectrumRu(spectrum),
				}
				loot["epic"] = epic
				// Snapshot rebuilds loot from loadEncounter, so stash on the
				// battle to merge it back into the client-facing payload.
				b.EpicLoot = epic
			}
		}
	}
	// E1 deepening: a won assault grinds the hive's intensity; on collapse it
	// closes + cleanses its region and drops an epic resonance signature (the
	// hive is a signature source like III bosses).
	if s.hives != nil && b.TargetType == "hive" {
		closed, err := s.hives.OnAssaultVictory(ctx, b.TargetID)
		if err != nil {
			slog.Error("hive assault resolution failed", "battle_id", b.ID, "hive_id", b.TargetID, "error", err)
		} else if closed {
			// E2: collapsing a hive is a III-class Human faction-war feat.
			if s.faction != nil {
				_ = s.faction.AwardHuman(ctx, b.AttackerID, factionwar.PointsCollapseHive)
			}
			// Collapsing a hive guarantees a blueprint fragment (Spire gate).
			if s.inventory != nil {
				_, _ = s.inventory.Grant(ctx, b.AttackerID, item.TypeBlueprintFragment, "", 1)
			}
			// E3: reaching and collapsing the infection root is "Contact with
			// Эзра" — it unlocks the faction-side choice. A player still on
			// contact_step must ALSO run the full onboarding bridge (temp
			// symbiont + step advance), otherwise contacted=true while the
			// onboarding FSM stays on contact_step forever.
			if s.contacter != nil {
				_ = s.contacter.MarkContacted(ctx, b.AttackerID)
				s.completeContactRift(ctx, b.AttackerID)
			}
			epic := map[string]any{"hive_closed": true}
			if s.inventory != nil {
				spectrum := rollEpicSpectrum(b.ID)
				if _, gErr := s.inventory.Grant(ctx, b.AttackerID, item.TypeResonanceSignature, spectrum, 1); gErr == nil {
					epic["item_type"] = item.TypeResonanceSignature
					epic["variant"] = spectrum
					epic["name"] = "Резонансная сигнатура · " + item.SpectrumRu(spectrum)
					epic["spectrum_ru"] = item.SpectrumRu(spectrum)
				}
			}
			loot["epic"] = epic
			b.EpicLoot = epic // Snapshot rebuilds loot, so stash it here too
		} else {
			weakened := map[string]any{"hive_weakened": true}
			loot["epic"] = weakened
			b.EpicLoot = weakened
		}
	}
	if b.TargetType == "tutorial" {
		if _, err := s.completeTutorialBattle(ctx, b.AttackerID); err != nil {
			return err
		}
	}
	if s.quests != nil && b.TargetType == "rift" {
		_ = s.quests.UpdateProgress(ctx, b.AttackerID, "win_3_battles")
	}
	return nil
}

// Snapshot builds a client-facing battle view with current HP and result status.
func (s *Service) Snapshot(ctx context.Context, b *Battle) (*Snapshot, error) {
	units, err := s.squads.GetSquadUnits(ctx, b.SquadID)
	if err != nil {
		return nil, fmt.Errorf("get squad units: %w", err)
	}
	spirits, encounterType, loot, err := s.loadEncounter(ctx, b)
	if err != nil {
		return nil, err
	}
	enemyHPMax, _ := computeEnemyStats(spirits)
	squadHPMax, _ := computeSquadStats(units)
	swarmShare, humanoidShare := enemyTypeShares(spirits)
	enemyHP := enemyHPMax
	squadHP := squadHPMax
	for _, rd := range b.Rounds {
		enemyHP -= rd.DmgDealt
		squadHP -= rd.DmgTaken
	}
	if enemyHP < 0 {
		enemyHP = 0
	}
	if squadHP < 0 {
		squadHP = 0
	}

	snapshot := &Snapshot{
		BattleID:    b.ID,
		Status:      battleStatusForClient(b.Status),
		BattleState: b.Status,
		Round:       len(b.Rounds),
		TargetType:  b.TargetType,
		TargetID:    b.TargetID,
		EnemyHP:     enemyHP,
		SquadHP:     squadHP,
		Enemy: BattleSide{
			HP:            enemyHP,
			HPMax:         enemyHPMax,
			RiftType:      encounterType,
			SpiritsCount:  len(spirits),
			SwarmShare:    swarmShare,
			HumanoidShare: humanoidShare,
			Weakness:      recommendedTactic(swarmShare, humanoidShare),
		},
		Squad: BattleSide{
			HP:        squadHP,
			HPMax:     squadHPMax,
			UnitCount: len(units),
		},
	}
	if len(b.Rounds) > 0 {
		lastRound := b.Rounds[len(b.Rounds)-1]
		snapshot.LastRound = &lastRound
	}
	if b.Status == canon.BattleStateResolvedVictory {
		if b.EpicLoot != nil {
			loot["epic"] = b.EpicLoot
		}
		snapshot.Loot = loot
		snapshot.UnitLevelUps = b.UnitLevelUps
	}
	if s.players != nil {
		p, err := s.players.GetByID(ctx, b.AttackerID)
		if err == nil && p != nil {
			snapshot.Profile = player.BuildProfilePayload(p)
			snapshot.Onboarding = player.BuildOnboardingPayload(p)
		}
	}
	return snapshot, nil
}

// Overcharge applies a x2.5 damage multiplier for the next 3 rounds,
// with a 25% chance to lose 1-3 units.
func (s *Service) Overcharge(ctx context.Context, battleID, playerID string) (*Battle, error) {
	if !canon.IsFeatureAvailable(canon.FeatureBattleOvercharge) {
		return nil, httputil.NewBadRequest("feature_locked", "overcharge is not available in the current release")
	}

	b, err := s.battles.GetByID(ctx, battleID)
	if err != nil {
		return nil, httputil.NewNotFound("not_found", "battle not found")
	}
	if b.AttackerID != playerID {
		return nil, httputil.NewForbidden("not_owner", "not your battle")
	}
	if canon.IsBattleResolved(b.Status) {
		return nil, httputil.NewBadRequest("battle_ended", "battle is not ongoing")
	}

	units, err := s.squads.GetSquadUnits(ctx, b.SquadID)
	if err != nil {
		return nil, fmt.Errorf("get squad units: %w", err)
	}
	if len(units) == 0 {
		b.Status = canon.BattleStateResolvedDefeat
		_ = s.squads.FinishMission(ctx, b.SquadID)
		_ = s.battles.Update(ctx, b)
		return b, nil
	}

	spirits, _, loot, err := s.loadEncounter(ctx, b)
	if err != nil {
		return nil, err
	}
	enemyTotalHP, enemyTotalATK := computeEnemyStats(spirits)
	squadTotalHP, squadTotalATK := computeSquadStats(units)
	for _, rd := range b.Rounds {
		enemyTotalHP -= rd.DmgDealt
		squadTotalHP -= rd.DmgTaken
	}
	if enemyTotalHP < 0 {
		enemyTotalHP = 0
	}
	if squadTotalHP < 0 {
		squadTotalHP = 0
	}
	prevSquadHP := squadTotalHP

	// Overcharge burst: x2.5 dealt, slightly elevated retaliation.
	dmgDealt := int(math.Round(float64(squadTotalATK) * canon.BattleOverchargeDealtMult))
	dmgTaken := int(math.Round(float64(enemyTotalATK) * canon.BattleOverchargeTakenMult))
	enemyTotalHP -= dmgDealt
	squadTotalHP -= dmgTaken
	if enemyTotalHP < 0 {
		enemyTotalHP = 0
	}
	if squadTotalHP < 0 {
		squadTotalHP = 0
	}

	totalLost := cumulativeUnitsLost(units, squadTotalHP)
	prevLost := cumulativeUnitsLost(units, prevSquadHP)
	// Overcharge risk: 25% chance the surge fries 1-3 of your own units.
	if rand.Float64() < canon.BattleOverchargeMisfireChance {
		totalLost += 1 + rand.Intn(3)
	}
	if totalLost > len(units) {
		totalLost = len(units)
	}
	if totalLost >= len(units) {
		squadTotalHP = 0 // squad wiped out
	}

	round := Round{
		Number:    len(b.Rounds) + 1,
		Tactic:    "overcharge",
		DmgDealt:  dmgDealt,
		DmgTaken:  dmgTaken,
		UnitsLost: totalLost - prevLost,
		EnemyHP:   enemyTotalHP,
		SquadHP:   squadTotalHP,
	}

	if enemyTotalHP <= 0 {
		b.Status = canon.BattleStateResolvedVictory
	} else if squadTotalHP <= 0 {
		b.Status = canon.BattleStateResolvedDefeat
	} else {
		b.Status = canon.BattleStateAwaitingTactic
	}

	b.Rounds = append(b.Rounds, round)
	if round.UnitsLost > 0 {
		if err := s.squads.MarkUnitsLost(ctx, lostUnitIDs(units, totalLost)); err != nil {
			return nil, fmt.Errorf("mark units lost: %w", err)
		}
	}
	if err := s.onBattleResolved(ctx, b, loot); err != nil {
		return nil, err
	}
	if err := s.battles.Update(ctx, b); err != nil {
		return nil, fmt.Errorf("update battle overcharge: %w", err)
	}
	return b, nil
}
