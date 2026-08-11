package roster

import (
	"context"
	"log/slog"
	"time"

	"github.com/ezra-game/server/internal/canon"
	"github.com/ezra-game/server/internal/rift"
	"github.com/ezra-game/server/internal/tower"
	"github.com/ezra-game/server/pkg/httputil"
)

// TowerCorroder applies an entity's autonomous corrosion to a specific hostile
// beacon (by id) and acquires the weakest hostile beacon as an assign target.
// Implemented by tower.Service.
type TowerCorroder interface {
	CorrodeTower(ctx context.Context, towerID, attackerID string, damage int) (*tower.CorrodeResult, error)
	WeakestHostile(ctx context.Context, lat, lng float64, attackerID string, radiusM int) (*tower.WeakestResult, error)
}

// RiftAmplifier raises a rift's intensity (an entity's autonomous "усиление
// разлома") and acquires the nearest open rift as an assign target. Implemented
// by rift.Service.
type RiftAmplifier interface {
	AmplifyRift(ctx context.Context, riftID string, delta int) (open bool, intensity int, err error)
	NearestOpenRift(ctx context.Context, lat, lng, radiusM float64) (*rift.Rift, error)
}

// WithEffectors wires the autonomous-tick effectors (L2b). Optional; the tick is
// a no-op for a target kind whose effector is nil.
func (s *Service) WithEffectors(corroder TowerCorroder, amplifier RiftAmplifier) *Service {
	s.corroder = corroder
	s.amplifier = amplifier
	return s
}

// EntityView is the client-facing entity DTO (Symbiont command screen).
type EntityView struct {
	ID             string `json:"id"`
	Archetype      string `json:"archetype"`
	Name           string `json:"name"`
	Status         string `json:"status"`
	TargetKind     string `json:"target_kind,omitempty"`
	TargetID       string `json:"target_id,omitempty"`
	Attunement     int    `json:"attunement"`
	AttunementUses int    `json:"attunement_uses"`           // cumulative successful uses
	AttunementNext int    `json:"attunement_next"`           // uses for next rank, -1 if max
	CapWeight      int    `json:"cap_weight"`
	UnlockLevel    int    `json:"unlock_level"`
	Locked         bool   `json:"locked"`                    // archetype above current RL → cannot be assigned yet
	ETASeconds     int    `json:"eta_seconds,omitempty"`     // until manifesting/recovering resolves
	// L4 command-screen card detail.
	PressurePower  int    `json:"pressure_power"` // per-tick effect magnitude at current rank
	Spec           string `json:"spec"`           // specialization line (RU)
	RangeM         int    `json:"range_m"`        // action/acquire range
	RecoveryMin    int    `json:"recovery_min"`   // recovery window at current rank (minutes)
}

func (s *Service) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// toView renders an Entity to its DTO at the given Resonance Level / clock.
func (s *Service) toView(e *Entity, level int, now time.Time) EntityView {
	v := EntityView{
		ID:             e.ID,
		Archetype:      e.Archetype,
		Name:           canon.EntityNameRu(e.Archetype),
		Status:         e.Status,
		Attunement:     e.Attunement,
		AttunementUses: e.AttunementUses,
		AttunementNext: canon.AttunementUsesForNextRank(e.Attunement),
		CapWeight:      canon.EntityCapWeight(e.Archetype),
		UnlockLevel:    canon.EntityUnlockLevel(e.Archetype),
	}
	v.Locked = level < v.UnlockLevel
	// L4 card detail: pressure power (scaled by rank), spec, range, recovery.
	if spec, ok := canon.EntitySpecFor(e.Archetype); ok {
		mul := canon.AttunementEffectMultiplier(e.Attunement)
		base := spec.TowerDamage
		if spec.TargetKind == canon.EntityTargetRift {
			base = spec.RiftAmplify
		}
		v.PressurePower = scaleEffect(base, mul)
		v.RangeM = canon.EntityTargetRangeM
		v.RecoveryMin = int(float64(spec.RecoveryMin)*canon.AttunementRecoveryFactor(e.Attunement) + 0.5)
	}
	v.Spec = canon.EntitySpecRu(e.Archetype)
	if e.TargetKind != nil {
		v.TargetKind = *e.TargetKind
	}
	if e.TargetID != nil {
		v.TargetID = *e.TargetID
	}
	switch e.Status {
	case canon.EntityManifesting:
		if e.ManifestAt != nil {
			if eta := int(e.ManifestAt.Sub(now).Seconds()); eta > 0 {
				v.ETASeconds = eta
			}
		}
	case canon.EntityRecovering:
		if e.RecoveryAt != nil {
			if eta := int(e.RecoveryAt.Sub(now).Seconds()); eta > 0 {
				v.ETASeconds = eta
			}
		}
	}
	return v
}

// EnsureStarterRoster materializes the starter set of entities the first time a
// Symbiont's roster is read (idempotent: no-op once any entity exists). The set
// is derived from the Resonance Pool per canon.StarterRoster. Safe to call for
// any player — a zero pool still yields the guaranteed assault entity, but the
// caller (symbiont handler) only invokes this for Symbionts.
func (s *Service) EnsureStarterRoster(ctx context.Context, playerID string) error {
	if s.entities == nil {
		return nil
	}
	n, err := s.entities.CountByPlayer(ctx, playerID)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	pool := 0
	if s.resonance != nil {
		if p, _, _, rErr := s.resonance.ResonanceProgressOf(ctx, playerID); rErr == nil {
			pool = p
		}
	}
	for _, archetype := range canon.StarterRoster(pool) {
		e := &Entity{PlayerID: playerID, Archetype: archetype, Attunement: 1, Status: canon.EntityAvailable}
		if err := s.entities.Create(ctx, e); err != nil {
			return err
		}
	}
	return nil
}

// SyncStarterRoster tops the roster up to what the given pool grants: any
// archetype from canon.StarterRoster(pool) not yet materialized is created.
// Never deletes. Needed because the tutorial materializes the guaranteed
// assault at pool 0 before the real Pool conversion happens on the actual
// faction choice — EnsureStarterRoster alone no-ops once any entity exists.
func (s *Service) SyncStarterRoster(ctx context.Context, playerID string, pool int) error {
	if s.entities == nil {
		return nil
	}
	existing, err := s.entities.ListByPlayer(ctx, playerID)
	if err != nil {
		return err
	}
	have := map[string]int{}
	for _, e := range existing {
		have[e.Archetype]++
	}
	for _, archetype := range canon.StarterRoster(pool) {
		if have[archetype] > 0 {
			have[archetype]--
			continue
		}
		e := &Entity{PlayerID: playerID, Archetype: archetype, Attunement: 1, Status: canon.EntityAvailable}
		if err := s.entities.Create(ctx, e); err != nil {
			return err
		}
	}
	return nil
}

// ListEntities returns the player's roster (materializing the starter set on
// first read) as DTOs, evaluated against the current Resonance Level.
func (s *Service) ListEntities(ctx context.Context, playerID string) ([]EntityView, error) {
	if s.entities == nil {
		return nil, httputil.NewBadRequest("unavailable", "ростер недоступен")
	}
	if err := s.EnsureStarterRoster(ctx, playerID); err != nil {
		return nil, err
	}
	level := s.levelOf(ctx, playerID)
	list, err := s.entities.ListByPlayer(ctx, playerID)
	if err != nil {
		return nil, err
	}
	now := s.clock()
	views := make([]EntityView, 0, len(list))
	for i := range list {
		views = append(views, s.toView(&list[i], level, now))
	}
	return views, nil
}

// ControlUsage returns the control slots in use (sum of cap weights of engaged
// entities — manifesting + assigned), the cap for the player's Resonance Level,
// and that level.
func (s *Service) ControlUsage(ctx context.Context, playerID string) (used, capacity, level int, err error) {
	if s.entities == nil {
		return 0, 0, 1, nil
	}
	level = s.levelOf(ctx, playerID)
	capacity = canon.ResonanceControlCap(level)
	list, err := s.entities.ListByPlayer(ctx, playerID)
	if err != nil {
		return 0, capacity, level, err
	}
	for i := range list {
		if list[i].Status == canon.EntityManifesting || list[i].Status == canon.EntityAssigned {
			used += canon.EntityCapWeight(list[i].Archetype)
		}
	}
	return used, capacity, level, nil
}

// AssignEntity sends an available entity to a target (a hostile beacon or a
// rift, matching its archetype's kind). Enforces: ownership, the entity is idle,
// the archetype is unlocked at the player's Resonance Level, the target kind
// matches, and the control-slot cap is not exceeded. The entity enters
// manifesting and begins acting after its manifest delay.
func (s *Service) AssignEntity(ctx context.Context, playerID, entityID, targetKind, targetID string) (*EntityView, error) {
	e, spec, level, err := s.validateAssign(ctx, playerID, entityID)
	if err != nil {
		return nil, err
	}
	if targetKind != spec.TargetKind {
		return nil, httputil.NewBadRequest("wrong_target", "этот архетип так не назначить")
	}
	if targetID == "" {
		return nil, httputil.NewBadRequest("no_target", "не указана цель")
	}
	return s.applyAssign(ctx, e, spec, level, targetKind, targetID)
}

// AssignEntityAuto dispatches an entity to a target it auto-acquires near
// (lat,lng): the weakest hostile beacon (tower archetypes) or the nearest open
// rift (rift archetypes). This is the client-facing flow — you dispatch the
// entity and it finds its mark.
func (s *Service) AssignEntityAuto(ctx context.Context, playerID, entityID string, lat, lng float64) (*EntityView, error) {
	e, spec, level, err := s.validateAssign(ctx, playerID, entityID)
	if err != nil {
		return nil, err
	}
	var targetID string
	switch spec.TargetKind {
	case canon.EntityTargetTower:
		if s.corroder == nil {
			return nil, httputil.NewBadRequest("unavailable", "цели недоступны")
		}
		w, wErr := s.corroder.WeakestHostile(ctx, lat, lng, playerID, canon.EntityTargetRangeM)
		if wErr != nil {
			return nil, wErr
		}
		if w == nil || !w.Found {
			return nil, httputil.NewBadRequest("no_target", "рядом нет вражеского маяка для этой сущности")
		}
		targetID = w.TowerID
	case canon.EntityTargetRift:
		if s.amplifier == nil {
			return nil, httputil.NewBadRequest("unavailable", "цели недоступны")
		}
		r, rErr := s.amplifier.NearestOpenRift(ctx, lat, lng, canon.EntityTargetRangeM)
		if rErr != nil {
			return nil, rErr
		}
		if r == nil {
			return nil, httputil.NewBadRequest("no_target", "рядом нет открытого разлома для этой сущности")
		}
		targetID = r.ID
	default:
		return nil, httputil.NewBadRequest("wrong_target", "у архетипа нет цели")
	}
	return s.applyAssign(ctx, e, spec, level, spec.TargetKind, targetID)
}

// validateAssign runs the ownership / idle / unlock checks shared by both assign
// paths and returns the entity, its spec, and the player's Resonance Level.
func (s *Service) validateAssign(ctx context.Context, playerID, entityID string) (*Entity, canon.EntitySpec, int, error) {
	if s.entities == nil {
		return nil, canon.EntitySpec{}, 0, httputil.NewBadRequest("unavailable", "ростер недоступен")
	}
	e, err := s.entities.GetByID(ctx, entityID)
	if err != nil || e == nil {
		return nil, canon.EntitySpec{}, 0, httputil.NewNotFound("not_found", "сущность не найдена")
	}
	if e.PlayerID != playerID {
		return nil, canon.EntitySpec{}, 0, httputil.NewForbidden("not_owner", "это не ваша сущность")
	}
	if e.Status != canon.EntityAvailable {
		return nil, canon.EntitySpec{}, 0, httputil.NewBadRequest("busy", "сущность уже занята")
	}
	spec, ok := canon.EntitySpecFor(e.Archetype)
	if !ok {
		return nil, canon.EntitySpec{}, 0, httputil.NewBadRequest("unknown_archetype", "неизвестный архетип")
	}
	level := s.levelOf(ctx, playerID)
	if level < spec.UnlockLevel {
		return nil, canon.EntitySpec{}, 0, httputil.NewBadRequest("locked", "архетип ещё не разблокирован по Резонанс-уровню")
	}
	return e, spec, level, nil
}

// applyAssign enforces the control cap and moves the entity into manifesting.
func (s *Service) applyAssign(ctx context.Context, e *Entity, spec canon.EntitySpec, level int, targetKind, targetID string) (*EntityView, error) {
	used, capacity, _, err := s.ControlUsage(ctx, e.PlayerID)
	if err != nil {
		return nil, err
	}
	if used+spec.CapWeight > capacity {
		return nil, httputil.NewBadRequest("no_slots", "не хватает слотов контроля")
	}
	now := s.clock()
	manifestAt := now.Add(time.Duration(spec.ManifestMin) * time.Minute)
	e.Status = canon.EntityManifesting
	e.TargetKind = &targetKind
	e.TargetID = &targetID
	e.ManifestAt = &manifestAt
	e.RecoveryAt = nil
	if err := s.entities.Update(ctx, e); err != nil {
		return nil, err
	}
	v := s.toView(e, level, now)
	return &v, nil
}

// RecallEntity disengages an assigned/manifesting entity into recovery (it frees
// its control slot only once recovery completes). No-op-ish for already idle
// entities (returns a clear error).
func (s *Service) RecallEntity(ctx context.Context, playerID, entityID string) (*EntityView, error) {
	if s.entities == nil {
		return nil, httputil.NewBadRequest("unavailable", "ростер недоступен")
	}
	e, err := s.entities.GetByID(ctx, entityID)
	if err != nil || e == nil {
		return nil, httputil.NewNotFound("not_found", "сущность не найдена")
	}
	if e.PlayerID != playerID {
		return nil, httputil.NewForbidden("not_owner", "это не ваша сущность")
	}
	if e.Status != canon.EntityManifesting && e.Status != canon.EntityAssigned {
		return nil, httputil.NewBadRequest("not_engaged", "сущность не назначена")
	}
	now := s.clock()
	s.disengage(e, now)
	if err := s.entities.Update(ctx, e); err != nil {
		return nil, err
	}
	v := s.toView(e, s.levelOf(ctx, playerID), now)
	return &v, nil
}

// disengage moves an entity into recovery and clears its target (in place). The
// recovery window scales with Attunement (rank III bounces back faster, L3).
func (s *Service) disengage(e *Entity, now time.Time) {
	spec, _ := canon.EntitySpecFor(e.Archetype)
	recoveryMin := float64(spec.RecoveryMin) * canon.AttunementRecoveryFactor(e.Attunement)
	recoveryAt := now.Add(time.Duration(recoveryMin * float64(time.Minute)))
	e.Status = canon.EntityRecovering
	e.RecoveryAt = &recoveryAt
	e.ManifestAt = nil
	e.TargetKind = nil
	e.TargetID = nil
}

// RunTick advances every engaged entity one autonomous step (R4-2 L2): promote
// due manifesting→assigned, apply each assigned entity's effect to its target
// (corrode a beacon / amplify a rift; disengage if the target is gone), and wake
// due recovering→available. Intended to run on the EntityTickIntervalMin cadence.
func (s *Service) RunTick(ctx context.Context) (applied, disengaged int, err error) {
	if s.entities == nil {
		return 0, 0, nil
	}
	engaged, err := s.entities.ListEngaged(ctx)
	if err != nil {
		return 0, 0, err
	}
	now := s.clock()
	for i := range engaged {
		e := &engaged[i]
		switch e.Status {
		case canon.EntityManifesting:
			if e.ManifestAt != nil && !now.Before(*e.ManifestAt) {
				e.Status = canon.EntityAssigned
				e.ManifestAt = nil
				if uErr := s.entities.Update(ctx, e); uErr != nil {
					slog.Warn("entity tick: promote failed", "entity_id", e.ID, "error", uErr)
				}
			}
		case canon.EntityAssigned:
			gone, used := s.applyEffect(ctx, e)
			if used {
				// Attunement grows through use (L3): bank the use and promote
				// the rank when it crosses a threshold.
				e.AttunementUses++
				e.Attunement = canon.AttunementRankForUses(e.AttunementUses)
			}
			if gone {
				s.disengage(e, now)
				disengaged++
			}
			if uErr := s.entities.Update(ctx, e); uErr != nil {
				slog.Warn("entity tick: update failed", "entity_id", e.ID, "error", uErr)
			} else if used {
				applied++
			}
		case canon.EntityRecovering:
			if e.RecoveryAt != nil && !now.Before(*e.RecoveryAt) {
				e.Status = canon.EntityAvailable
				e.RecoveryAt = nil
				if uErr := s.entities.Update(ctx, e); uErr != nil {
					slog.Warn("entity tick: wake failed", "entity_id", e.ID, "error", uErr)
				}
			}
		}
	}
	slog.Info("symbiont entity tick", "engaged", len(engaged), "applied", applied, "disengaged", disengaged)
	return applied, disengaged, nil
}

// applyEffect applies one assigned entity's autonomous effect, scaled by its
// Attunement rank (L3). Returns gone=true if the target is gone (caller
// disengages) and used=true if the effect actually landed on a live target
// (which banks an Attunement use). Missing effector → no-op (not gone, not
// used), so a misconfigured deploy doesn't recall everyone.
func (s *Service) applyEffect(ctx context.Context, e *Entity) (gone, used bool) {
	if e.TargetKind == nil || e.TargetID == nil {
		return true, false // malformed assignment → disengage
	}
	spec, ok := canon.EntitySpecFor(e.Archetype)
	if !ok {
		return true, false
	}
	mul := canon.AttunementEffectMultiplier(e.Attunement)
	switch *e.TargetKind {
	case canon.EntityTargetTower:
		if s.corroder == nil {
			return false, false
		}
		damage := scaleEffect(spec.TowerDamage, mul)
		res, err := s.corroder.CorrodeTower(ctx, *e.TargetID, e.PlayerID, damage)
		if err != nil {
			slog.Warn("entity tick: corrode failed", "entity_id", e.ID, "error", err)
			return false, false
		}
		if !res.Found {
			return true, false // target gone → disengage, no use banked
		}
		return res.Destroyed, true // destroyed → disengage, but the hit counts
	case canon.EntityTargetRift:
		if s.amplifier == nil {
			return false, false
		}
		delta := scaleEffect(spec.RiftAmplify, mul)
		open, _, err := s.amplifier.AmplifyRift(ctx, *e.TargetID, delta)
		if err != nil {
			slog.Warn("entity tick: amplify failed", "entity_id", e.ID, "error", err)
			return false, false
		}
		if !open {
			return true, false
		}
		return false, true
	default:
		return true, false
	}
}

// scaleEffect applies an Attunement multiplier to a base magnitude, rounding to
// the nearest int with a floor of 1 so a scaled effect never vanishes.
func scaleEffect(base int, mul float64) int {
	v := int(float64(base)*mul + 0.5)
	if v < 1 {
		v = 1
	}
	return v
}

// VerbBonus is the aggregate augmentation an active roster gives the player's
// verbs (R4-2 L5). Zero value = no roster effect.
type VerbBonus struct {
	OverloadDamage int `json:"overload_damage"` // + to Overload damage
	ReconRangeM    int `json:"recon_range_m"`   // + to Recon range
	DrainReduction int `json:"drain_reduction"` // − to soft-drain energy
}

// VerbBonuses sums the verb augmentations from a player's ASSIGNED entities,
// each scaled by its Attunement rank (L5). Manifesting/recovering entities don't
// contribute — only those actively projecting.
func (s *Service) VerbBonuses(ctx context.Context, playerID string) (VerbBonus, error) {
	var b VerbBonus
	if s.entities == nil {
		return b, nil
	}
	list, err := s.entities.ListByPlayer(ctx, playerID)
	if err != nil {
		return b, err
	}
	for i := range list {
		e := &list[i]
		if e.Status != canon.EntityAssigned {
			continue
		}
		mul := canon.AttunementEffectMultiplier(e.Attunement)
		switch e.Archetype {
		case canon.EntityDistortion:
			b.OverloadDamage += scaleEffect(canon.EntityOverloadBonusBase, mul)
		case canon.EntityScout:
			b.ReconRangeM += scaleEffect(canon.EntityReconRangeBonusBase, mul)
		case canon.EntityAbsorber:
			b.DrainReduction += scaleEffect(canon.EntityDrainReductionBase, mul)
		}
	}
	return b, nil
}

// levelOf reads the player's Resonance Level (defaults to RL1).
func (s *Service) levelOf(ctx context.Context, playerID string) int {
	if s.resonance == nil {
		return 1
	}
	if _, level, _, err := s.resonance.ResonanceProgressOf(ctx, playerID); err == nil && level >= 1 {
		return level
	}
	return 1
}
