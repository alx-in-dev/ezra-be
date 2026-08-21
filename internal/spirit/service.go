package spirit

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"github.com/ezra-game/server/internal/canon"
	"github.com/ezra-game/server/pkg/geo"
	"github.com/ezra-game/server/pkg/httputil"
)

// ── Ports (kept minimal so the service stays testable) ──────────────────────

// BeaconOps resolves a beacon's owner and applies spirit brownout pressure.
type BeaconOps interface {
	OwnerOf(ctx context.Context, towerID string) (string, error)
	SetSpiritPressure(ctx context.Context, towerID string, until time.Time) error
}

// ResourceDebiter debits the wave-drain from the beacon owner (best-effort).
type ResourceDebiter interface {
	SpendEnergy(ctx context.Context, playerID string, energy int) error
}

// NetworkRecomputer recomputes an owner's dome after a brownout push (T-864).
type NetworkRecomputer interface {
	Recompute(ctx context.Context, playerID string) error
}

// EventPublisher fans a realtime event to a player (realtime.Hub.PublishEvent).
type EventPublisher interface {
	PublishEvent(playerID, eventType string, data map[string]any)
}

// ProfileReader reads a player's level + Resonance Level (novice gate + tame gate).
type ProfileReader interface {
	LevelAndResonanceLevel(ctx context.Context, playerID string) (level, resonanceLevel int, err error)
}

// Exhauster sets a player's "Истощение" debuff window after a spirit touch.
type Exhauster interface {
	SetExhaustion(ctx context.Context, playerID string, until time.Time) error
}

// RosterGranter adds a tamed spirit to the player's roster as an entity.
type RosterGranter interface {
	GrantEntity(ctx context.Context, playerID, archetype string) error
}

// SymbiontGate reports whether a player is a committed Symbiont (weaken/tame gate).
type SymbiontGate interface {
	CanOwnNest(ctx context.Context, playerID string) (bool, error)
}

// SurgeBroadcaster fans a world event (surge start / recovery) to every connected
// player (realtime.Hub.BroadcastEvent). Optional — nil skips the telegraph.
type SurgeBroadcaster interface {
	BroadcastEvent(eventType string, data map[string]any)
}

// ItemDebiter atomically spends a fungible item — the hack_key sink behind the
// Ionized Charge repellent (T-881). Implemented by item.Service.
type ItemDebiter interface {
	Consume(ctx context.Context, playerID, itemType string, qty int) (bool, error)
}

// Service holds the wild-spirit domain logic.
type Service struct {
	repo    Repository
	beacons BeaconOps
	res     ResourceDebiter
	network NetworkRecomputer
	events  EventPublisher
	profile ProfileReader
	exhaust Exhauster
	roster  RosterGranter
	faction SymbiontGate
	surge   SurgeBroadcaster
	items   ItemDebiter
	rng     *rand.Rand

	// Seasonal surge state (T-880). surgeActive+surgeUntil gate a denser-wave
	// window; OFF by default (zero value) until a playtest trigger calls BeginSurge.
	surgeActive bool
	surgeUntil  time.Time
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, rng: rand.New(rand.NewSource(1))}
}

func (s *Service) WithBeacons(b BeaconOps, r ResourceDebiter, n NetworkRecomputer) *Service {
	s.beacons, s.res, s.network = b, r, n
	return s
}
func (s *Service) WithEvents(e EventPublisher) *Service    { s.events = e; return s }
func (s *Service) WithProfile(p ProfileReader) *Service    { s.profile = p; return s }
func (s *Service) WithExhauster(e Exhauster) *Service      { s.exhaust = e; return s }
func (s *Service) WithRoster(r RosterGranter) *Service     { s.roster = r; return s }
func (s *Service) WithFactionGate(g SymbiontGate) *Service { s.faction = g; return s }
func (s *Service) WithSurge(b SurgeBroadcaster) *Service   { s.surge = b; return s }
func (s *Service) WithItems(d ItemDebiter) *Service        { s.items = d; return s }

// Repel is the Ionized Charge repellent (T-881): spends hack_key, then clears
// every live spirit within RepellentRadiusM of the player. The cleared area is
// the protection window. Returns how many spirits were repelled. Charging the
// key first means a short player never loses the key with no effect.
func (s *Service) Repel(ctx context.Context, playerID string, lat, lng float64) (int, error) {
	if s.items != nil {
		ok, err := s.items.Consume(ctx, playerID, "hack_key", canon.RepellentHackKeyCost)
		if err != nil {
			return 0, err
		}
		if !ok {
			return 0, httputil.NewBadRequest("no_hack_key", "нужен «Ключ взлома» для ионизированного заряда")
		}
	}
	n, err := s.repo.RepelWithin(ctx, lat, lng, canon.RepellentRadiusM)
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// BeginSurge starts a telegraphed spirit-surge window (T-880): for its duration
// waves spawn denser (canon.SurgeBudgetMultiplier). Broadcasts the start so every
// player sees the spike coming; a recovery beat fires from the tick when it ends
// (White-Hat bookend). Playtest-triggered — nothing schedules it automatically,
// so surges stay OFF by default until the numbers are tuned.
func (s *Service) BeginSurge(d time.Duration) {
	if d <= 0 {
		return
	}
	s.surgeActive = true
	s.surgeUntil = time.Now().Add(d)
	if s.surge != nil {
		s.surge.BroadcastEvent("spirit_surge", map[string]any{
			"active":          true,
			"ends_in_seconds": int(d.Seconds()),
		})
	}
	slog.Info("spirit surge begun", "duration", d)
}

// surgeOn reports whether a surge window is currently active.
func (s *Service) surgeOn() bool { return s.surgeActive && time.Now().Before(s.surgeUntil) }

// Tick runs one spirit lifecycle pass (spirit:tick, ADR-N2): spawn new waves
// (telegraphing each ETA), apply arrivals (wave-drain + brownout cascade), and
// expire the old. Spawn/expire are set-based; the per-spawn/per-arrival Go loops
// run over a small, budget-bounded set (never the hot path).
func (s *Service) Tick(ctx context.Context) error {
	// T-880: close a finished surge window with a recovery beat (White-Hat
	// bookend) before spawning this tick's waves.
	if s.surgeActive && !time.Now().Before(s.surgeUntil) {
		s.surgeActive = false
		if s.surge != nil {
			s.surge.BroadcastEvent("spirit_surge_recovery", map[string]any{"active": false})
		}
		slog.Info("spirit surge ended (recovery)")
	}

	// T-872: slowly grow geysers (the rare class-V source) in deep Symbiont
	// territory. Gated behind a low probability + minimum spacing so class V stays
	// a special-place threat, not scenery.
	if rand.Float64() < canon.GeyserSeedChance {
		if n, gerr := s.repo.SeedGeyser(ctx, canon.GeyserMinGapM); gerr != nil {
			slog.Warn("geyser seed failed", "error", gerr)
		} else if n > 0 {
			slog.Info("geyser seeded")
		}
	}

	// T-880: a surge window temporarily raises the per-region budget → denser waves.
	regionBudget := canon.SpiritRegionBudget
	if s.surgeOn() {
		regionBudget = int(float64(regionBudget) * canon.SurgeBudgetMultiplier)
	}

	var speeds [canon.SpiritMaxClass + 1]float64
	for c := 1; c <= canon.SpiritMaxClass; c++ {
		speeds[c] = canon.SpiritConfig(c).SpeedMps
	}
	spawned, err := s.repo.SpawnWave(ctx, canon.SpiritRouteMaxM, regionBudget, speeds)
	if err != nil {
		return err
	}
	now := time.Now()
	for i := range spawned {
		s.telegraph(ctx, &spawned[i], now)
	}

	arrivals, err := s.repo.TakeArrivals(ctx)
	if err != nil {
		return err
	}
	for i := range arrivals {
		s.applyWave(ctx, &arrivals[i])
	}

	if _, err := s.repo.ExpireOld(ctx, canon.SpiritExpireAfter); err != nil {
		return err
	}
	if len(spawned) > 0 || len(arrivals) > 0 {
		slog.Info("spirit tick", "spawned", len(spawned), "arrived", len(arrivals))
	}
	return nil
}

// telegraph warns the target beacon's owner that a wave is inbound, with an ETA
// (§4.2: the soft-timer is built in — the owner has the whole flight to react).
func (s *Service) telegraph(ctx context.Context, sp *Spirit, now time.Time) {
	if s.events == nil || sp.TargetTowerID == nil {
		return
	}
	owner, err := s.ownerOf(ctx, *sp.TargetTowerID)
	if err != nil || owner == "" {
		return
	}
	eta := int(sp.ArriveAt.Sub(now).Seconds())
	if eta < 0 {
		eta = 0
	}
	s.events.PublishEvent(owner, "beacon_targeted", map[string]any{
		"tower_id":    *sp.TargetTowerID,
		"spirit_id":   sp.ID,
		"class":       sp.Class,
		"eta_seconds": eta,
	})
}

// applyWave discharges an arrived spirit on its target beacon: drains energy
// (Executable Loss, never a wipe) and — for a strong enough class — forces the
// perimeter beacon into a time-boxed spirit-brownout that shrinks the dome
// (T-864 cascade), then recomputes and notifies the owner.
func (s *Service) applyWave(ctx context.Context, sp *Spirit) {
	if sp.TargetTowerID == nil || s.beacons == nil {
		return
	}
	owner, err := s.ownerOf(ctx, *sp.TargetTowerID)
	if err != nil || owner == "" {
		return
	}
	drained := canon.SpiritWaveDrainEnergy(sp.Class)
	if s.res != nil {
		_ = s.res.SpendEnergy(ctx, owner, drained) // best-effort: drains up to what they have
	}

	brownout := sp.Class >= canon.SpiritBrownoutClass
	if brownout {
		until := time.Now().Add(canon.SpiritBrownoutWindowMinutes * time.Minute)
		if err := s.beacons.SetSpiritPressure(ctx, *sp.TargetTowerID, until); err != nil {
			slog.Error("set spirit pressure failed", "tower_id", *sp.TargetTowerID, "error", err)
		} else if s.network != nil {
			if err := s.network.Recompute(ctx, owner); err != nil {
				slog.Error("recompute after spirit brownout failed", "player_id", owner, "error", err)
			}
		}
	}
	if s.events != nil {
		s.events.PublishEvent(owner, "beacon_drained", map[string]any{
			"tower_id": *sp.TargetTowerID,
			"class":    sp.Class,
			"drained":  drained,
			"brownout": brownout,
		})
	}
}

// TouchCheck applies the field-touch "Истощение" debuff to a player who has
// walked within a wild spirit's danger radius — UNLESS they are a novice (fully
// inert, §4.3) or under a dome (no spirits there). Returns the touching class
// (0 = no touch). Called on player position updates.
func (s *Service) TouchCheck(ctx context.Context, playerID string, lat, lng float64) (int, error) {
	// Novice inertness first — cheapest gate, protects onboarding.
	if s.profile != nil {
		level, _, err := s.profile.LevelAndResonanceLevel(ctx, playerID)
		if err == nil && level <= canon.SpiritNoviceLevelFloor {
			return 0, nil
		}
	}
	cands, err := s.repo.TouchCandidates(ctx, lat, lng, canon.SpiritConfig(canon.SpiritMaxClass).DangerRadiusM)
	if err != nil {
		return 0, err
	}
	touchedClass := 0
	for i := range cands {
		if geo.Haversine(lat, lng, cands[i].Lat, cands[i].Lng) <= cands[i].Config().DangerRadiusM {
			if cands[i].Class > touchedClass {
				touchedClass = cands[i].Class
			}
		}
	}
	if touchedClass == 0 {
		return 0, nil
	}
	if s.exhaust != nil {
		until := time.Now().Add(canon.SpiritTouchDebuffMinutes * time.Minute)
		if err := s.exhaust.SetExhaustion(ctx, playerID, until); err != nil {
			slog.Error("set exhaustion failed", "player_id", playerID, "error", err)
		}
	}
	if s.events != nil {
		s.events.PublishEvent(playerID, "spirit_touch", map[string]any{
			"class":          touchedClass,
			"debuff_minutes": canon.SpiritTouchDebuffMinutes,
		})
	}
	return touchedClass, nil
}

// GetNearby returns live spirits near a point (map layer).
func (s *Service) GetNearby(ctx context.Context, lat, lng, radiusM float64) ([]Spirit, error) {
	return s.repo.GetNearby(ctx, lat, lng, radiusM)
}

// Weaken softens a wild spirit toward tameable (N4 loop). Symbiont-gated.
func (s *Service) Weaken(ctx context.Context, playerID, spiritID string) (*Spirit, error) {
	if err := s.requireSymbiont(ctx, playerID); err != nil {
		return nil, err
	}
	return s.repo.Weaken(ctx, spiritID, canon.SpiritWeakenPerAction)
}

// Tame attempts to tame a softened wild spirit into the roster (N4). Symbiont +
// RL gated; chance scales with class, RL and how softened the spirit is (weaken
// doubles as pity). On success the spirit joins the roster as its archetype.
func (s *Service) Tame(ctx context.Context, playerID, spiritID string) (bool, error) {
	if err := s.requireSymbiont(ctx, playerID); err != nil {
		return false, err
	}
	sp, err := s.repo.GetByID(ctx, spiritID)
	if err != nil {
		return false, err
	}
	if sp.State != StateWeakened || sp.WeakenedPct < canon.SpiritTameMinWeakened {
		return false, httputil.NewBadRequest("not_weakened", "сначала ослабь духа")
	}
	rl := 1
	if s.profile != nil {
		if _, r, e := s.profile.LevelAndResonanceLevel(ctx, playerID); e == nil {
			rl = r
		}
	}
	chance := canon.SpiritTameChance(sp.Class, rl, sp.WeakenedPct)
	if chance <= 0 {
		return false, httputil.NewBadRequest("rl_too_low",
			fmt.Sprintf("класс %d приручается с Resonance Level %d", sp.Class, sp.Config().TameRLGate))
	}
	if s.rng.Float64() > chance {
		// Miss: leave the spirit weakened (weaken more → better next time = pity).
		return false, nil
	}
	if err := s.repo.MarkTamed(ctx, spiritID, playerID); err != nil {
		return false, err
	}
	if s.roster != nil {
		if err := s.roster.GrantEntity(ctx, playerID, canon.SpiritArchetype(sp.Class)); err != nil {
			slog.Error("grant tamed entity failed", "player_id", playerID, "class", sp.Class, "error", err)
		}
	}
	slog.Info("spirit tamed", "player_id", playerID, "spirit_id", spiritID, "class", sp.Class)
	return true, nil
}

func (s *Service) requireSymbiont(ctx context.Context, playerID string) error {
	if s.faction == nil {
		return nil
	}
	ok, err := s.faction.CanOwnNest(ctx, playerID)
	if err != nil {
		return fmt.Errorf("faction gate: %w", err)
	}
	if !ok {
		return httputil.NewBadRequest("not_symbiont", "приручение доступно только симбионтам")
	}
	return nil
}

func (s *Service) ownerOf(ctx context.Context, towerID string) (string, error) {
	if s.beacons == nil {
		return "", nil
	}
	return s.beacons.OwnerOf(ctx, towerID)
}
