package canon

import "time"

type ReleaseStage string

const (
	ReleaseStageMVP  ReleaseStage = "mvp"
	ReleaseStageV1_0 ReleaseStage = "v1.0"
	ReleaseStageV1_1 ReleaseStage = "v1.1"
)

// CurrentReleaseStage pins the backend to the currently supported release scope.
// Advanced to v1.0 (Phase D): unlocks beacon L4/L5 and battle overcharge.
// tower_pressure (v1.1) stays gated.
const CurrentReleaseStage = ReleaseStageV1_0

// Battle overcharge (D4): a risky burst action — big damage, chance to fry your
// own units. See battle_system.md.
const (
	BattleOverchargeDealtMult     = 2.5
	BattleOverchargeTakenMult     = 1.15
	BattleOverchargeMisfireChance = 0.25
)

type Feature string

const (
	FeatureBattleOvercharge Feature = "battle_overcharge"
	FeatureTowerLevel4      Feature = "tower_level_4"
	FeatureTowerLevel5      Feature = "tower_level_5"
	FeatureTowerPressure    Feature = "tower_pressure"
)

var featureMinimumStage = map[Feature]ReleaseStage{
	FeatureBattleOvercharge: ReleaseStageV1_0,
	FeatureTowerLevel4:      ReleaseStageV1_0,
	FeatureTowerLevel5:      ReleaseStageV1_0,
	FeatureTowerPressure:    ReleaseStageV1_1,
}

var stageRank = map[ReleaseStage]int{
	ReleaseStageMVP:  0,
	ReleaseStageV1_0: 1,
	ReleaseStageV1_1: 2,
}

func IsFeatureAvailable(feature Feature) bool {
	required, ok := featureMinimumStage[feature]
	if !ok {
		return true
	}
	return stageRank[CurrentReleaseStage] >= stageRank[required]
}

type AccessType string

const (
	AccessAnywhere                     AccessType = "Anywhere"
	AccessHomePreferred                AccessType = "Home preferred"
	AccessFieldRequired                AccessType = "Field required"
	AccessFieldRequiredForStartOnly    AccessType = "Field required for start only"
	AccessFieldRequiredForConfirmation AccessType = "Field required for confirmation / completion"
)

const (
	TowerTypeStandard  = "standard"
	TowerTypeAmplified = "amplified"
	TowerTypeRelay     = "relay"
	TowerTypeSymbiont  = "symbiont"
)

const (
	OnboardingNotStarted     = "not_started"
	OnboardingLoreStep       = "lore_step"
	OnboardingFirstTowerStep = "first_tower_step"
	OnboardingSurvivorStep   = "survivor_step"
	OnboardingTutorialBattle = "tutorial_battle_step"
	OnboardingPetUnlockStep  = "pet_unlock_step"
	// Symbiont onboarding bridge (см. docs/feature/symbiont_onboarding.md):
	// после стартового дрона игрок закрывает разлом → переживает Контакт →
	// временно становится симбионтом → проходит туториал симбионта → выбирает
	// фракцию. Только после выбора онбординг завершается.
	OnboardingContactStep    = "contact_step"         // закрыть разлом → Контакт
	OnboardingSymbiontIntro  = "symbiont_intro_step"  // лор: ты временно симбионт
	OnboardingSymbiontVerb   = "symbiont_verb_step"   // применить вербу (Поднять заражение)
	OnboardingSymbiontEntity = "symbiont_entity_step" // призвать сущность
	OnboardingFactionChoice  = "faction_choice_step"  // остаться/вернуться
	OnboardingCompleted      = "completed"
)

const (
	TowerStateActive        = "active"
	TowerStateUpgrading     = "upgrading"
	TowerStateOverloaded    = "overloaded"
	TowerStateDisabled      = "disabled"
	TowerStateLegacyStable  = "legacy_stable"
	TowerStateLegacyFading  = "legacy_fading"
	TowerStateLegacyOffline = "legacy_offline"
)

const (
	SquadStateIdle       = "idle"
	SquadStateAssigned   = "assigned"
	SquadStateTravelling = "travelling"
	SquadStateEngaged    = "engaged"
	SquadStateReturning  = "returning"
	SquadStateRecovering = "recovering"
	SquadStateDeparted   = "departed"
)

const (
	PetStateIdle            = "idle"
	PetStateMissionActive   = "mission_active"
	PetStateReturnReady     = "return_ready"
	PetStateSupportingTower = "supporting_tower"
	PetStateRetired         = "retired"
)

const (
	BattleStatePendingStart    = "pending_start"
	BattleStateRoundActive     = "round_active"
	BattleStateAwaitingTactic  = "awaiting_tactic"
	BattleStateResolvedVictory = "resolved_victory"
	BattleStateResolvedDefeat  = "resolved_defeat"
	BattleStateResolvedRetreat = "resolved_retreat"
)

func IsBattleResolved(state string) bool {
	switch state {
	case BattleStateResolvedVictory, BattleStateResolvedDefeat, BattleStateResolvedRetreat:
		return true
	default:
		return false
	}
}

const (
	BattleTacticAttack     = "attack"
	BattleTacticDefend     = "defend"
	BattleTacticEnergy     = "energy"
	BattleTacticRetreat    = "retreat"
	BattleTacticOvercharge = "overcharge"
)

var BattleTacticsMVP = map[string]struct{}{
	BattleTacticAttack:  {},
	BattleTacticDefend:  {},
	BattleTacticEnergy:  {},
	BattleTacticRetreat: {},
}

// Battle engagement energy cost (balance_tables.md "Базовые sinks → Бой"):
// paid up front to engage an encounter. Defeat forfeits it; retreat refunds
// half (BattleRetreatRefundNumerator/Denominator). Keyed by rift type.
var battleEnergyCostByRiftType = map[string]int{
	"minor":    10,
	"medium":   25,
	"critical": 100,
}

const (
	BattleRetreatRefundNumerator   = 1
	BattleRetreatRefundDenominator = 2
)

// BattleEnergyCost returns the ⚡ cost to engage a rift of the given type
// (unknown types fall back to the small-encounter cost).
func BattleEnergyCost(riftType string) int {
	if c, ok := battleEnergyCostByRiftType[riftType]; ok {
		return c
	}
	return battleEnergyCostByRiftType["minor"]
}

// riftDailyCapByType caps how many rifts of a tier a player may close per day
// (anti-farm / anti-inflation — docs/06_economy.md §6.3). Currently DISABLED
// (empty = all tiers uncapped): the cap added friction in development (and could
// soft-lock the onboarding Contact rift) with little upside pre-balance. The
// limiter infrastructure stays wired, so re-enabling is one line here, e.g.
// {"minor": 10}. 0 / missing = uncapped.
var riftDailyCapByType = map[string]int{}

// RiftDailyCap returns the per-player daily closure cap for a rift tier, or 0
// when the tier is uncapped. Closures are counted on victory, so abandoned or
// lost battles never burn the cap.
func RiftDailyCap(riftType string) int {
	return riftDailyCapByType[riftType]
}

// Rift difficulty scaling (E4): gently scale rift enemy stats to the player's
// level so mid-tier rifts neither wall an under-leveled player nor stay trivial
// for a veteran. Anchored to PLAYER LEVEL — monotonic progress that can't be
// gamed downward (unlike squad composition) — and clamped both ways so it can't
// be exploited into free farming. Reference level sits mid-MVP (player cap 15).
const (
	riftScaleReferenceLevel = 8
	riftScalePerLevel       = 0.03
	riftScaleFloor          = 0.85
	riftScaleCeil           = 1.30
)

// RiftDifficultyScale returns the multiplier applied to a rift's spirit HP/ATK
// for a player of the given level: <1 eases the fight for the under-leveled,
// >1 sharpens it for veterans, clamped to [floor, ceil].
func RiftDifficultyScale(playerLevel int) float64 {
	if playerLevel < 1 {
		playerLevel = 1
	}
	f := 1.0 + float64(playerLevel-riftScaleReferenceLevel)*riftScalePerLevel
	if f < riftScaleFloor {
		return riftScaleFloor
	}
	if f > riftScaleCeil {
		return riftScaleCeil
	}
	return f
}

const (
	EventBattleStarted         = "BattleStarted"
	EventBattleRoundResolved   = "BattleRoundResolved"
	EventRiftClosed            = "RiftClosed"
	EventTowerPlaced           = "TowerPlaced"
	EventTowerUpgradeStarted   = "TowerUpgradeStarted"
	EventTowerUpgraded         = "TowerUpgraded"
	EventTowerPressureResolved = "TowerPressureResolved"
	EventPetMissionStarted     = "PetMissionStarted"
	EventPetReturned           = "PetReturned"
	EventCityLinkUnstable      = "CityLinkUnstable"
	EventCityLinkRestoring     = "CityLinkRestoring"
	EventCityLinkRestored      = "CityLinkRestored"
	EventQuestProgressUpdated  = "QuestProgressUpdated"
	EventQuestCompleted        = "QuestCompleted"
)

type TowerLevelSpec struct {
	RadiusM          float64
	EffectPerHour    float64
	HPMax            int
	EnergyCost       int
	MaterialCost     int
	EnergyPerHour    int
	MaterialsPerHour int
	BuildTime        time.Duration
	Access           AccessType
}

var TowerLevelSpecs = map[int]TowerLevelSpec{
	// L1/L2 yield 1 material/hour (was 0) so every powered beacon gives a
	// guaranteed materials trickle — infrastructure-poor regions are no longer
	// starved of 🔩 (C5). See docs/feature/balance_tables.md.
	1: {RadiusM: 100, EffectPerHour: 3.0, HPMax: 100, EnergyCost: 50, MaterialCost: 10, EnergyPerHour: 2, MaterialsPerHour: 1, BuildTime: 0, Access: AccessFieldRequired},
	2: {RadiusM: 150, EffectPerHour: 5.5, HPMax: 150, EnergyCost: 150, MaterialCost: 30, EnergyPerHour: 4, MaterialsPerHour: 1, BuildTime: 5 * time.Minute, Access: AccessAnywhere},
	3: {RadiusM: 200, EffectPerHour: 8.0, HPMax: 220, EnergyCost: 400, MaterialCost: 80, EnergyPerHour: 7, MaterialsPerHour: 1, BuildTime: 20 * time.Minute, Access: AccessFieldRequired},
	4: {RadiusM: 300, EffectPerHour: 12.0, HPMax: 320, EnergyCost: 1200, MaterialCost: 240, EnergyPerHour: 11, MaterialsPerHour: 2, BuildTime: time.Hour, Access: AccessFieldRequired},
	5: {RadiusM: 400, EffectPerHour: 16.0, HPMax: 450, EnergyCost: 3200, MaterialCost: 640, EnergyPerHour: 16, MaterialsPerHour: 3, BuildTime: 4 * time.Hour, Access: AccessFieldRequired},
}

func IsTowerLevelAvailable(level int) bool {
	switch level {
	case 4:
		return IsFeatureAvailable(FeatureTowerLevel4)
	case 5:
		return IsFeatureAvailable(FeatureTowerLevel5)
	default:
		return true
	}
}

const (
	// 40m (was 30): beacons/Core are placed on the cell NEAREST the player's
	// GPS (no center reticle), and the nearest 50m-grid cell centre can be up to
	// ~35m away, so 30m would reject corner positions. 40m covers the geometry
	// with a small GPS-sync margin while staying local (anti-cheat).
	TowerPlacementMaxDistanceM = 40.0
	TowerOverloadRadiusM       = 100.0
	TowerOverloadMaxCount      = 5
	TowerL3NearbyRadiusM       = 200.0
	PlayerMaxLevelMVP          = 15
)

// SquadMissionDuration is how long a squad stays "on_mission" (the displayed
// returns_at ETA). Battles resolve via FinishMission well before this; it is a
// single source of truth shared by squad.Send and the battle squad provider so
// the ETA shown to players is consistent across both entry points.
const SquadMissionDuration = 10 * time.Minute

// Timed away-mission rewards, granted when a squad's ReturnsAt passes (the
// mission worker completes it). Patrol secures the target area (caps infection
// in a radius) plus a small resource haul; capture is a bigger resource haul.
// Tunable; documented in docs/feature/survivors_and_squads.md.
const (
	SquadPatrolEnergy         = 30
	SquadPatrolMaterials      = 6
	SquadPatrolCleanseRadiusM = 120.0
	SquadPatrolCleanseTarget  = 25.0 // infection capped to this in the radius
	SquadCaptureEnergy        = 70
	SquadCaptureMaterials     = 18
	// SquadMissionRangeM is how far from the player's server-side position a
	// mission target may be — missions are field sorties, not remote commands.
	SquadMissionRangeM = 500.0
	// SquadMissionCostEnergy is the refundless dispatch cost of a timed
	// mission (keeps patrol net-positive but not a free faucet).
	SquadMissionCostEnergy = 10
)

var ResourceGlossary = map[string]string{
	"energy":    "Стабилизированная сетевая энергия человеческой инфраструктуры.",
	"materials": "Техкомплекты, сплавы и монтажные наборы для полевой инфраструктуры.",
	"hack_keys": "Резонансные модули и протоколы доступа для перенастройки защитных контуров.",
	"crystals":  "Редкие энергокристаллы и высокоценные носители энергии/доступа.",
}

var SurvivorGlossary = map[string]string{
	"fighters":  "Боевой костяк human army для PvE и защиты сети.",
	"engineers": "Специалисты по инфраструктуре и поддержке human network.",
}

var PetGlossary = map[string]string{
	"human_pet":      "Роботизированный разведчик или инженерный дрон human side.",
	"symbiont_pet":   "Малая связанная энергетическая сущность Ezra side.",
	"legacy_network": "Бывшая человеческая сеть игрока после перехода в симбионта.",
}
