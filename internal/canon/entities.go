package canon

// Symbiont entity archetypes and their autonomous-agent tuning (R4-2 roster L2,
// symbiont_roster.md §"Архетипы сущностей" + §"Progression model"). A Symbiont
// does not command a human army; instead the Resonance Pool materializes a small
// roster of bound entities, each assigned to a hostile beacon or a rift, acting
// autonomously on a server timer (manifesting → assigned → recovering).

// Archetype identifiers.
const (
	EntityAssault    = "assault"    // Штурмовая сущность — основной атакующий юнит
	EntityDistortion = "distortion" // Сущность искажения — sabotage/ослабление маяков
	EntityScout      = "scout"      // Следящая сущность — разведка/усиление узлов
	EntityAbsorber   = "absorber"   // Поглотитель — sustain/hold разлома
)

// Target kinds an archetype can be assigned to.
const (
	EntityTargetTower = "tower" // corrode a hostile beacon's HP
	EntityTargetRift  = "rift"  // amplify a rift's intensity
)

// Entity status machine (symbiont_roster.md §"Symbiont command screen").
const (
	EntityAvailable   = "available"   // idle, can be assigned
	EntityManifesting = "manifesting" // assigned, materializing toward first tick
	EntityAssigned    = "assigned"    // active, applying its effect each tick
	EntityRecovering  = "recovering"  // disengaged, on cooldown before available
)

// EntitySpec is the per-archetype tuning the roster and the autonomous tick read.
type EntitySpec struct {
	Archetype   string
	TargetKind  string // EntityTargetTower | EntityTargetRift
	CapWeight   int    // control-slot cost (Поглотитель=2, прочие=1)
	UnlockLevel int    // minimum Resonance Level to control this archetype
	TowerDamage int    // HP corroded per tick (tower-kind only)
	RiftAmplify int    // intensity added per tick (rift-kind only)
	ManifestMin int    // minutes manifesting before the first tick
	RecoveryMin int    // minutes recovering after disengage
}

// entitySpecs holds the canon tuning. RL unlocks follow symbiont_roster.md
// §"Resonance Level": RL1 assault+scout, RL2 distortion, RL3 absorber.
// Per-tick magnitudes are conservative MVP numbers (tick runs every
// EntityTickIntervalMin); the make-or-break balance is a playtest question.
var entitySpecs = map[string]EntitySpec{
	EntityAssault:    {EntityAssault, EntityTargetTower, 1, 1, 20, 0, 10, 30},
	EntityDistortion: {EntityDistortion, EntityTargetTower, 1, 2, 12, 0, 10, 30},
	EntityScout:      {EntityScout, EntityTargetRift, 1, 1, 0, 2, 5, 20},
	EntityAbsorber:   {EntityAbsorber, EntityTargetRift, 2, 3, 0, 3, 15, 45},
}

// EntityTickIntervalMin is how often the autonomous-pressure worker runs. The
// per-tick magnitudes above are calibrated against this cadence; control cap
// (RL 2–6 slots) bounds how much offline pressure a player can project.
const EntityTickIntervalMin = 10

// EntitySpecFor returns the tuning for an archetype (ok=false if unknown).
func EntitySpecFor(archetype string) (EntitySpec, bool) {
	s, ok := entitySpecs[archetype]
	return s, ok
}

// IsEntityArchetype reports whether the string is a known archetype.
func IsEntityArchetype(archetype string) bool {
	_, ok := entitySpecs[archetype]
	return ok
}

// EntityCapWeight returns an archetype's control-slot cost (0 if unknown).
func EntityCapWeight(archetype string) int {
	if s, ok := entitySpecs[archetype]; ok {
		return s.CapWeight
	}
	return 0
}

// EntityUnlockLevel returns the Resonance Level an archetype needs (large if
// unknown, so unknown archetypes never unlock).
func EntityUnlockLevel(archetype string) int {
	if s, ok := entitySpecs[archetype]; ok {
		return s.UnlockLevel
	}
	return ResonanceMaxLevel + 1
}

// EntityTargetKind returns whether an archetype acts on towers or rifts.
func EntityTargetKind(archetype string) string {
	if s, ok := entitySpecs[archetype]; ok {
		return s.TargetKind
	}
	return ""
}

// StarterRoster maps a Resonance Pool value to the starter set of archetypes it
// materializes (symbiont_roster.md §"Starter roster conversion"). The guaranteed
// rule: even minimal progress yields at least 1 assault entity.
func StarterRoster(pool int) []string {
	roster := make([]string, 0, 5)
	if pool >= 6 {
		roster = append(roster, EntityAssault)
	}
	if pool >= 10 {
		roster = append(roster, EntityScout)
	}
	if pool >= 14 {
		roster = append(roster, EntityDistortion)
	}
	if pool >= 18 {
		roster = append(roster, EntityAbsorber)
	}
	if pool >= 22 {
		roster = append(roster, EntityAssault) // дополнительная Штурмовая
	}
	if len(roster) == 0 {
		roster = append(roster, EntityAssault) // guaranteed starter
	}
	return roster
}

// EntitySpecRu is the Russian specialization line for an archetype (command-
// screen card, L4).
func EntitySpecRu(archetype string) string {
	switch archetype {
	case EntityAssault:
		return "Коррозия маяка"
	case EntityDistortion:
		return "Ослабление маяка"
	case EntityScout:
		return "Усиление разлома · разведка"
	case EntityAbsorber:
		return "Удержание разлома"
	default:
		return "—"
	}
}

// EntityNameRu is the Russian display name for an archetype.
func EntityNameRu(archetype string) string {
	switch archetype {
	case EntityAssault:
		return "Штурмовая сущность"
	case EntityDistortion:
		return "Сущность искажения"
	case EntityScout:
		return "Следящая сущность"
	case EntityAbsorber:
		return "Поглотитель"
	default:
		return archetype
	}
}

// RiftAmplifyCap bounds how high an entity can drive a rift's intensity, so a
// parked absorber can't run a rift to infinity (symbiont_roster.md milestone
// logic, not infinite farm).
const RiftAmplifyCap = 100

// Entity → verb bonuses (R4-2 roster L5): an ASSIGNED entity passively augments
// the player's active verbs, closing the loop between the autonomous roster and
// the verb playstyle. Per-entity base contribution, scaled by Attunement rank.
//   distortion → +Overload damage ("помощь в Overload")
//   scout      → +Recon range ("увеличенный радиус поиска слабых мест")
//   absorber   → −soft-drain ("поглощение части входящего давления")
const (
	EntityOverloadBonusBase   = 8  // +Overload damage per assigned distortion
	EntityReconRangeBonusBase = 60 // +Recon range (m) per assigned scout
	EntityDrainReductionBase  = 3  // −soft-drain (energy) per assigned absorber
)

// EntityTargetRangeM is how far from the player an entity auto-acquires its
// target at assign time (weakest hostile beacon / nearest open rift). Generous
// so dispatching is reliable; the entity then acts at range.
const EntityTargetRangeM = 400

// ── Attunement (R4-2 roster L3, symbiont_roster.md §"Attunement") ────────────
// Per-entity quality, ranks I..III. Grows through USE (a successful autonomous
// tick on a live target), not passively over time. Each rank lifts the entity's
// profile effect; rank III also grants a small edge (faster recovery).

// AttunementMaxRank is the Attunement cap (rank III).
const AttunementMaxRank = 3

// attunementUsesThreshold[r] is the CUMULATIVE successful uses to reach rank r.
// Index 0/1 are 0 (every entity starts at rank I). II at 10 uses, III at 30 —
// roughly 1.7h / 5h of active assignment at EntityTickIntervalMin.
var attunementUsesThreshold = [AttunementMaxRank + 1]int{0, 0, 10, 30}

// attunementEffectMul[r] scales the entity's per-tick effect by rank: I ×1.00,
// II +12% (canon "+10-15%"), III +28% (canon "+25-30%").
var attunementEffectMul = [AttunementMaxRank + 1]float64{0, 1.0, 1.12, 1.28}

// AttunementRankForUses returns the Attunement rank (1..AttunementMaxRank) for a
// given cumulative use count.
func AttunementRankForUses(uses int) int {
	rank := 1
	for r := 2; r <= AttunementMaxRank; r++ {
		if uses >= attunementUsesThreshold[r] {
			rank = r
		}
	}
	return rank
}

// AttunementUsesForNextRank returns the cumulative uses needed for the next
// rank, or -1 if already at max (for client "X/Y to rank N" readouts).
func AttunementUsesForNextRank(rank int) int {
	if rank >= AttunementMaxRank {
		return -1
	}
	if rank < 1 {
		rank = 1
	}
	return attunementUsesThreshold[rank+1]
}

// AttunementEffectMultiplier returns the per-tick effect multiplier for a rank.
func AttunementEffectMultiplier(rank int) float64 {
	if rank < 1 {
		rank = 1
	}
	if rank > AttunementMaxRank {
		rank = AttunementMaxRank
	}
	return attunementEffectMul[rank]
}

// AttunementRecoveryFactor returns the recovery-time scale for a rank — the
// rank III "small unique perk": disengaged entities bounce back faster
// (×0.67, i.e. −33% recovery). Ranks I/II are unaffected (×1.0). NB: the
// canon's archetype-flavoured perks (assault first-impulse, distortion debuff
// duration, absorber incoming-pressure relief) depend on combat systems not yet
// built and are folded into L5; this uniform recovery edge stands in for now.
func AttunementRecoveryFactor(rank int) float64 {
	if rank >= AttunementMaxRank {
		return 0.67
	}
	return 1.0
}
