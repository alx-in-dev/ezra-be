package tower

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/ezra-game/server/internal/canon"
	"github.com/ezra-game/server/internal/cell"
	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/pkg/geo"
	"github.com/ezra-game/server/pkg/httputil"
)

// Service implements tower business logic.
type Service struct {
	towers    Repository
	cells     cell.Repository
	players   player.Repository
	resources *player.ResourceService
	quests    interface {
		UpdateProgress(ctx context.Context, playerID, eventType string) error
	}
	push interface {
		NotifyTowerPlaced(ctx context.Context, playerID string) error
		NotifyTowerUnderAttack(ctx context.Context, defenderID, attackerName string) error
	}
	// network is the optional beacon-network hook (auto-link on place, cascade
	// on remove). Wired via WithNetwork; nil-safe so existing tests are unaffected.
	network NetworkHook
	// stations is the optional power-plant capacity contribution (T-777).
	stations StationCapacity
	// events is the optional realtime publisher (R4-6 live-feedback). nil-safe.
	events EventPublisher
	// allies resolves City-Link alliances (R4-4b mutual support). nil-safe.
	allies AllyResolver
}

// EventPublisher pushes a realtime event to a player's SSE stream (R4-6).
// Implemented by realtime.Hub.PublishEvent.
type EventPublisher interface {
	PublishEvent(playerID, eventType string, data map[string]any)
}

// WithEvents wires the realtime publisher (live defense feedback). Optional.
func (s *Service) WithEvents(p EventPublisher) *Service { s.events = p; return s }

// AllyResolver reports City-Link alliances (R4-4b mutual support): who may repair
// a beacon they don't own, and who to fan defense alerts out to. Implemented by
// citylink.Service.
type AllyResolver interface {
	AreLinked(ctx context.Context, a, b string) (bool, error)
	LinkedPartnerIDs(ctx context.Context, playerID string) ([]string, error)
}

// WithAllies wires City-Link mutual support (ally repair + ally alert fan-out).
// Optional; nil → no cross-player support.
func (s *Service) WithAllies(a AllyResolver) *Service { s.allies = a; return s }

// NetworkHook lets the tower service notify the beacon network of placements
// and removals without a hard dependency on the network package. The network
// derives links/dome from node positions, so place/remove both reduce to a
// recompute (Perimeter model).
type NetworkHook interface {
	Recompute(ctx context.Context, playerID string) error
	CanConnect(ctx context.Context, playerID string, lat, lng float64) (bool, bool, error)
	NearestCellID(ctx context.Context, lat, lng float64) (string, error)
}

// PlaceAtPlayer places a beacon at the player's exact current position
// (server-authoritative free placement, no grid snapping). The nearest cell
// becomes the beacon's anchor to the infection grid. Requires the network
// hook, which is always wired in production.
func (s *Service) PlaceAtPlayer(ctx context.Context, playerID string) (*Tower, error) {
	if s.network == nil {
		return nil, httputil.NewInternal("placement unavailable")
	}
	p, err := s.players.GetByID(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("get player: %w", err)
	}
	cellID, err := s.network.NearestCellID(ctx, p.Position.Lat, p.Position.Lng)
	if err != nil {
		return nil, fmt.Errorf("nearest cell: %w", err)
	}
	if cellID == "" {
		return nil, httputil.NewBadRequest("no_cell", "рядом нет клетки")
	}
	c, err := s.cells.GetByID(ctx, cellID)
	if err != nil {
		return nil, httputil.NewNotFound("not_found", "cell not found")
	}
	// The anchor cell must be reasonably close — far away means the spot is
	// outside the seeded world and its terrain flags say nothing about it.
	if d := geo.Haversine(p.Position.Lat, p.Position.Lng, c.Lat, c.Lng); d > canon.TowerAnchorMaxDistanceM {
		return nil, httputil.NewBadRequest("no_cell", "слишком далеко от размеченной зоны")
	}
	slog.Info("beacon placement point (server-side player position)",
		"player_id", playerID,
		"lat", p.Position.Lat, "lng", p.Position.Lng,
		"position_age_s", time.Since(p.Position.UpdatedAt).Seconds())
	return s.place(ctx, p, p.Position.Lat, p.Position.Lng, c)
}

// WithNetwork wires the beacon-network hook and returns the service.
func (s *Service) WithNetwork(n NetworkHook) *Service {
	s.network = n
	return s
}

// NewService creates a new tower service.
func NewService(towers Repository, cells cell.Repository, players player.Repository, resources *player.ResourceService, quests interface {
	UpdateProgress(ctx context.Context, playerID, eventType string) error
}, push interface {
	NotifyTowerPlaced(ctx context.Context, playerID string) error
	NotifyTowerUnderAttack(ctx context.Context, defenderID, attackerName string) error
}) *Service {
	return &Service{towers: towers, cells: cells, players: players, resources: resources, quests: quests, push: push}
}

// Place creates a new tower at the centre of the given cell (legacy grid
// path: explicit cell selection by tap). Free placement is PlaceAtPlayer.
func (s *Service) Place(ctx context.Context, playerID, cellID string) (*Tower, error) {
	p, err := s.players.GetByID(ctx, playerID)
	if err != nil {
		slog.Error("tower place failed: get player", "player_id", playerID, "cell_id", cellID, "error", err)
		return nil, fmt.Errorf("get player: %w", err)
	}

	c, err := s.cells.GetByID(ctx, cellID)
	if err != nil {
		slog.Warn("tower place rejected: cell not found", "player_id", playerID, "cell_id", cellID, "error", err)
		return nil, httputil.NewNotFound("not_found", "cell not found")
	}

	// Validate distance < 30m
	dist := geo.Haversine(p.Position.Lat, p.Position.Lng, c.Lat, c.Lng)
	if dist > canon.TowerPlacementMaxDistanceM {
		slog.Warn("tower place rejected: too far",
			"player_id", playerID,
			"cell_id", cellID,
			"distance_m", dist,
			"max_distance_m", canon.TowerPlacementMaxDistanceM,
			"player_lat", p.Position.Lat,
			"player_lng", p.Position.Lng,
			"cell_lat", c.Lat,
			"cell_lng", c.Lng)
		return nil, httputil.NewBadRequest("too_far", "must be within 30m of cell")
	}

	return s.place(ctx, p, c.Lat, c.Lng, c)
}

// place creates a tower at the exact point (lat,lng), anchored to cell c for
// infection bookkeeping. All placement gates run against the point itself.
// buildingCounter is optionally implemented by the cell repository: counts
// building-flagged cells in a radius (OSM grid-density proxy for capacity).
type buildingCounter interface {
	CountBuildingCellsInRadius(ctx context.Context, lat, lng, radiusM float64) (int, error)
}

// StationCapacity sums power-plant capacity bonus in a radius (T-777). Optional
// dep wired via WithStations; nil → no station bonus.
type StationCapacity interface {
	StationBonusInRadius(ctx context.Context, lat, lng, radiusM float64) (int, error)
}

// WithStations wires the power-plant capacity contribution (energy pillar T-777).
func (s *Service) WithStations(sc StationCapacity) { s.stations = sc }

// areaCapacity returns how many beacons the local grid powers within the
// overload radius of (lat,lng): OSM building density + any power-plant bonus.
// Falls back to the flat legacy cap when the cell repo can't count (unit tests).
// Call only on rare paths — it runs spatial count queries.
func (s *Service) areaCapacity(ctx context.Context, lat, lng float64) int {
	bc, ok := s.cells.(buildingCounter)
	if !ok {
		return canon.TowerOverloadMaxCount
	}
	cells, err := bc.CountBuildingCellsInRadius(ctx, lat, lng, canon.TowerOverloadRadiusM)
	if err != nil {
		return canon.TowerOverloadMaxCount
	}
	stationBonus := 0
	if s.stations != nil {
		if b, err := s.stations.StationBonusInRadius(ctx, lat, lng, canon.TowerOverloadRadiusM); err == nil {
			stationBonus = b
		}
	}
	return canon.BeaconAreaCapacityWith(cells, stationBonus)
}

func (s *Service) place(ctx context.Context, p *player.Player, lat, lng float64, c *cell.Cell) (*Tower, error) {
	playerID := p.ID
	cellID := c.ID

	// Canon (decision 8.2, zone_generation_analysis.md): towers require a
	// power source nearby. Cell is valid if there is at least one building
	// or road within 30m (flags populated at seed time from Mapbox Tilequery).
	// Lore: beacons need electricity from infrastructure power lines.
	if !c.HasNearbyBuilding && !c.HasNearbyRoad {
		slog.Warn("tower place rejected: no power source",
			"player_id", playerID,
			"cell_id", cellID,
			"terrain", c.Terrain)
		return nil, httputil.NewBadRequest("no_power_source", "no building or road within 30m; tower needs a power source")
	}

	// Validate no existing tower
	if c.TowerID != nil {
		slog.Warn("tower place rejected: cell occupied",
			"player_id", playerID,
			"cell_id", cellID,
			"existing_tower_id", *c.TowerID)
		return nil, httputil.NewBadRequest("cell_occupied", "cell already has a tower")
	}

	// Min spacing: free placement must not let beacons stack on one spot.
	tooClose, err := s.towers.CountAllInRadius(ctx, lat, lng, canon.NetworkMinSpacingM)
	if err != nil {
		return nil, fmt.Errorf("count towers in spacing radius: %w", err)
	}
	if tooClose > 0 {
		slog.Warn("tower place rejected: too close to another beacon",
			"player_id", playerID, "min_spacing_m", canon.NetworkMinSpacingM)
		return nil, httputil.NewBadRequest("too_close", "слишком близко к другому маяку")
	}

	cfg := LevelConfigs[1]
	isStarterBeacon := p.OnboardingStep == canon.OnboardingFirstTowerStep && p.StarterBeaconAvailable

	// Grid-capacity cap (energy pillar #7): the area holds at most as many
	// beacons (any owner) as the LOCAL electrical grid powers — derived from
	// OSM building density, not a flat number. Dense districts host more; sparse
	// areas fall to a floor. Existing beacons are never removed (grandfather);
	// this only blocks NEW placement once the area is at capacity. The onboarding
	// starter beacon is exempt so a crowded spawn can never softlock onboarding.
	if !isStarterBeacon {
		overloadCount, err := s.towers.CountAllInRadius(ctx, lat, lng, canon.TowerOverloadRadiusM)
		if err != nil {
			slog.Error("tower place failed: count nearby towers",
				"player_id", playerID, "cell_id", cellID, "lat", lat, "lng", lng, "error", err)
			return nil, fmt.Errorf("count towers in radius: %w", err)
		}
		capacity := s.areaCapacity(ctx, lat, lng)
		if overloadCount >= capacity {
			slog.Warn("tower place rejected: grid at capacity",
				"player_id", playerID,
				"cell_id", cellID,
				"nearby_towers", overloadCount,
				"capacity", capacity,
				"radius_m", canon.TowerOverloadRadiusM)
			return nil, httputil.NewBadRequest("overloaded",
				fmt.Sprintf("электросеть зоны заполнена (%d/%d маяков)", overloadCount, capacity))
		}
	}

	// Connectivity gate (Perimeter port): once the player has a Core, a new
	// beacon must OVERLAP the powered energy field (its disk touches a connected
	// node's disk). The onboarding starter beacon is exempt. No network yet
	// (no Core) → unrestricted. See docs/feature/beacon_network_dome.md.
	if s.network != nil && !isStarterBeacon {
		canConnect, hasNetwork, nerr := s.network.CanConnect(ctx, playerID, lat, lng)
		if nerr != nil {
			slog.Warn("tower place: connectivity check failed", "player_id", playerID, "error", nerr)
		} else if hasNetwork && !canConnect {
			slog.Warn("tower place rejected: outside energy field",
				"player_id", playerID, "cell_id", cellID)
			return nil, httputil.NewBadRequest("no_link", "слишком далеко от сети — встань ближе к энергополю")
		}
	}

	// Spend resources
	if !isStarterBeacon {
		_, err = s.resources.Spend(ctx, playerID, cfg.EnergyCost, cfg.MaterialCost)
		if err != nil {
			if appErr, ok := err.(*httputil.AppError); ok {
				slog.Warn("tower place rejected: resource spend failed",
					"player_id", playerID,
					"cell_id", cellID,
					"error_code", appErr.Code,
					"error_message", appErr.Message,
					"energy_cost", cfg.EnergyCost,
					"material_cost", cfg.MaterialCost,
					"player_energy", p.Energy,
					"player_materials", p.Materials)
			} else {
				slog.Error("tower place failed: resource spend failed",
					"player_id", playerID,
					"cell_id", cellID,
					"energy_cost", cfg.EnergyCost,
					"material_cost", cfg.MaterialCost,
					"error", err)
			}
			return nil, err
		}
	}

	// Create tower
	t := &Tower{
		CellID:        cellID,
		Lat:           lat,
		Lng:           lng,
		OwnerID:       playerID,
		Level:         1,
		Type:          canon.TowerTypeStandard,
		HP:            cfg.HPMax,
		HPMax:         cfg.HPMax,
		RadiusM:       cfg.RadiusM,
		EffectPerHour: cfg.EffectPerHour,
	}

	if err := s.towers.Create(ctx, t); err != nil {
		slog.Error("tower place failed: create tower",
			"player_id", playerID,
			"cell_id", cellID,
			"error", err)
		return nil, fmt.Errorf("create tower: %w", err)
	}

	// Link tower to cell
	if err := s.cells.SetTowerID(ctx, cellID, &t.ID); err != nil {
		slog.Error("tower place failed: link tower to cell",
			"player_id", playerID,
			"cell_id", cellID,
			"tower_id", t.ID,
			"error", err)
		return nil, fmt.Errorf("link tower to cell: %w", err)
	}
	if isStarterBeacon || p.OnboardingStep == canon.OnboardingFirstTowerStep {
		p.StarterBeaconAvailable = false
		p.OnboardingStep = canon.OnboardingSurvivorStep
		if err := s.players.Update(ctx, p); err != nil {
			slog.Error("tower place failed: update onboarding after placement",
				"player_id", playerID,
				"cell_id", cellID,
				"tower_id", t.ID,
				"error", err)
			return nil, fmt.Errorf("update onboarding after tower placement: %w", err)
		}
	}
	if s.quests != nil {
		_ = s.quests.UpdateProgress(ctx, playerID, "place_beacon")
	}
	if s.push != nil {
		_ = s.push.NotifyTowerPlaced(ctx, playerID)
	}
	// Re-derive the player's network with the new beacon (non-fatal).
	if s.network != nil {
		if err := s.network.Recompute(ctx, playerID); err != nil {
			slog.Warn("network recompute on beacon place failed",
				"player_id", playerID, "tower_id", t.ID, "error", err)
		}
	}

	slog.Info("tower placed",
		"player_id", playerID,
		"cell_id", cellID,
		"tower_id", t.ID,
		"starter_beacon", isStarterBeacon,
		"lat", lat,
		"lng", lng,
		"terrain", c.Terrain)

	return t, nil
}

// Upgrade advances a tower to the next level.
func (s *Service) Upgrade(ctx context.Context, towerID, playerID string) (*Tower, error) {
	t, err := s.towers.GetByID(ctx, towerID)
	if err != nil {
		return nil, httputil.NewNotFound("not_found", "tower not found")
	}

	if t.OwnerID != playerID {
		return nil, httputil.NewForbidden("not_owner", "not your tower")
	}

	nextLevel := t.Level + 1
	if !canon.IsTowerLevelAvailable(nextLevel) {
		return nil, httputil.NewBadRequest("feature_locked", "tower level is not available in the current release")
	}
	cfg, ok := LevelConfigs[nextLevel]
	if !ok {
		return nil, httputil.NewBadRequest("max_level", "tower is already max level")
	}

	// Canon: 1->2 can be initiated remotely, 2+ requires field presence.
	if nextLevel >= 3 {
		p, err := s.players.GetByID(ctx, playerID)
		if err != nil {
			return nil, fmt.Errorf("get player: %w", err)
		}

		c, err := s.cells.GetByID(ctx, t.CellID)
		if err != nil {
			return nil, err
		}

		dist := geo.Haversine(p.Position.Lat, p.Position.Lng, c.Lat, c.Lng)
		if dist > canon.TowerPlacementMaxDistanceM {
			return nil, httputil.NewBadRequest("too_far", "must be within 30m of tower for this upgrade")
		}

		// L3 is the last MVP level. Canon requires "1 allied beacon nearby";
		// with no factions in MVP, "allied" maps to the player's own beacons.
		// CountInRadius counts this player's towers within the radius and
		// includes the one being upgraded (distance 0), so count must be >= 2
		// (the upgrading tower + at least one more of the player's beacons).
		if nextLevel != 3 {
			goto spendResources
		}

		count, err := s.towers.CountInRadius(ctx, c.Lat, c.Lng, canon.TowerL3NearbyRadiusM, playerID)
		if err != nil {
			return nil, err
		}
		if count < 2 {
			return nil, httputil.NewBadRequest("allies_required", "need 1 more of your beacons within 200m for level 3")
		}
	}

spendResources:
	// Spend resources
	_, err = s.resources.Spend(ctx, playerID, cfg.EnergyCost, cfg.MaterialCost)
	if err != nil {
		return nil, err
	}

	// Apply upgrade
	t.Level = nextLevel
	t.RadiusM = cfg.RadiusM
	t.EffectPerHour = cfg.EffectPerHour
	t.HPMax = cfg.HPMax
	t.HP = cfg.HPMax

	if err := s.towers.Update(ctx, t); err != nil {
		return nil, err
	}

	return t, nil
}

// Remove deletes a tower and returns partial resources to the player.
func (s *Service) Remove(ctx context.Context, towerID, playerID string) (int, int, error) {
	t, err := s.towers.GetByID(ctx, towerID)
	if err != nil {
		return 0, 0, httputil.NewNotFound("not_found", "tower not found")
	}
	if t.OwnerID != playerID {
		return 0, 0, httputil.NewForbidden("not_owner", "not your tower")
	}

	// Unlink from cell
	if err := s.cells.SetTowerID(ctx, t.CellID, nil); err != nil {
		return 0, 0, fmt.Errorf("unlink tower from cell: %w", err)
	}

	// Delete tower
	if err := s.towers.Delete(ctx, towerID); err != nil {
		return 0, 0, err
	}

	// Recompute the network without the beacon (cascade collapse).
	if s.network != nil {
		if err := s.network.Recompute(ctx, t.OwnerID); err != nil {
			slog.Warn("network recompute on beacon remove failed",
				"tower_id", towerID, "error", err)
		}
	}

	// Return 50% of level 1 cost
	returnEnergy := LevelConfigs[1].EnergyCost / 2
	returnMaterials := LevelConfigs[1].MaterialCost / 2
	s.resources.Add(ctx, playerID, returnEnergy, returnMaterials)

	return returnEnergy, returnMaterials, nil
}

// ApplyPressure grinds beacon HP by the infection of the cell each beacon sits
// on (PvE "давление на сеть"). A beacon at 0 HP is destroyed, which unlinks its
// cell and triggers a network recompute (cascade collapse). Domed beacons sit
// in suppressed cells and take no damage — neglect is what eats your network.
// Runs hourly; canon damage values are per-hour. See beacon_network_dome.md §8.
func (s *Service) ApplyPressure(ctx context.Context) error {
	towers, err := s.towers.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("pressure: get towers: %w", err)
	}

	destroyed := 0
	for i := range towers {
		t := &towers[i]
		c, err := s.cells.GetByID(ctx, t.CellID)
		if err != nil || c == nil {
			continue
		}
		dmg := int(math.Round(canon.BeaconPressureDamagePerHour(c.Infection)))
		if dmg <= 0 {
			continue
		}

		prevHP := t.HP
		t.HP -= dmg
		if t.HP > 0 {
			// Defense race: alert the owner the first tick the beacon crosses
			// into the low-HP band, so they can run to repair before it falls.
			warnAt := int(math.Ceil(float64(t.HPMax) * canon.BeaconLowHPWarnFraction))
			if prevHP > warnAt && t.HP <= warnAt {
				if s.push != nil {
					_ = s.push.NotifyTowerUnderAttack(ctx, t.OwnerID, "Заражение")
				}
				// Defense race (R4-6): rich realtime event so the client can
				// point the player at the threatened beacon with a countdown.
				// Pressure damage is per-hour, so it yields a real ETA-to-zero.
				s.publishUnderAttack(ctx, t, dmg, "infection")
			}
			if err := s.towers.Update(ctx, t); err != nil {
				slog.Warn("pressure: update tower hp failed", "tower_id", t.ID, "error", err)
			}
			continue
		}

		// Destroyed by infection — unlink, delete, cascade.
		s.destroyTower(ctx, t, "infection")
		destroyed++
	}

	slog.Info("beacon pressure tick", "towers", len(towers), "destroyed", destroyed)
	return nil
}

// destroyTower unlinks a fallen beacon's cell, deletes it, and cascades a
// network recompute for its owner. Shared by infection pressure (ApplyPressure)
// and symbiont corrosion (Corrode). cause labels what felled it ("infection" /
// "symbiont") for the live-feedback event (R4-6).
func (s *Service) destroyTower(ctx context.Context, t *Tower, cause string) {
	if err := s.cells.SetTowerID(ctx, t.CellID, nil); err != nil {
		slog.Warn("destroy: unlink tower failed", "tower_id", t.ID, "error", err)
	}
	if err := s.towers.Delete(ctx, t.ID); err != nil {
		slog.Warn("destroy: delete tower failed", "tower_id", t.ID, "error", err)
		return
	}
	// R4-6: tell the owner live that a beacon just fell (the defense race's
	// worst beat — no longer silent). R4-4b: also alert City-Linked allies so
	// they can come help rebuild.
	if s.events != nil && t.OwnerID != "" {
		data := map[string]any{"tower_id": t.ID, "lat": t.Lat, "lng": t.Lng, "cause": cause}
		s.events.PublishEvent(t.OwnerID, "tower_destroyed", data)
		if s.allies != nil {
			if partners, err := s.allies.LinkedPartnerIDs(ctx, t.OwnerID); err == nil {
				for _, pid := range partners {
					s.events.PublishEvent(pid, "ally_tower_destroyed", data)
				}
			}
		}
	}
	if s.network != nil {
		if err := s.network.Recompute(ctx, t.OwnerID); err != nil {
			slog.Warn("destroy: network cascade failed", "tower_id", t.ID, "error", err)
		}
	}
}

// publishUnderAttack emits a rich realtime "tower_under_attack" event so the
// client can point the player at the threatened beacon and show a countdown.
// Mirrors the destroyed fan-out: owner + City-Linked allies (SSE only — no FCM
// spam to allies). dmgPerHour > 0 (infection pressure) yields eta_seconds to
// zero HP; event-driven hits (symbiont corrosion) pass 0 → eta_seconds omitted.
// nil-safe: no realtime publisher or no owner → no-op.
func (s *Service) publishUnderAttack(ctx context.Context, t *Tower, dmgPerHour int, cause string) {
	if s.events == nil || t == nil || t.OwnerID == "" {
		return
	}
	data := map[string]any{
		"tower_id": t.ID, "lat": t.Lat, "lng": t.Lng,
		"hp": t.HP, "hp_max": t.HPMax, "cause": cause,
	}
	if dmgPerHour > 0 && t.HP > 0 {
		data["eta_seconds"] = int(math.Round(float64(t.HP) / float64(dmgPerHour) * 3600))
	}
	s.events.PublishEvent(t.OwnerID, "tower_under_attack", data)
	if s.allies != nil {
		if partners, err := s.allies.LinkedPartnerIDs(ctx, t.OwnerID); err == nil {
			for _, pid := range partners {
				s.events.PublishEvent(pid, "ally_tower_under_attack", data)
			}
		}
	}
}

// CorrodeResult reports the outcome of a Symbiont overload/corrosion strike.
type CorrodeResult struct {
	Found       bool   `json:"found"`
	TowerID     string `json:"tower_id,omitempty"`
	RemainingHP int    `json:"remaining_hp"`
	HPMax       int    `json:"hp_max"`
	Destroyed   bool   `json:"destroyed"`
}

// Corrode is the tower-side effect of the Symbiont "Перегрузить маяк" verb:
// damage the nearest hostile (not attacker-owned) beacon within radius, and
// destroy it at 0 HP via the same cascade as infection pressure. This attacks
// enemy *infrastructure*, not players, so it stays within the no-direct-PvP
// rule (symbiont_geo_playstyle.md §2.3). Returns Found=false if no hostile
// beacon is in range.
func (s *Service) Corrode(ctx context.Context, lat, lng float64, attackerID string, damage, radiusM int) (*CorrodeResult, error) {
	towers, err := s.towers.GetInRadius(ctx, lat, lng, float64(radiusM))
	if err != nil {
		return nil, fmt.Errorf("corrode: towers in radius: %w", err)
	}
	var target *Tower
	best := math.MaxFloat64
	for i := range towers {
		t := &towers[i]
		if t.OwnerID == attackerID {
			continue // never your own beacons
		}
		if d := geo.Haversine(lat, lng, t.Lat, t.Lng); d < best {
			best = d
			target = t
		}
	}
	if target == nil {
		return &CorrodeResult{Found: false}, nil
	}

	target.HP -= damage
	if target.HP > 0 {
		if err := s.towers.Update(ctx, target); err != nil {
			return nil, fmt.Errorf("corrode: update hp: %w", err)
		}
		if s.push != nil {
			_ = s.push.NotifyTowerUnderAttack(ctx, target.OwnerID, "Симбионт")
		}
		s.publishUnderAttack(ctx, target, 0, "symbiont")
		return &CorrodeResult{Found: true, TowerID: target.ID, RemainingHP: target.HP, HPMax: target.HPMax}, nil
	}
	s.destroyTower(ctx, target, "symbiont")
	return &CorrodeResult{Found: true, TowerID: target.ID, RemainingHP: 0, HPMax: target.HPMax, Destroyed: true}, nil
}

// CorrodeTower is the autonomous-entity variant of Corrode (R4-2 L2): damage a
// SPECIFIC hostile beacon by id (the entity's standing target), destroying it at
// 0 HP via the shared cascade. Returns Found=false when the target is gone
// (destroyed/missing) or no longer hostile — the caller then disengages the
// entity. Like Corrode it hits infrastructure, never players (no-PvP rule).
func (s *Service) CorrodeTower(ctx context.Context, towerID, attackerID string, damage int) (*CorrodeResult, error) {
	t, err := s.towers.GetByID(ctx, towerID)
	if err != nil || t == nil {
		return &CorrodeResult{Found: false}, nil // target gone
	}
	if t.OwnerID == attackerID {
		return &CorrodeResult{Found: false}, nil // not hostile (anymore)
	}
	t.HP -= damage
	if t.HP > 0 {
		if err := s.towers.Update(ctx, t); err != nil {
			return nil, fmt.Errorf("corrode tower: update hp: %w", err)
		}
		if s.push != nil {
			_ = s.push.NotifyTowerUnderAttack(ctx, t.OwnerID, "Симбионт")
		}
		s.publishUnderAttack(ctx, t, 0, "symbiont")
		return &CorrodeResult{Found: true, TowerID: t.ID, RemainingHP: t.HP, HPMax: t.HPMax}, nil
	}
	s.destroyTower(ctx, t, "symbiont")
	return &CorrodeResult{Found: true, TowerID: t.ID, RemainingHP: 0, HPMax: t.HPMax, Destroyed: true}, nil
}

// WeakestResult reports the most vulnerable hostile beacon found by a recon scan.
type WeakestResult struct {
	Found     bool   `json:"found"`
	TowerID   string `json:"tower_id,omitempty"`
	DistanceM int    `json:"distance_m"`
	HP        int    `json:"hp"`
	HPMax     int    `json:"hp_max"`
}

// WeakestHostile is the tower-side effect of the Symbiont "Разведать слабое
// место сети" verb: among hostile (not attacker-owned) beacons within radius,
// return the one closest to collapse (lowest HP fraction, nearest as tiebreak).
// Read-only intel — no mutation.
func (s *Service) WeakestHostile(ctx context.Context, lat, lng float64, attackerID string, radiusM int) (*WeakestResult, error) {
	towers, err := s.towers.GetInRadius(ctx, lat, lng, float64(radiusM))
	if err != nil {
		return nil, fmt.Errorf("recon: towers in radius: %w", err)
	}
	var target *Tower
	bestFrac, bestDist := math.MaxFloat64, math.MaxFloat64
	for i := range towers {
		t := &towers[i]
		if t.OwnerID == attackerID {
			continue
		}
		frac := 1.0
		if t.HPMax > 0 {
			frac = float64(t.HP) / float64(t.HPMax)
		}
		d := geo.Haversine(lat, lng, t.Lat, t.Lng)
		if frac < bestFrac || (frac == bestFrac && d < bestDist) {
			bestFrac, bestDist = frac, d
			target = t
		}
	}
	if target == nil {
		return &WeakestResult{Found: false}, nil
	}
	return &WeakestResult{Found: true, TowerID: target.ID, DistanceM: int(bestDist), HP: target.HP, HPMax: target.HPMax}, nil
}

// Repair restores a damaged beacon to full HP for energy poured in on site —
// the player's half of the defense race (beacon_network_dome.md §8). Requires
// physical presence (<40m) and ownership; cost scales with the HP restored.
func (s *Service) Repair(ctx context.Context, towerID, playerID string) (*Tower, error) {
	t, err := s.towers.GetByID(ctx, towerID)
	if err != nil {
		return nil, httputil.NewNotFound("not_found", "tower not found")
	}
	// R4-4b mutual support: a City-Linked ally may repair a beacon they don't own
	// (presence is still required below). Otherwise it must be your own.
	if t.OwnerID != playerID {
		allowed := false
		if s.allies != nil {
			if linked, lErr := s.allies.AreLinked(ctx, playerID, t.OwnerID); lErr == nil && linked {
				allowed = true
			}
		}
		if !allowed {
			return nil, httputil.NewForbidden("not_owner", "это не ваш маяк")
		}
	}
	if t.HP >= t.HPMax {
		return nil, httputil.NewBadRequest("not_damaged", "beacon is already at full HP")
	}

	p, err := s.players.GetByID(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("get player: %w", err)
	}
	c, err := s.cells.GetByID(ctx, t.CellID)
	if err != nil {
		return nil, err
	}
	if d := geo.Haversine(p.Position.Lat, p.Position.Lng, c.Lat, c.Lng); d > canon.TowerPlacementMaxDistanceM {
		return nil, httputil.NewBadRequest("too_far", "must be within range of the beacon to repair")
	}

	missing := t.HPMax - t.HP
	cost := int(math.Ceil(float64(missing) / float64(canon.BeaconRepairHPPerEnergy)))
	if _, err := s.resources.Spend(ctx, playerID, cost, 0); err != nil {
		return nil, err
	}

	t.HP = t.HPMax
	if err := s.towers.Update(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}
