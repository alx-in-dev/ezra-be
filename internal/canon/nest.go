package canon

import "time"

// Symbiont home Nest constants (N3, spirit_world_and_symbiont_nest.md §5,
// architecture.md ADR-N3). All numbers are MVP placeholders to be tuned via
// balance_tables.md + playtest — especially the Executable-Loss corridor
// (accrued-resonance cap + rebuild cost), which is load-bearing (ADR-N3-7).

// NestMaxLevel is the nest level cap (L1–L5, mirror of the human Core/beacons).
const NestMaxLevel = 5

// NestLevelConfig is the per-level nest behaviour (mirror of hive.LevelConfig /
// the beacon L1–L5 curve): higher level reaches farther, trickles more, holds
// more siege HP, and defends longer.
type NestLevelConfig struct {
	// AuraRadiusM is the infection/territory aura radius (ST_DWithin, not a
	// polygon — ADR-N3-2). Grows with level: the visible "inverse dome".
	AuraRadiusM float64
	// TricklePerTick is the Resonance added to the nest's accrued buffer per
	// nest:tick at full support (scaled down by decayed support_level).
	TricklePerTick float64
	// SiegeHPMax is the siege HP a raid must drain (across assaults) before the
	// nest enters `collapsing`.
	SiegeHPMax float64
	// DefenseWindow is the BASE wall-clock time from the first assault telegraph
	// to the earliest collapse (multiplied by DefenseModifiers — ADR-N3-5).
	DefenseWindow time.Duration
}

var nestLevelConfigs = map[int]NestLevelConfig{
	1: {AuraRadiusM: 200, TricklePerTick: 2.0, SiegeHPMax: 100, DefenseWindow: 2 * time.Hour},
	2: {AuraRadiusM: 275, TricklePerTick: 3.0, SiegeHPMax: 140, DefenseWindow: 3 * time.Hour},
	3: {AuraRadiusM: 350, TricklePerTick: 4.5, SiegeHPMax: 190, DefenseWindow: 4 * time.Hour},
	4: {AuraRadiusM: 425, TricklePerTick: 6.0, SiegeHPMax: 250, DefenseWindow: 5 * time.Hour},
	5: {AuraRadiusM: 500, TricklePerTick: 8.0, SiegeHPMax: 320, DefenseWindow: 6 * time.Hour},
}

// NestConfig returns the level config (clamped to L1 for unknown levels).
func NestConfig(level int) NestLevelConfig {
	if cfg, ok := nestLevelConfigs[level]; ok {
		return cfg
	}
	return nestLevelConfigs[1]
}

// NestAuraRadiusM is the aura radius for a nest level — the single source of
// truth the three source-set queries (infection CTE, factionwar, budget) read.
// factionwar keeps its own local literals (import-edge) that must stay in sync.
func NestAuraRadiusM(level int) float64 { return NestConfig(level).AuraRadiusM }

const (
	// NestRelocationCostCrystals: the first relocation is FREE (relocated_at is
	// nil), each later one costs this — mirror of CoreRelocationCostCrystals.
	NestRelocationCostCrystals = 50

	// NestTrickleCap bounds the accrued-resonance buffer so an unattended nest
	// cannot bank unbounded Resonance offline, and so what is lost on collapse
	// stays inside the Executable-Loss corridor (ADR-N3-7).
	NestTrickleCap = 500.0

	// NestSupportMax / NestSupportFloor: feed restores support to max; decay
	// drives it down toward the floor (never below — neglect degrades OUTPUT,
	// not existence; US-N3-5). Newbie protection is applied on top in code.
	NestSupportMax   = 100.0
	NestSupportFloor = 20.0

	// NestSupportDecayPerTick: support lost per nest:tick when unfed. At ~5m/tick
	// this is a soft daily drift (a nest survives an absence — decay degrades the
	// aura/trickle output via the support factor, never existence). Placeholder.
	NestSupportDecayPerTick = 1.0

	// NestMinReactionFloor: the guaranteed wall-clock window between the first
	// under-attack telegraph and the earliest possible collapse, even under
	// back-to-back assaults (ADR-N3-4 invariant 1; venue-safety for an offline
	// owner).
	NestMinReactionFloor = 30 * time.Minute

	// NestAssaultDamage is the siege HP one won human assault drains (ADR-N3-4).
	// SiegeHPMax/this ≈ the number of assaults to push a nest into `collapsing`
	// (L1 100/34 ≈ 3, L5 320/34 ≈ 10) — placeholder, tune via balance_tables.
	NestAssaultDamage = 34.0

	// NestAssaultEnergyCost is the ⚡ price a human squad pays per nest assault
	// (mirror of HiveAssaultEnergyCost=150).
	NestAssaultEnergyCost = 150

	// NestAssaultMinFighters: assaulting a home nest demands a full squad, like a
	// hive.
	NestAssaultMinFighters = 5
)

// NestDefenseWindow returns the BASE siege window for a nest level (before the
// DefenseModifier multipliers). A nest never collapses faster than the level-1
// base regardless of level lookup failure.
func NestDefenseWindow(level int) time.Duration { return NestConfig(level).DefenseWindow }

const (
	// NestDefenderWindowPerPoint: each Defender skill point lengthens the siege
	// window by this fraction (T-844 — the dead skill tree finally does something
	// on the Symbiont side). Placeholder.
	NestDefenderWindowPerPoint = 0.1

	// NestEnergistTricklePerPoint: each Energist skill point raises the nest's
	// trickle by this fraction (T-845). Placeholder.
	NestEnergistTricklePerPoint = 0.05

	// NestGarrisonWindowPerSpirit: each tamed spirit kept HOME (idle/available,
	// not sent out to attack) lengthens the nest siege window by this fraction
	// (T-873). This is the N4 spirit-garrison — the payoff of taming and the
	// strategic tension of "hold the swarm home vs. send it to raid". Deliberately
	// below NestDefenderWindowPerPoint per spirit so it stacks meaningfully but a
	// garrison is a soft buffer, not an unbreakable wall (venue-safety / П2).
	NestGarrisonWindowPerSpirit = 0.06

	// NestGarrisonCap: max tamed spirits that count toward the garrison bonus.
	// Caps the modifier so a large roster can't make a nest effectively immortal
	// (Executable-Loss corridor). Above this, extra spirits are better off raiding.
	NestGarrisonCap = 5

	// NestPocketHoldSeconds: how far ahead nest:tick pushes pierced_until for a
	// nest's pocket cells (T-843, ADR-N3-8). ~2× the 5-min tick so a live nest
	// always holds its pocket open; when it dies the refresh stops and the cell
	// expires on its own (self-healing — no infinity sentinel to unwind).
	NestPocketHoldSeconds = 900
)
