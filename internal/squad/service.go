package squad

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ezra-game/server/internal/canon"
	"github.com/ezra-game/server/internal/cell"
	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/internal/unit"
	"github.com/ezra-game/server/pkg/geo"
	"github.com/ezra-game/server/pkg/httputil"
)

// ResourceAdder credits a player's resources (mission rewards). Optional dep.
type ResourceAdder interface {
	Add(ctx context.Context, playerID string, energy, materials int) (*player.Player, error)
}

// MissionCells locates the target cell and can cleanse infection around it
// (patrol reward). Optional dep.
type MissionCells interface {
	GetByID(ctx context.Context, id string) (*cell.Cell, error)
	ClearInfectionInRadius(ctx context.Context, lat, lng, radiusM, target float64) (int64, error)
}

// MissionNotifier pushes the "squad returned" notification. Optional dep.
type MissionNotifier interface {
	NotifySquadReturned(ctx context.Context, playerID, missionType string) error
}

// PositionReader supplies the player's server-side position — the
// anti-remote-farm gate on Send. Optional dep.
type PositionReader interface {
	GetByID(ctx context.Context, id string) (*player.Player, error)
}

// CostSpender charges the mission dispatch cost. Optional dep.
type CostSpender interface {
	Spend(ctx context.Context, playerID string, energy, materials int) (*player.Player, error)
}

// Service handles squad business logic.
type Service struct {
	squads    Repository
	units     unit.Repository
	resources ResourceAdder
	cells     MissionCells
	push      MissionNotifier
	positions PositionReader
	spender   CostSpender
}

// SetMissionDeps wires the optional dependencies used to complete timed away
// missions (rewards, infection cleanse, push). Kept off the constructor so
// existing callers/tests stay unchanged.
func (s *Service) SetMissionDeps(resources ResourceAdder, cells MissionCells, push MissionNotifier) {
	s.resources = resources
	s.cells = cells
	s.push = push
}

// SetMissionGuards wires position/cost validation for Send (optional; nil in
// tests keeps legacy behaviour).
func (s *Service) SetMissionGuards(positions PositionReader, spender CostSpender) {
	s.positions = positions
	s.spender = spender
}

// NewService creates a new squad service.
func NewService(squads Repository, units unit.Repository) *Service {
	return &Service{squads: squads, units: units}
}

// Create forms a new squad from the given unit IDs.
func (s *Service) Create(ctx context.Context, playerID, name string, unitIDs []string) (*Squad, error) {
	if name == "" {
		return nil, httputil.NewBadRequest("invalid_name", "squad name is required")
	}
	if len(unitIDs) == 0 {
		return nil, httputil.NewBadRequest("no_units", "at least one unit is required")
	}

	fighterCount := 0
	// Validate all units belong to the player and are idle
	for _, uid := range unitIDs {
		u, err := s.units.GetByID(ctx, uid)
		if err != nil {
			return nil, httputil.NewNotFound("unit_not_found", "unit "+uid+" not found")
		}
		if u.PlayerID != playerID {
			return nil, httputil.NewForbidden("not_owner", "unit "+uid+" does not belong to you")
		}
		if u.Status != "idle" {
			return nil, httputil.NewBadRequest("unit_busy", "unit "+uid+" is not idle")
		}
		if u.SquadID != nil {
			return nil, httputil.NewBadRequest("unit_assigned", "unit "+uid+" is already in a squad")
		}
		if u.Type == "fighter" {
			fighterCount++
		}
	}

	sq := &Squad{
		PlayerID: playerID,
		Name:     name,
		Status:   "idle",
	}

	if err := s.squads.Create(ctx, sq); err != nil {
		return nil, httputil.NewInternal("failed to create squad")
	}

	// Assign units to the squad
	for _, uid := range unitIDs {
		if err := s.units.AssignSquad(ctx, uid, &sq.ID); err != nil {
			return nil, httputil.NewInternal("failed to assign unit to squad")
		}
	}
	sq.UnitCount = len(unitIDs)
	sq.FighterCount = fighterCount

	return sq, nil
}

// List returns player's squads with computed unit counts.
func (s *Service) List(ctx context.Context, playerID string) ([]Squad, error) {
	squads, err := s.squads.GetByPlayer(ctx, playerID)
	if err != nil {
		return nil, httputil.NewInternal("failed to list squads")
	}

	units, err := s.units.GetByPlayer(ctx, playerID)
	if err != nil {
		return nil, httputil.NewInternal("failed to list units")
	}

	counts := make(map[string]int, len(squads))
	fighterCounts := make(map[string]int, len(squads))
	for _, u := range units {
		if u.SquadID != nil {
			counts[*u.SquadID]++
			if u.Type == "fighter" {
				fighterCounts[*u.SquadID]++
			}
		}
	}
	for i := range squads {
		squads[i].UnitCount = counts[squads[i].ID]
		squads[i].FighterCount = fighterCounts[squads[i].ID]
	}
	return squads, nil
}

// Send dispatches a squad on a mission.
func (s *Service) Send(ctx context.Context, squadID, playerID, missionType, targetID string) (*Squad, error) {
	sq, err := s.squads.GetByID(ctx, squadID)
	if err != nil {
		return nil, httputil.NewNotFound("squad_not_found", "squad not found")
	}
	if sq.PlayerID != playerID {
		return nil, httputil.NewForbidden("not_owner", "this squad does not belong to you")
	}
	if sq.Status != "idle" {
		return nil, httputil.NewBadRequest("squad_busy", "squad is already on a mission")
	}

	// Validate mission type. attack_rift missions are set only by the battle
	// flow (squad_provider), never via this endpoint — accepting them here let
	// players park squads on no-op "missions" the worker then paid for.
	switch missionType {
	case "patrol":
		if err := s.validatePatrol(ctx, playerID, targetID); err != nil {
			return nil, err
		}
	case "capture":
		// Tower capture is v1.1 (FeatureTowerPressure); until then a "capture"
		// mission validated nothing and was just a bigger free payout.
		if !canon.IsFeatureAvailable(canon.FeatureTowerPressure) {
			return nil, httputil.NewBadRequest("feature_locked", "capture missions unlock in v1.1")
		}
	default:
		return nil, httputil.NewBadRequest("invalid_mission", "mission type must be patrol or capture")
	}

	// Dispatch cost (refundless) — a mission is a sortie, not a faucet.
	if s.spender != nil {
		if _, err := s.spender.Spend(ctx, playerID, canon.SquadMissionCostEnergy, 0); err != nil {
			return nil, err
		}
	}

	now := time.Now()
	sq.Status = "on_mission"
	sq.Mission = &Mission{
		Type:      missionType,
		TargetID:  targetID,
		StartedAt: now,
		ReturnsAt: now.Add(canon.SquadMissionDuration),
	}

	if err := s.squads.Update(ctx, sq); err != nil {
		return nil, httputil.NewInternal("failed to update squad")
	}

	return sq, nil
}

// validatePatrol requires a real target cell within field range of the
// player's server-side position — patrol used to accept any cellID anywhere
// on the planet (remote infection cleanse + free reward). Nil deps (tests)
// skip the check.
func (s *Service) validatePatrol(ctx context.Context, playerID, targetID string) error {
	if targetID == "" {
		return httputil.NewBadRequest("invalid_target", "patrol requires a target cell")
	}
	if s.cells == nil || s.positions == nil {
		return nil
	}
	c, err := s.cells.GetByID(ctx, targetID)
	if err != nil || c == nil {
		return httputil.NewBadRequest("invalid_target", "target cell not found")
	}
	p, err := s.positions.GetByID(ctx, playerID)
	if err != nil || p == nil {
		return httputil.NewInternal("failed to load player position")
	}
	if geo.Haversine(p.Position.Lat, p.Position.Lng, c.Lat, c.Lng) > canon.SquadMissionRangeM {
		return httputil.NewBadRequest("too_far", "patrol target is out of range — get closer")
	}
	return nil
}

// Return brings a squad back from a mission, resetting its status.
func (s *Service) Return(ctx context.Context, squadID, playerID string) (*Squad, error) {
	sq, err := s.squads.GetByID(ctx, squadID)
	if err != nil {
		return nil, httputil.NewNotFound("squad_not_found", "squad not found")
	}
	if sq.PlayerID != playerID {
		return nil, httputil.NewForbidden("not_owner", "this squad does not belong to you")
	}
	if sq.Status != "on_mission" {
		return nil, httputil.NewBadRequest("squad_not_on_mission", "squad is not on a mission")
	}

	sq.Status = "idle"
	sq.Mission = nil

	if err := s.squads.Update(ctx, sq); err != nil {
		return nil, httputil.NewInternal("failed to update squad")
	}
	sq.UnitCount = s.countSquadUnits(ctx, playerID, sq.ID)
	sq.FighterCount = s.countSquadFighters(ctx, playerID, sq.ID)

	return sq, nil
}

// Disband deletes an idle squad and frees its units back to the idle pool.
// A squad on a mission must be recalled first.
func (s *Service) Disband(ctx context.Context, squadID, playerID string) error {
	sq, err := s.squads.GetByID(ctx, squadID)
	if err != nil {
		return httputil.NewNotFound("squad_not_found", "squad not found")
	}
	if sq.PlayerID != playerID {
		return httputil.NewForbidden("not_owner", "this squad does not belong to you")
	}
	if sq.Status != "idle" {
		return httputil.NewBadRequest("squad_busy", "recall the squad before disbanding it")
	}

	// Free every member back to the unassigned pool.
	units, err := s.units.GetByPlayer(ctx, playerID)
	if err != nil {
		return httputil.NewInternal("failed to load units")
	}
	for _, u := range units {
		if u.SquadID != nil && *u.SquadID == squadID {
			if err := s.units.AssignSquad(ctx, u.ID, nil); err != nil {
				return httputil.NewInternal("failed to free unit")
			}
		}
	}

	if err := s.squads.Delete(ctx, squadID); err != nil {
		return httputil.NewInternal("failed to disband squad")
	}
	return nil
}

// UpdateComposition renames an idle squad and/or reconciles its membership to
// the exact set of unitIDs (units already in this squad stay; new ones must be
// the player's idle, unassigned units; removed ones return to the pool).
func (s *Service) UpdateComposition(ctx context.Context, squadID, playerID, name string, unitIDs []string) (*Squad, error) {
	sq, err := s.squads.GetByID(ctx, squadID)
	if err != nil {
		return nil, httputil.NewNotFound("squad_not_found", "squad not found")
	}
	if sq.PlayerID != playerID {
		return nil, httputil.NewForbidden("not_owner", "this squad does not belong to you")
	}
	if sq.Status != "idle" {
		return nil, httputil.NewBadRequest("squad_busy", "recall the squad before editing it")
	}
	if len(unitIDs) == 0 {
		return nil, httputil.NewBadRequest("no_units", "a squad needs at least one unit; disband it instead")
	}

	units, err := s.units.GetByPlayer(ctx, playerID)
	if err != nil {
		return nil, httputil.NewInternal("failed to load units")
	}
	byID := make(map[string]*unit.Unit, len(units))
	for i := range units {
		byID[units[i].ID] = &units[i]
	}

	target := make(map[string]bool, len(unitIDs))
	fighterCount := 0
	for _, uid := range unitIDs {
		u, ok := byID[uid]
		if !ok {
			return nil, httputil.NewNotFound("unit_not_found", "unit "+uid+" not found")
		}
		inThisSquad := u.SquadID != nil && *u.SquadID == squadID
		if u.SquadID != nil && !inThisSquad {
			return nil, httputil.NewBadRequest("unit_assigned", "unit "+uid+" is already in another squad")
		}
		if !inThisSquad && u.Status != "idle" {
			return nil, httputil.NewBadRequest("unit_busy", "unit "+uid+" is not idle")
		}
		target[uid] = true
		if u.Type == "fighter" {
			fighterCount++
		}
	}

	// Remove members no longer in the target set.
	for _, u := range units {
		if u.SquadID != nil && *u.SquadID == squadID && !target[u.ID] {
			if err := s.units.AssignSquad(ctx, u.ID, nil); err != nil {
				return nil, httputil.NewInternal("failed to free unit")
			}
		}
	}
	// Add new members.
	for uid := range target {
		u := byID[uid]
		if u.SquadID == nil {
			if err := s.units.AssignSquad(ctx, uid, &squadID); err != nil {
				return nil, httputil.NewInternal("failed to assign unit")
			}
		}
	}

	if name != "" && name != sq.Name {
		sq.Name = name
		if err := s.squads.Update(ctx, sq); err != nil {
			return nil, httputil.NewInternal("failed to rename squad")
		}
	}

	sq.UnitCount = len(unitIDs)
	sq.FighterCount = fighterCount
	return sq, nil
}

// CompleteDueMissions finishes every timed mission whose ReturnsAt has passed:
// grants the reward, returns the squad to idle, and pushes a notification.
// Driven by the squad:complete_missions worker. Errors on one squad are logged
// and skipped so the rest still complete.
func (s *Service) CompleteDueMissions(ctx context.Context) error {
	due, err := s.squads.GetDueMissions(ctx, time.Now())
	if err != nil {
		return fmt.Errorf("get due missions: %w", err)
	}
	completed := 0
	for i := range due {
		if err := s.completeMission(ctx, &due[i]); err != nil {
			slog.Warn("complete mission failed", "squad_id", due[i].ID, "error", err)
			continue
		}
		completed++
	}
	if completed > 0 {
		slog.Info("squad missions completed", "count", completed)
	}
	return nil
}

func (s *Service) completeMission(ctx context.Context, sq *Squad) error {
	missionType := ""
	if sq.Mission != nil {
		missionType = sq.Mission.Type
	}

	// Only real away missions pay on return. attack_rift squads are released by
	// this worker too (battle marks them busy), but their loot comes from the
	// battle itself — the old default-to-patrol payout meant an abandoned
	// battle earned more than it cost to enter.
	energy, materials := 0, 0
	switch missionType {
	case "patrol":
		energy, materials = canon.SquadPatrolEnergy, canon.SquadPatrolMaterials
		// Patrol secures the target area: cap infection in a radius around it.
		if s.cells != nil && sq.Mission != nil && sq.Mission.TargetID != "" {
			if c, err := s.cells.GetByID(ctx, sq.Mission.TargetID); err == nil && c != nil {
				_, _ = s.cells.ClearInfectionInRadius(ctx, c.Lat, c.Lng,
					canon.SquadPatrolCleanseRadiusM, canon.SquadPatrolCleanseTarget)
			}
		}
	case "capture":
		energy, materials = canon.SquadCaptureEnergy, canon.SquadCaptureMaterials
	}

	if s.resources != nil && (energy > 0 || materials > 0) {
		if _, err := s.resources.Add(ctx, sq.PlayerID, energy, materials); err != nil {
			return fmt.Errorf("grant reward: %w", err)
		}
	}

	sq.Status = "idle"
	sq.Mission = nil
	if err := s.squads.Update(ctx, sq); err != nil {
		return fmt.Errorf("update squad: %w", err)
	}

	if s.push != nil {
		_ = s.push.NotifySquadReturned(ctx, sq.PlayerID, missionType)
	}
	return nil
}

func (s *Service) countSquadUnits(ctx context.Context, playerID, squadID string) int {
	units, err := s.units.GetByPlayer(ctx, playerID)
	if err != nil {
		return 0
	}
	count := 0
	for _, u := range units {
		if u.SquadID != nil && *u.SquadID == squadID {
			count++
		}
	}
	return count
}

func (s *Service) countSquadFighters(ctx context.Context, playerID, squadID string) int {
	units, err := s.units.GetByPlayer(ctx, playerID)
	if err != nil {
		return 0
	}
	count := 0
	for _, u := range units {
		if u.SquadID != nil && *u.SquadID == squadID && u.Type == "fighter" {
			count++
		}
	}
	return count
}
