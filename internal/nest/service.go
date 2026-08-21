package nest

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ezra-game/server/internal/canon"
	"github.com/ezra-game/server/internal/cell"
	"github.com/ezra-game/server/pkg/httputil"
)

// CellLocator resolves a cell id to its coordinates (kept minimal so the
// service stays nil-safe / testable without the full cell repository).
type CellLocator interface {
	GetByID(ctx context.Context, id string) (*cell.Cell, error)
}

// crystalSpender charges crystals atomically for a paid relocation/rebuild
// (past the first free one). Optional — nil in tests skips billing.
type crystalSpender interface {
	SpendCrystals(ctx context.Context, playerID string, amount int) (bool, error)
}

// ResonanceGranter moves collected trickle from the nest buffer into the
// player's profile (ADR-N3-7). Optional — nil skips the grant (tests).
type ResonanceGranter interface {
	AddSymbiontResonance(ctx context.Context, playerID string, delta int) (int, error)
}

// EventPublisher fans a realtime event out to a player (implemented by
// realtime.Hub.PublishEvent). Optional — nil skips the telegraph.
type EventPublisher interface {
	PublishEvent(playerID, eventType string, data map[string]any)
}

// DefenseModifier lengthens a nest's siege window (stronger defense). This is
// the N4 seam (ADR-N3-5): N3 registers skillDefenderModifier; N4 adds a
// spirit-garrison modifier through the same interface without touching the
// state-machine. A multiplier of 1.0 means no effect.
type DefenseModifier interface {
	DefenseMultiplier(ctx context.Context, n *Nest) (float64, error)
}

// PlayerPlacer resolves a placement cell at the player's current position when
// no cell id is supplied (nest onboarding: "open at where I stand", ADR-N3-11
// guaranteed site). Optional — without it an empty cell id is an error.
type PlayerPlacer interface {
	ResolvePlayerPlacement(ctx context.Context, playerID string) (lat, lng float64, cellID string, err error)
}

// FactionGate reports whether a player may OWN a nest — only a COMMITTED
// Symbiont (faction=symbiont AND chosen=true) can (ADR-N3-11). This stops a
// human from opening a nest (which would farm faction-war territory), and stops
// a temporary onboarding symbiont (chosen=false) from leaving an orphan nest
// after reverting to human. Optional — nil skips the gate (tests).
type FactionGate interface {
	CanOwnNest(ctx context.Context, playerID string) (bool, error)
}

// Service holds the nest domain logic.
type Service struct {
	repo    Repository
	cells   CellLocator
	spender crystalSpender
	granter ResonanceGranter
	events  EventPublisher
	defense []DefenseModifier
	faction FactionGate
	placer  PlayerPlacer
}

// NewService wires the nest service. spender/granter may be nil (billing / grant
// skipped — used in tests).
func NewService(repo Repository, cells CellLocator, spender crystalSpender, granter ResonanceGranter) *Service {
	return &Service{repo: repo, cells: cells, spender: spender, granter: granter}
}

// WithEvents wires the realtime telegraph for siege warnings (fluent, optional).
func (s *Service) WithEvents(p EventPublisher) *Service { s.events = p; return s }

// WithFactionGate wires the "only a committed Symbiont may own a nest" gate
// (fluent, optional). Without it the gate is skipped (tests).
func (s *Service) WithFactionGate(g FactionGate) *Service { s.faction = g; return s }

// WithPlacer wires the "open at my current position" resolver (fluent, optional).
func (s *Service) WithPlacer(p PlayerPlacer) *Service { s.placer = p; return s }

// requireSymbiont rejects a non-Symbiont-owner attempt to hold a nest (ADR-N3-11).
func (s *Service) requireSymbiont(ctx context.Context, playerID string) error {
	if s.faction == nil {
		return nil
	}
	ok, err := s.faction.CanOwnNest(ctx, playerID)
	if err != nil {
		return fmt.Errorf("faction gate: %w", err)
	}
	if !ok {
		return httputil.NewBadRequest("not_symbiont",
			"гнездо доступно только симбионтам")
	}
	return nil
}

// GetByID returns a nest by id (satisfies battle.NestProvider).
func (s *Service) GetByID(ctx context.Context, id string) (*Nest, error) {
	return s.repo.GetByID(ctx, id)
}

// GetNearby returns live nests near a point (attacker-side map, T-849).
func (s *Service) GetNearby(ctx context.Context, lat, lng, radiusM float64) ([]Nest, error) {
	return s.repo.GetNearby(ctx, lat, lng, radiusM)
}

// GetForOwner returns the player's live nest, or (nil, nil) if none (API read).
func (s *Service) GetForOwner(ctx context.Context, ownerID string) (*Nest, error) {
	return s.repo.GetLiveByOwner(ctx, ownerID)
}

// AddDefenseModifier registers a siege-window modifier (ADR-N3-5 seam). N3 adds
// the Defender-skill modifier; N4 adds the spirit garrison the same way.
func (s *Service) AddDefenseModifier(m DefenseModifier) { s.defense = append(s.defense, m) }

// defenseWindow computes the wall-clock siege window: base(level) × Π(modifiers),
// floored at NestMinReactionFloor (ADR-N3-4 invariant 1 — the honest ETA floor).
func (s *Service) defenseWindow(ctx context.Context, n *Nest) time.Duration {
	w := float64(canon.NestDefenseWindow(n.Level))
	for _, m := range s.defense {
		mult, err := m.DefenseMultiplier(ctx, n)
		if err != nil {
			slog.Error("nest defense modifier failed", "nest_id", n.ID, "error", err)
			continue
		}
		if mult > 0 {
			w *= mult
		}
	}
	win := time.Duration(w)
	if win < canon.NestMinReactionFloor {
		win = canon.NestMinReactionFloor
	}
	return win
}

// OpenFirstNest opens a player's FIRST nest, free, at onboarding (ADR-N3-11).
// It refuses if the player already owns any nest row (live or history) — a
// rebuild after collapse goes through Create (paid). Placement validation
// (dome/pocket, guaranteed site) lands in T-848/T-843; here we resolve and plant.
func (s *Service) OpenFirstNest(ctx context.Context, ownerID, cellID string) (*Nest, error) {
	if err := s.requireSymbiont(ctx, ownerID); err != nil {
		return nil, err
	}
	everOwned, err := s.repo.HasEverOwned(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	if everOwned {
		return nil, httputil.NewBadRequest("nest_exists",
			"у вас уже было гнездо; используйте восстановление, не первичное открытие")
	}
	return s.plant(ctx, ownerID, cellID)
}

// Create (re)builds a nest after the first one — the paid rebuild path. The DB
// partial-UNIQUE index is the ultimate cap-1-live guard; we check first for a
// friendly error. Rebuild after collapse costs crystals (part of the
// Executable-Loss corridor, ADR-N3-7 — full calibration in T-842).
func (s *Service) Create(ctx context.Context, ownerID, cellID string) (*Nest, error) {
	if err := s.requireSymbiont(ctx, ownerID); err != nil {
		return nil, err
	}
	live, err := s.repo.GetLiveByOwner(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	if live != nil {
		return nil, httputil.NewBadRequest("nest_exists", "у вас уже есть гнездо; используйте перенос")
	}
	everOwned, err := s.repo.HasEverOwned(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	if everOwned {
		if err := s.charge(ctx, ownerID, canon.NestRelocationCostCrystals,
			"восстановление гнезда стоит %d 💎"); err != nil {
			return nil, err
		}
	}
	return s.plant(ctx, ownerID, cellID)
}

// Relocate moves the player's live nest (ADR-N3-6, mirror of RelocateCore):
// first move free, later ones cost crystals + cooldown; blocked while the nest
// is under siege (anti-escape); level/buffer/siege are preserved.
func (s *Service) Relocate(ctx context.Context, ownerID, cellID string) (*Nest, error) {
	n, err := s.repo.GetLiveByOwner(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	if n == nil {
		return nil, httputil.NewBadRequest("no_nest", "нет гнезда для переноса; сначала откройте его")
	}
	// Anti-escape: cannot flee a raid mid-siege (ADR-N3-6). The single-writer
	// nest:tick makes "collapse beats relocation" deterministic.
	if n.IsUnderThreat() {
		return nil, httputil.NewBadRequest("nest_under_siege",
			"нельзя переносить гнездо под штурмом")
	}

	lat, lng, resolvedCell, err := s.resolvePoint(ctx, ownerID, cellID)
	if err != nil {
		return nil, err
	}
	pocket, err := s.checkPlacement(ctx, resolvedCell)
	if err != nil {
		return nil, err
	}

	// First relocation free; later ones billed only AFTER the target validates.
	if n.RelocatedAt != nil {
		if err := s.charge(ctx, ownerID, canon.NestRelocationCostCrystals,
			"перенос гнезда стоит %d 💎"); err != nil {
			return nil, err
		}
	}

	n.CellID = resolvedCell
	n.Lat = lat
	n.Lng = lng
	if err := s.repo.UpdateLocation(ctx, n); err != nil {
		return nil, err
	}
	if pocket {
		if err := s.repo.AddPocketCell(ctx, n.ID, resolvedCell); err != nil {
			slog.Error("record nest pocket cell failed", "nest_id", n.ID, "cell_id", resolvedCell, "error", err)
		}
	}
	slog.Info("nest relocated", "owner_id", ownerID, "cell_id", resolvedCell, "pocket", pocket)
	return s.repo.GetLiveByOwner(ctx, ownerID)
}

// plant resolves the placement point and inserts a level-1 nest.
func (s *Service) plant(ctx context.Context, ownerID, cellID string) (*Nest, error) {
	lat, lng, resolvedCell, err := s.resolvePoint(ctx, ownerID, cellID)
	if err != nil {
		return nil, err
	}
	pocket, err := s.checkPlacement(ctx, resolvedCell)
	if err != nil {
		return nil, err
	}
	n := &Nest{OwnerID: ownerID, CellID: resolvedCell, Lat: lat, Lng: lng, Level: 1}
	if err := s.repo.Create(ctx, n); err != nil {
		return nil, err
	}
	if pocket {
		// The nest holds the carved pocket cell open (T-843); the tick refreshes
		// its pierced_until so it never lapses while the nest lives.
		if err := s.repo.AddPocketCell(ctx, n.ID, resolvedCell); err != nil {
			slog.Error("record nest pocket cell failed", "nest_id", n.ID, "cell_id", resolvedCell, "error", err)
		}
	}
	n.decorate() // guarantee client fields regardless of the repo implementation
	slog.Info("nest opened", "owner_id", ownerID, "cell_id", resolvedCell, "level", 1, "pocket", pocket)
	return n, nil
}

// checkPlacement enforces "no nest under a foreign dome, except in a carved
// pocket" (ADR-N3-8, symbiont_geo_playstyle.md §2.4). Returns whether the cell
// is a pocket (currently pierced) so the caller records it for tick-refresh.
func (s *Service) checkPlacement(ctx context.Context, cellID string) (pocket bool, err error) {
	domed, pierced, err := s.repo.CellPlacementStatus(ctx, cellID)
	if err != nil {
		return false, err
	}
	if domed && !pierced {
		return false, httputil.NewBadRequest("under_foreign_dome",
			"нельзя ставить гнездо под чужим куполом — сначала выгрызи карман")
	}
	return pierced, nil
}

// resolvePoint turns a cell id into coordinates. An empty cell id means "open at
// where I stand": it resolves the player's current position via the placer
// (guaranteed site, ADR-N3-11). Dome/pocket validation is applied by the caller.
func (s *Service) resolvePoint(ctx context.Context, ownerID, cellID string) (lat, lng float64, resolved string, err error) {
	if cellID == "" {
		if s.placer == nil {
			return 0, 0, "", httputil.NewBadRequest("no_cell", "укажите клетку для гнезда")
		}
		return s.placer.ResolvePlayerPlacement(ctx, ownerID)
	}
	c, err := s.cells.GetByID(ctx, cellID)
	if err != nil {
		return 0, 0, "", fmt.Errorf("locate cell: %w", err)
	}
	if c == nil {
		return 0, 0, "", httputil.NewNotFound("not_found", "клетка не найдена")
	}
	return c.Lat, c.Lng, cellID, nil
}

// charge spends crystals if a spender is wired, returning a friendly error when
// the player can't afford it. msgf must contain a single %d for the amount.
func (s *Service) charge(ctx context.Context, playerID string, amount int, msgf string) error {
	if amount <= 0 || s.spender == nil {
		return nil
	}
	paid, err := s.spender.SpendCrystals(ctx, playerID, amount)
	if err != nil {
		return fmt.Errorf("charge nest: %w", err)
	}
	if !paid {
		return httputil.NewBadRequest("insufficient_crystals", fmt.Sprintf(msgf, amount))
	}
	return nil
}

// Feed restores the player's nest support to full (T-834, hive.Empower
// semantics): the daily "tend the garden" action that resets the decay timer.
func (s *Service) Feed(ctx context.Context, ownerID string) (*Nest, error) {
	n, err := s.repo.GetLiveByOwner(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	if n == nil {
		return nil, httputil.NewBadRequest("no_nest", "нет гнезда для подпитки")
	}
	if err := s.repo.Feed(ctx, n.ID, canon.NestSupportMax); err != nil {
		return nil, err
	}
	return s.repo.GetLiveByOwner(ctx, ownerID)
}

// Collect moves the nest's accrued trickle buffer into the player's profile
// Resonance (ADR-N3-7) and zeroes the buffer. The floored integer is granted;
// the sub-1 fractional remainder is dropped (buffer is zeroed) — negligible and
// keeps the profile an integer store. Returns the amount granted.
func (s *Service) Collect(ctx context.Context, ownerID string) (int, error) {
	n, err := s.repo.GetLiveByOwner(ctx, ownerID)
	if err != nil {
		return 0, err
	}
	if n == nil {
		return 0, httputil.NewBadRequest("no_nest", "нет гнезда для сбора Резонанса")
	}
	collected, err := s.repo.CollectBuffer(ctx, n.ID)
	if err != nil {
		return 0, err
	}
	amount := int(collected)
	if amount > 0 && s.granter != nil {
		if _, err := s.granter.AddSymbiontResonance(ctx, ownerID, amount); err != nil {
			return 0, fmt.Errorf("grant collected resonance: %w", err)
		}
	}
	slog.Info("nest resonance collected", "owner_id", ownerID, "amount", amount)
	return amount, nil
}

// Tick runs one nest lifecycle pass (ADR-N3-9): accrue trickle into buffers and
// apply support decay. Set-based, invoked by nest:tick. Siege advance / pocket
// refresh are added with the defense wave (T-836..843/T-850 completion).
func (s *Service) Tick(ctx context.Context) error {
	var trickle [canon.NestMaxLevel + 1]float64
	for lvl := 1; lvl <= canon.NestMaxLevel; lvl++ {
		trickle[lvl] = canon.NestConfig(lvl).TricklePerTick
	}
	if _, err := s.repo.AccrueTrickle(ctx, trickle, canon.NestTrickleCap, canon.NestEnergistTricklePerPoint); err != nil {
		return err
	}
	if _, err := s.repo.ApplyDecay(ctx, canon.NestSupportDecayPerTick, canon.NestSupportFloor); err != nil {
		return err
	}
	// Terminal collapse is applied ONLY here (single-writer, ADR-N3-4 inv. 2):
	// nests past their collapse_at are marked collapsed and their buffer zeroed.
	collapsed, err := s.repo.ApplyCollapses(ctx)
	if err != nil {
		return err
	}
	for _, id := range collapsed {
		slog.Info("nest collapsed", "nest_id", id)
	}
	// Hold each live nest's pocket cells open (T-843). Collapsed nests are
	// skipped inside the query, so their cells lapse on their own.
	if _, err := s.repo.RefreshPocketCells(ctx, canon.NestPocketHoldSeconds); err != nil {
		return err
	}
	return nil
}

// OnAssaultVictory records a won human assault against a nest (ADR-N3-4,
// ADR-N3-10). It NEVER closes the nest (unlike hive.OnAssaultVictory): it drains
// siege HP, arms/advances the collapse timer (floored so an offline owner always
// gets a reaction window), and telegraphs the attack with an honest ETA. The
// terminal collapse is applied only by nest:tick (single-writer invariant).
func (s *Service) OnAssaultVictory(ctx context.Context, nestID, attackerID string) error {
	n, err := s.repo.GetByID(ctx, nestID)
	if err != nil {
		return err
	}
	if n.CollapsedAt != nil || n.SiegeState == SiegeCollapsed {
		return nil // already gone; nothing to grind
	}

	n.SiegeHP -= canon.NestAssaultDamage
	if n.SiegeHP < 0 {
		n.SiegeHP = 0
	}
	n.SiegeAttackerID = &attackerID

	now := time.Now()
	proposed := now.Add(s.defenseWindow(ctx, n))
	floor := now.Add(canon.NestMinReactionFloor)
	switch n.SiegeState {
	case SiegeHealthy:
		// First assault arms the timer.
		n.SiegeState = SiegeUnderSiege
		n.CollapseAt = &proposed
	default:
		// Repeat assault: bring the collapse nearer, but never below the floor.
		if n.CollapseAt == nil || proposed.Before(*n.CollapseAt) {
			n.CollapseAt = &proposed
		}
	}
	if n.CollapseAt != nil && n.CollapseAt.Before(floor) {
		n.CollapseAt = &floor
	}
	if n.SiegeHP <= 0 {
		n.SiegeState = SiegeCollapsing
	}

	if err := s.repo.UpdateSiege(ctx, n); err != nil {
		return err
	}
	s.telegraph(n, now)
	slog.Info("nest under assault", "nest_id", nestID, "attacker_id", attackerID,
		"siege_hp", n.SiegeHP, "state", n.SiegeState)
	return nil
}

// RepairAlly lets any Symbiont pour into a FELLOW Symbiont's nest (coop defense,
// T-840). The actor must be a committed Symbiont; the target nest is repaired
// regardless of owner. Prevents humans from "repairing" (griefing/probing) nests.
func (s *Service) RepairAlly(ctx context.Context, actorID, nestID string) (*Nest, error) {
	if err := s.requireSymbiont(ctx, actorID); err != nil {
		return nil, err
	}
	return s.Repair(ctx, nestID)
}

// Repair restores a nest's siege HP and cancels a pending collapse (ADR-N3-4
// invariant 3, hive.Empower semantics). Allowed to allied Symbionts too — the
// coop defense (T-840): anyone of the faction near the nest can pour in.
func (s *Service) Repair(ctx context.Context, nestID string) (*Nest, error) {
	n, err := s.repo.GetByID(ctx, nestID)
	if err != nil {
		return nil, err
	}
	if n.CollapsedAt != nil {
		return nil, httputil.NewBadRequest("nest_collapsed", "гнездо уже разрушено")
	}
	n.SiegeHP = n.Config().SiegeHPMax
	n.SiegeState = SiegeHealthy
	n.CollapseAt = nil
	n.SiegeAttackerID = nil
	if err := s.repo.UpdateSiege(ctx, n); err != nil {
		return nil, err
	}
	slog.Info("nest repaired", "nest_id", nestID)
	return s.repo.GetByID(ctx, nestID)
}

// telegraph emits a realtime "nest_under_attack" event with an ETA (seconds to
// the earliest collapse) so the owner — even offline via the FCM fan-out — has a
// reaction window (venue-safety, ADR-N3-4 invariant 1).
func (s *Service) telegraph(n *Nest, now time.Time) {
	if s.events == nil || n.CollapseAt == nil {
		return
	}
	eta := int(n.CollapseAt.Sub(now).Seconds())
	if eta < 0 {
		eta = 0
	}
	s.events.PublishEvent(n.OwnerID, "nest_under_attack", map[string]any{
		"nest_id":     n.ID,
		"siege_hp":    n.SiegeHP,
		"siege_state": n.SiegeState,
		"eta_seconds": eta,
	})
}
