// Package symbiont implements the Symbiont geo-playstyle (#6): playing under
// hostile human domes against infrastructure. This slice covers covered-by-dome
// detection (T-760) and soft-drain (T-761) — standing under a foreign dome
// bleeds the Symbiont's energy, eased as they raise the cell's infection toward
// carving a pocket (lightweight T-762; the make-or-break balance is T-765).
package symbiont

import (
	"context"

	"github.com/ezra-game/server/internal/canon"
	"github.com/ezra-game/server/internal/factionwar"
	"github.com/ezra-game/server/internal/hearth"
	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/internal/roster"
	"github.com/ezra-game/server/internal/station"
	"github.com/ezra-game/server/internal/tower"
	"github.com/ezra-game/server/pkg/httputil"
)

// Soft-drain tuning (#6 T-761). Conservative MVP numbers — the real balance
// (1 Symbiont vs N dome holders) is the open make-or-break question (T-765).
const (
	DrainBaseEnergy  = 15 // energy bled per drain tick under a fresh hostile dome
	DrainMinEnergy   = 2  // floor so a near-carved pocket still nips a little
	DrainIntervalMin = 2  // at most one drain per this many minutes (throttle)
	PocketInfection  = 90 // at/above this cell infection, drain is fully relieved

	// Raise-infection action (T-764): the Symbiont's pocket-carving verb,
	// gated to under-a-foreign-dome. Costs energy, nudges the cell's infection
	// up, and feeds a little Resonance.
	RaiseEnergyCost    = 20
	RaiseInfectionStep = 15
	ResonancePerRaise  = 2

	// Pierce / pocket (T-762). Raising a domed cell to PierceThreshold punches a
	// hole the dome can't suppress for PierceTTLMin minutes — the pocket, where
	// the soft-drain lifts. Numbers provisional (T-765 balance); the make-or-break
	// "1 vs N holders" risk is moot here — dome suppression is a fixed per-cell
	// rate (not ×holders), so an active raiser always out-paces it.
	PierceThreshold = 85.0
	PierceTTLMin    = 30

	// Overload action (R4-2 "Перегрузить маяк"): the Symbiont corrodes the
	// nearest hostile beacon's HP — sabotaging enemy infrastructure (NOT players)
	// to thin the dome. Gated like RaiseInfection (Symbiont + under a foreign
	// dome). Range is generous so a player standing under a dome reliably has the
	// source beacon in reach.
	OverloadEnergyCost = 30
	OverloadDamage     = 40
	OverloadRangeM     = 150
	// ResonancePerOverloadFloor is the Resonance reward when the struck
	// beacon's level is unknown (Level=0 — e.g. a legacy/untiered row).
	// T-803: normally the reward scales with the beacon's actual income rate
	// via resonanceForOverload, so a fat L5 beacon pays out far more than a
	// fresh L1 one — Overload reads as "I took what it collected", not a
	// flat griefing tax (symbiont_geo_playstyle.md §7).
	ResonancePerOverloadFloor = 3

	// ResonancePerStationSabotage rewards a Rebellion-campaign strike against
	// a power plant (T-804, symbiont_geo_playstyle.md §8). Flat — plants
	// aren't level-tiered like beacons — picked mid-tier between the L1
	// floor and a maxed beacon (provisional; balance T-765-style pass later).
	ResonancePerStationSabotage = 5

	// Recon action (R4-2 "Разведать слабое место сети"): free intel scan for the
	// most vulnerable hostile beacon nearby (Symbiont-only, no energy/dome gate).
	// Generous range — scouting is how you find where to strike.
	ReconRangeM = 300

	// Resonance XP feeds the Resonance Level (R4-2). Sabotage of infrastructure
	// is the main RXP source; scouting (recon) is free intel and grants none.
	RXPPerOverload = 15
	RXPPerRaise    = 5
)

// FactionChecker reports whether a player has sided with the Symbionts.
type FactionChecker interface {
	IsSymbiont(ctx context.Context, playerID string) (bool, error)
}

// DomeCoverage reports dome coverage and raises a cell's infection (the
// pocket-carving verb).
type DomeCoverage interface {
	UnderForeignDome(ctx context.Context, lat, lng float64, playerID string) (covered bool, infection float64, pierced bool, err error)
	RaiseInfectionNearest(ctx context.Context, lat, lng, delta, pierceThreshold float64, ttlMin int) (infection float64, pierced bool, err error)
}

// EnergyDrainer applies the throttled soft-drain.
type EnergyDrainer interface {
	TryDrainSymbiontEnergy(ctx context.Context, id string, amount, intervalMin int) (drained, energy int, err error)
}

// EnergySpender charges the raise-infection energy cost.
type EnergySpender interface {
	Spend(ctx context.Context, playerID string, energy, materials int) (*player.Player, error)
}

// ResonanceGranter credits portable Symbiont Resonance.
type ResonanceGranter interface {
	AddSymbiontResonance(ctx context.Context, playerID string, delta int) (int, error)
}

// ResonanceXPGranter credits Resonance XP toward the Resonance Level (R4-2).
type ResonanceXPGranter interface {
	AddResonanceXP(ctx context.Context, playerID string, delta int) (xp, level int, leveledUp bool, err error)
}

// TowerCorroder damages the nearest hostile beacon (Overload) and scouts the
// weakest one (Recon).
type TowerCorroder interface {
	Corrode(ctx context.Context, lat, lng float64, attackerID string, damage, radiusM int) (*tower.CorrodeResult, error)
	WeakestHostile(ctx context.Context, lat, lng float64, attackerID string, radiusM int) (*tower.WeakestResult, error)
}

// PressureReader reports the world-wide count of currently-pierced cells
// (T-806, symbiont_geo_playstyle.md §10) — the faction's collective,
// visible goal, distinct from a player's personal Resonance Level.
type PressureReader interface {
	PiercedCellCount(ctx context.Context) (int, error)
}

// HearthBuffChecker reports whether a position is within range of an active
// Ephemeral Hearth (T-805, symbiont_geo_playstyle.md §9) — a coordinated-raid
// Overload damage bonus, shared by any Symbiont nearby regardless of who
// summoned it.
type HearthBuffChecker interface {
	Buffed(ctx context.Context, lat, lng float64) (bool, error)
}

// StationSaboteur forces a hostile power plant's lifecycle one step down
// (T-804, campaign "Rebellion" — symbiont_geo_playstyle.md §8): the second
// Overload target type, not tied to a foreign player's dome, so Resonance
// growth doesn't require human density nearby.
type StationSaboteur interface {
	Sabotage(ctx context.Context, lat, lng float64, attackerID string, radiusM int) (*station.SabotageResult, error)
	// NearestHostile scouts a plant without touching it — Recon's station leg.
	NearestHostile(ctx context.Context, lat, lng float64, attackerID string, radiusM int) (*station.NearestResult, error)
}

// EntityManager owns the Symbiont entity roster (R4-2 L2). Implemented by
// roster.Service; the symbiont service holds the faction gate and HTTP surface.
type EntityManager interface {
	EnsureStarterRoster(ctx context.Context, playerID string) error
	ListEntities(ctx context.Context, playerID string) ([]roster.EntityView, error)
	ControlUsage(ctx context.Context, playerID string) (used, capacity, level int, err error)
	AssignEntityAuto(ctx context.Context, playerID, entityID string, lat, lng float64) (*roster.EntityView, error)
	RecallEntity(ctx context.Context, playerID, entityID string) (*roster.EntityView, error)
	VerbBonuses(ctx context.Context, playerID string) (roster.VerbBonus, error)
}

// OnboardingBridge lets the symbiont verbs drive the symbiont-tutorial onboarding:
// it reads the player's step (to relax the gate for a tutorial cast) and advances
// the step when the taught action succeeds. Optional; nil-safe. Implemented by
// player.Service. See docs/feature/symbiont_onboarding.md.
type OnboardingBridge interface {
	OnboardingStep(ctx context.Context, playerID string) (string, error)
	AdvanceOnboardingAction(ctx context.Context, playerID, action string) error
}

// Service computes a Symbiont's under-dome status, applies soft-drain, and runs
// the gated pocket-carving verb.
type Service struct {
	faction    FactionChecker
	cells      DomeCoverage
	drainer    EnergyDrainer
	resources  EnergySpender
	resonance  ResonanceGranter
	corroder   TowerCorroder
	stations   StationSaboteur
	hearth     HearthBuffChecker
	pressure   PressureReader
	xp         ResonanceXPGranter
	roster     EntityManager
	onboarding OnboardingBridge
	war        WarScorer
	positions  PositionReader
}

func NewService(faction FactionChecker, cells DomeCoverage, drainer EnergyDrainer, resources EnergySpender, resonance ResonanceGranter) *Service {
	return &Service{faction: faction, cells: cells, drainer: drainer, resources: resources, resonance: resonance}
}

// WithCorroder wires the tower-corrosion effect for the Overload verb (optional
// dependency; matches towerSvc.WithStations / battleSvc.SetDiscoverer style).
func (s *Service) WithCorroder(c TowerCorroder) *Service { s.corroder = c; return s }

// WithStations wires the power-plant sabotage target (T-804, optional). When
// Overload finds no hostile beacon in range, it falls back to the nearest
// hostile plant.
func (s *Service) WithStations(st StationSaboteur) *Service { s.stations = st; return s }

// WithHearth wires the Ephemeral Hearth buff lookup (T-805, optional):
// Overload does extra damage when the caster is near an active hearth.
func (s *Service) WithHearth(h HearthBuffChecker) *Service { s.hearth = h; return s }

// WithPressure wires the world-pressure gauge (T-806, optional).
func (s *Service) WithPressure(p PressureReader) *Service { s.pressure = p; return s }

// PressureResult is the collective, faction-visible "how much dome is
// currently pierced" gauge (T-806) — not a personal stat.
type PressureResult struct {
	PiercedCells int `json:"pierced_cells"`
}

// Pressure reports the live world-wide pierced-cell count. Open to any
// caller (not Symbiont-gated) — humans benefit from seeing the pressure too.
func (s *Service) Pressure(ctx context.Context) (*PressureResult, error) {
	if s.pressure == nil {
		return &PressureResult{}, nil
	}
	n, err := s.pressure.PiercedCellCount(ctx)
	if err != nil {
		return nil, err
	}
	return &PressureResult{PiercedCells: n}, nil
}

// WithXP wires Resonance XP gain (drives the Resonance Level). Optional.
func (s *Service) WithXP(x ResonanceXPGranter) *Service { s.xp = x; return s }

// WithRoster wires the entity-roster manager (R4-2 L2). Optional.
func (s *Service) WithRoster(r EntityManager) *Service { s.roster = r; return s }

// WithOnboarding wires the symbiont-tutorial bridge (relaxes the verb gate during
// the tutorial and advances the step on a successful taught action). Optional.
func (s *Service) WithOnboarding(o OnboardingBridge) *Service { s.onboarding = o; return s }

// WarScorer credits Symbiont faction-war points (factionwar.Service).
type WarScorer interface {
	AwardSymbiont(ctx context.Context, playerID string, points int) error
}

// WithWar wires season scoring for Symbiont feats (canon faction_war.md:
// Overload +20). Optional.
func (s *Service) WithWar(w WarScorer) *Service { s.war = w; return s }

// PositionReader supplies the player's server-side position (player repo).
type PositionReader interface {
	GetByID(ctx context.Context, id string) (*player.Player, error)
}

// WithPositions makes verbs and drain trust the server-side position instead
// of client-supplied lat/lng: unlike tower placement (server position), the
// verbs took coordinates from the request body, so a spoofed body could
// Overload/Raise anywhere on the planet and dodge the drain. Optional.
func (s *Service) WithPositions(p PositionReader) *Service { s.positions = p; return s }

// serverPosition replaces client coords with the PATCH-validated server ones
// when available. Players who never reported a position (0,0) keep the client
// coords so nothing new breaks before the first fix.
func (s *Service) serverPosition(ctx context.Context, playerID string, lat, lng float64) (float64, float64) {
	if s.positions == nil {
		return lat, lng
	}
	p, err := s.positions.GetByID(ctx, playerID)
	if err != nil || p == nil || (p.Position.Lat == 0 && p.Position.Lng == 0) {
		return lat, lng
	}
	return p.Position.Lat, p.Position.Lng
}

// onTutorialStep reports whether the player is currently on the given onboarding
// step (nil-safe; false on any error or when no bridge is wired).
func (s *Service) onTutorialStep(ctx context.Context, playerID, step string) bool {
	if s.onboarding == nil {
		return false
	}
	cur, err := s.onboarding.OnboardingStep(ctx, playerID)
	return err == nil && cur == step
}

// RosterView is the Symbiont command-screen payload: the entity list, the
// control-slot budget for the player's Resonance Level, and the aggregate verb
// bonuses the active roster currently grants (L5).
type RosterView struct {
	Entities    []roster.EntityView `json:"entities"`
	ControlUsed int                 `json:"control_used"`
	ControlCap  int                 `json:"control_cap"`
	Level       int                 `json:"resonance_level"`
	Bonus       roster.VerbBonus    `json:"verb_bonus"`
}

// Roster returns the Symbiont's entity command screen (materializing the starter
// set on first read). Symbiont-only.
func (s *Service) Roster(ctx context.Context, playerID string) (*RosterView, error) {
	sym, err := s.faction.IsSymbiont(ctx, playerID)
	if err != nil {
		return nil, err
	}
	if !sym {
		return nil, httputil.NewForbidden("not_symbiont", "доступно только Симбионтам")
	}
	if s.roster == nil {
		return nil, httputil.NewBadRequest("unavailable", "ростер недоступен")
	}
	entities, err := s.roster.ListEntities(ctx, playerID)
	if err != nil {
		return nil, err
	}
	used, capacity, level, err := s.roster.ControlUsage(ctx, playerID)
	if err != nil {
		return nil, err
	}
	bonus, err := s.roster.VerbBonuses(ctx, playerID)
	if err != nil {
		return nil, err
	}
	return &RosterView{Entities: entities, ControlUsed: used, ControlCap: capacity, Level: level, Bonus: bonus}, nil
}

// AssignEntity dispatches one of the Symbiont's entities; it auto-acquires its
// target (weakest hostile beacon / nearest open rift) near (lat,lng).
// Symbiont-only.
func (s *Service) AssignEntity(ctx context.Context, playerID, entityID string, lat, lng float64) (*roster.EntityView, error) {
	if err := s.requireSymbiontRoster(ctx, playerID); err != nil {
		return nil, err
	}
	view, err := s.roster.AssignEntityAuto(ctx, playerID, entityID, lat, lng)
	if err != nil {
		return nil, err
	}
	// Advance the symbiont tutorial once the player has manifested an entity.
	if s.onboarding != nil && s.onTutorialStep(ctx, playerID, canon.OnboardingSymbiontEntity) {
		_ = s.onboarding.AdvanceOnboardingAction(ctx, playerID, "complete_symbiont_entity")
	}
	return view, nil
}

// RecallEntity disengages one of the Symbiont's entities. Symbiont-only.
func (s *Service) RecallEntity(ctx context.Context, playerID, entityID string) (*roster.EntityView, error) {
	if err := s.requireSymbiontRoster(ctx, playerID); err != nil {
		return nil, err
	}
	return s.roster.RecallEntity(ctx, playerID, entityID)
}

// requireSymbiontRoster gates entity actions: Symbiont side + roster wired.
func (s *Service) requireSymbiontRoster(ctx context.Context, playerID string) error {
	sym, err := s.faction.IsSymbiont(ctx, playerID)
	if err != nil {
		return err
	}
	if !sym {
		return httputil.NewForbidden("not_symbiont", "доступно только Симбионтам")
	}
	if s.roster == nil {
		return httputil.NewBadRequest("unavailable", "ростер недоступен")
	}
	return nil
}

// grantXP credits Resonance XP best-effort (never blocks the action).
func (s *Service) grantXP(ctx context.Context, playerID string, delta int) {
	if s.xp != nil && delta > 0 {
		_, _, _, _ = s.xp.AddResonanceXP(ctx, playerID, delta)
	}
}

// verbBonus returns the player's active-roster verb augmentation best-effort
// (R4-2 L5). Zero value when the roster is unwired or on error — never blocks.
func (s *Service) verbBonus(ctx context.Context, playerID string) roster.VerbBonus {
	if s.roster == nil {
		return roster.VerbBonus{}
	}
	b, err := s.roster.VerbBonuses(ctx, playerID)
	if err != nil {
		return roster.VerbBonus{}
	}
	return b
}

// Status is the Symbiont under-dome snapshot returned by GET /symbiont/status.
type Status struct {
	IsSymbiont       bool    `json:"is_symbiont"`
	UnderHostileDome bool    `json:"under_hostile_dome"`
	Pierced          bool    `json:"pierced"` // standing in a carved pocket → safe, no drain
	CellInfection    float64 `json:"cell_infection"`
	EnergyDrained    int     `json:"energy_drained"` // this poll (0 if throttled / relieved)
	Energy           int     `json:"energy,omitempty"`
}

// drainAmount scales the base drain down as the cell's infection rises toward a
// carved pocket: full drain on a fresh dome, zero once infection ≥ PocketInfection.
func drainAmount(infection float64) int {
	if infection >= PocketInfection {
		return 0
	}
	amt := int(float64(DrainBaseEnergy) * (1 - infection/float64(PocketInfection)))
	if amt < DrainMinEnergy {
		amt = DrainMinEnergy
	}
	return amt
}

// resonanceForOverload converts a struck beacon's level into the Resonance
// reward (T-803): the beacon's own canon income rate (energy + materials per
// hour), so a fat L5 beacon pays out far more than a fresh L1 one — Overload
// reads as "I took what it collected", not a flat griefing tax
// (symbiont_geo_playstyle.md §7). Falls back to ResonancePerOverloadFloor for
// an unknown/untiered level (e.g. a non-tower target).
func resonanceForOverload(level int) int {
	cfg, ok := canon.TowerLevelSpecs[level]
	if !ok {
		return ResonancePerOverloadFloor
	}
	r := cfg.EnergyPerHour + cfg.MaterialsPerHour
	if r < ResonancePerOverloadFloor {
		return ResonancePerOverloadFloor
	}
	return r
}

// Evaluate reports the player's under-dome status and applies soft-drain (once
// per throttle interval). Non-Symbionts short-circuit cheaply.
func (s *Service) Evaluate(ctx context.Context, playerID string, lat, lng float64) (*Status, error) {
	sym, err := s.faction.IsSymbiont(ctx, playerID)
	if err != nil {
		return nil, err
	}
	if !sym {
		return &Status{IsSymbiont: false}, nil
	}
	lat, lng = s.serverPosition(ctx, playerID, lat, lng)
	covered, infection, pierced, err := s.cells.UnderForeignDome(ctx, lat, lng, playerID)
	if err != nil {
		return nil, err
	}
	st := &Status{IsSymbiont: true, UnderHostileDome: covered, Pierced: pierced, CellInfection: infection}
	// In a carved pocket the dome is locally down — no drain (the reward for
	// piercing). Outside a dome: nothing to drain either.
	if !covered || pierced {
		return st, nil
	}
	if amt := drainAmount(infection); amt > 0 && s.drainer != nil {
		// L5: an assigned Поглотитель absorbs part of the incoming drain.
		if red := s.verbBonus(ctx, playerID).DrainReduction; red > 0 {
			if amt -= red; amt < 0 {
				amt = 0
			}
		}
		if amt > 0 {
			drained, energy, dErr := s.drainer.TryDrainSymbiontEnergy(ctx, playerID, amt, DrainIntervalMin)
			if dErr != nil {
				return nil, dErr
			}
			st.EnergyDrained = drained
			st.Energy = energy
		}
	}
	return st, nil
}

// RaiseResult is returned after the pocket-carving verb.
type RaiseResult struct {
	CellInfection float64 `json:"cell_infection"`
	Resonance     int     `json:"resonance"`
	Pierced       bool    `json:"pierced"` // pocket carved → dome locally down
}

// RaiseInfection is the Symbiont's pocket-carving verb (T-764): raise the
// current cell's infection to fight the dome's suppression and ease the drain.
// GATED action×covered_by_dome — allowed ONLY when standing under a foreign
// dome (the techno toolkit is for enemy infrastructure, not open ground).
func (s *Service) RaiseInfection(ctx context.Context, playerID string, lat, lng float64) (*RaiseResult, error) {
	sym, err := s.faction.IsSymbiont(ctx, playerID)
	if err != nil {
		return nil, err
	}
	if !sym {
		return nil, httputil.NewForbidden("not_symbiont", "доступно только Симбионтам")
	}
	lat, lng = s.serverPosition(ctx, playerID, lat, lng)
	// Symbiont tutorial: the very first cast is taught on open ground, so relax
	// the under-dome gate while the player is on the verb step.
	tutorial := s.onTutorialStep(ctx, playerID, canon.OnboardingSymbiontVerb)
	if !tutorial {
		covered, _, _, err := s.cells.UnderForeignDome(ctx, lat, lng, playerID)
		if err != nil {
			return nil, err
		}
		if !covered {
			return nil, httputil.NewBadRequest("not_under_dome", "поднимать заражение можно только под чужим куполом")
		}
	}
	if s.resources != nil {
		if _, err := s.resources.Spend(ctx, playerID, RaiseEnergyCost, 0); err != nil {
			return nil, err
		}
	}
	inf, pierced, err := s.cells.RaiseInfectionNearest(ctx, lat, lng, RaiseInfectionStep, PierceThreshold, PierceTTLMin)
	if err != nil {
		return nil, err
	}
	res := 0
	if s.resonance != nil {
		if r, rErr := s.resonance.AddSymbiontResonance(ctx, playerID, ResonancePerRaise); rErr == nil {
			res = r
		}
	}
	s.grantXP(ctx, playerID, RXPPerRaise)
	// Advance the symbiont tutorial once the player has cast the verb for real.
	if tutorial && s.onboarding != nil {
		_ = s.onboarding.AdvanceOnboardingAction(ctx, playerID, "complete_symbiont_verb")
	}
	return &RaiseResult{CellInfection: inf, Resonance: res, Pierced: pierced}, nil
}

// OverloadResult is returned after the Overload (corrode-beacon /
// sabotage-station) verb.
type OverloadResult struct {
	Found       bool `json:"found"`        // a hostile target was in range
	Destroyed   bool `json:"destroyed"`    // beacon: strike dropped it to 0 HP; station: strike ruined it
	RemainingHP int  `json:"remaining_hp"` // beacon HP after the hit (0 if destroyed); unset for a station
	Resonance   int  `json:"resonance"`    // portable Resonance after the gain
	// EnergySiphoned is this strike's Resonance delta (T-803): scales with
	// the struck beacon's level — client copy reads "energy taken: +N", not
	// "damage dealt", per symbiont_geo_playstyle.md §7 (Overload is theft,
	// not griefing).
	EnergySiphoned int `json:"energy_siphoned,omitempty"`
	// TargetKind distinguishes what was hit ("beacon" or "station", T-804) —
	// client copy/iconography differs (beacon: "маяк перегружен"; station:
	// "станция саботирована").
	TargetKind string `json:"target_kind,omitempty"`
	// HearthBuffed reports whether an active Ephemeral Hearth (T-805) boosted
	// this strike's damage — client copy can call out the coordinated-raid bonus.
	HearthBuffed bool `json:"hearth_buffed,omitempty"`
}

// Overload is the Symbiont's "Перегрузить маяк" verb (R4-2), extended by the
// Rebellion campaign (T-804) to a second target type. Primary target: the
// nearest hostile beacon's HP, GATED to standing under a foreign dome (the
// techno toolkit is for enemy infrastructure, never players). When no beacon
// is found — including when the player isn't under any foreign dome at all —
// it falls back to the nearest hostile power plant, forcing its lifecycle
// down a step. The station fallback needs no dome nearby: Resonance growth
// shouldn't be geo-locked to human density (symbiont_geo_playstyle.md §8,
// closes gameplay_review.md P2#18). Costs energy once per attempt; a hit
// feeds portable Resonance.
func (s *Service) Overload(ctx context.Context, playerID string, lat, lng float64) (*OverloadResult, error) {
	sym, err := s.faction.IsSymbiont(ctx, playerID)
	if err != nil {
		return nil, err
	}
	if !sym {
		return nil, httputil.NewForbidden("not_symbiont", "доступно только Симбионтам")
	}
	if s.corroder == nil && s.stations == nil {
		return nil, httputil.NewBadRequest("unavailable", "перегрузка недоступна")
	}
	lat, lng = s.serverPosition(ctx, playerID, lat, lng)
	covered, _, _, err := s.cells.UnderForeignDome(ctx, lat, lng, playerID)
	if err != nil {
		return nil, err
	}
	if !covered && s.stations == nil {
		// No dome-gated beacon target possible here, and no Rebellion-campaign
		// fallback wired — nothing this verb can do at this position.
		return nil, httputil.NewBadRequest("not_under_dome", "перегружать маяк можно только под чужим куполом")
	}
	if s.resources != nil {
		if _, err := s.resources.Spend(ctx, playerID, OverloadEnergyCost, 0); err != nil {
			return nil, err
		}
	}

	out := &OverloadResult{}
	if covered && s.corroder != nil {
		// L5: an assigned Сущность искажения lends extra corrosion to the strike.
		damage := OverloadDamage + s.verbBonus(ctx, playerID).OverloadDamage
		// T-805: a coordinated raid near an active Ephemeral Hearth hits harder.
		if s.hearth != nil {
			if buffed, hErr := s.hearth.Buffed(ctx, lat, lng); hErr == nil && buffed {
				damage += hearth.DamageBonus
				out.HearthBuffed = true
			}
		}
		cr, err := s.corroder.Corrode(ctx, lat, lng, playerID, damage, OverloadRangeM)
		if err != nil {
			return nil, err
		}
		if cr.Found {
			out.Found, out.Destroyed, out.RemainingHP, out.TargetKind = true, cr.Destroyed, cr.RemainingHP, "beacon"
			s.rewardOverload(ctx, playerID, resonanceForOverload(cr.Level), out)
		}
	}
	if !out.Found && s.stations != nil {
		sr, err := s.stations.Sabotage(ctx, lat, lng, playerID, OverloadRangeM)
		if err != nil {
			return nil, err
		}
		if sr.Found {
			out.Found, out.Destroyed, out.TargetKind = true, sr.Ruined, "station"
			s.rewardOverload(ctx, playerID, ResonancePerStationSabotage, out)
		}
	}
	return out, nil
}

// rewardOverload grants the Resonance/RXP/war-score payout shared by both
// Overload target types (beacon via Corrode, station via Sabotage — T-804).
func (s *Service) rewardOverload(ctx context.Context, playerID string, resonanceDelta int, out *OverloadResult) {
	if s.resonance != nil {
		if r, rErr := s.resonance.AddSymbiontResonance(ctx, playerID, resonanceDelta); rErr == nil {
			out.Resonance = r
			out.EnergySiphoned = resonanceDelta
		}
	}
	s.grantXP(ctx, playerID, RXPPerOverload)
	// E2: a landed Overload feat (either target type) scores Symbiont war
	// points (canon +20). Best-effort.
	if s.war != nil {
		_ = s.war.AwardSymbiont(ctx, playerID, factionwar.PointsOverloadBeacon)
	}
}

// ReconResult is returned after the Recon (scout-weak-node) verb.
type ReconResult struct {
	Found     bool `json:"found"`
	DistanceM int  `json:"distance_m"`
	HP        int  `json:"hp"` // beacon only; unset for a scouted station
	HPMax     int  `json:"hp_max"`
	// TargetKind distinguishes what was scouted ("beacon" or "station",
	// T-804) — mirrors OverloadResult.TargetKind.
	TargetKind string `json:"target_kind,omitempty"`
	// State is the scouted station's lifecycle state; unset for a beacon.
	State string `json:"state,omitempty"`
}

// Recon is the Symbiont's "Разведать слабое место сети" verb (R4-2): a free
// intel scan that surfaces the nearest most-vulnerable hostile beacon (lowest HP
// fraction), so the player knows where Overload bites hardest. Extended by
// T-804 to fall back to the nearest hostile power plant when no beacon is
// found — the Rebellion-campaign target Overload itself already accepts.
// Symbiont-only; no energy cost, no dome gate (scouting is how you find the
// weak point, wherever it is).
func (s *Service) Recon(ctx context.Context, playerID string, lat, lng float64) (*ReconResult, error) {
	sym, err := s.faction.IsSymbiont(ctx, playerID)
	if err != nil {
		return nil, err
	}
	if !sym {
		return nil, httputil.NewForbidden("not_symbiont", "доступно только Симбионтам")
	}
	if s.corroder == nil && s.stations == nil {
		return nil, httputil.NewBadRequest("unavailable", "разведка недоступна")
	}
	lat, lng = s.serverPosition(ctx, playerID, lat, lng)
	// L5: an assigned Следящая сущность widens the scan radius.
	rangeM := ReconRangeM + s.verbBonus(ctx, playerID).ReconRangeM

	if s.corroder != nil {
		w, err := s.corroder.WeakestHostile(ctx, lat, lng, playerID, rangeM)
		if err != nil {
			return nil, err
		}
		if w.Found {
			return &ReconResult{Found: true, DistanceM: w.DistanceM, HP: w.HP, HPMax: w.HPMax, TargetKind: "beacon"}, nil
		}
	}
	if s.stations != nil {
		st, err := s.stations.NearestHostile(ctx, lat, lng, playerID, rangeM)
		if err != nil {
			return nil, err
		}
		if st.Found {
			return &ReconResult{Found: true, DistanceM: st.DistanceM, TargetKind: "station", State: st.State}, nil
		}
	}
	return &ReconResult{Found: false}, nil
}
