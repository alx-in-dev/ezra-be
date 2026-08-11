package pet

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"sort"
	"time"

	"github.com/ezra-game/server/internal/canon"
	"github.com/ezra-game/server/internal/cell"
	"github.com/ezra-game/server/internal/item"
	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/internal/survivor"
	"github.com/ezra-game/server/internal/tower"
	"github.com/ezra-game/server/pkg/geo"
	"github.com/ezra-game/server/pkg/httputil"
)

// Loot represents the reward from a pet task.
type Loot struct {
	Energy    int          `json:"energy"`
	Materials int          `json:"materials"`
	Rare      bool         `json:"rare"`
	Scout     *ScoutResult `json:"scout,omitempty"`
	// Evolved is set when this claim leveled the pet across an evolution stage
	// (D3); EvolvedTo carries the new stage's display name for the celebration.
	Evolved   bool   `json:"evolved,omitempty"`
	EvolvedTo string `json:"evolved_to,omitempty"`
	LeveledTo int    `json:"leveled_to,omitempty"`
	// Signature is the resonance spectrum dropped by this evolution (D1).
	Signature   string `json:"signature,omitempty"`
	SignatureRu string `json:"signature_ru,omitempty"`
}

// evolutionSpectrum deterministically picks a resonance spectrum for a pet
// evolution drop, varied by pet id and stage (no rand seed available, but the
// spectrum should differ across pets/stages).
func evolutionSpectrum(petID string, stage int) string {
	spectra := item.ResonanceSpectra
	if len(spectra) == 0 {
		return ""
	}
	sum := stage
	for _, c := range petID {
		sum += int(c)
	}
	return spectra[sum%len(spectra)]
}

// Service implements pet business logic.
type Service struct {
	pets      Repository
	towers    tower.Repository
	cells     cell.Repository
	players   player.Repository
	resources *player.ResourceService
	quests    interface {
		UpdateProgress(ctx context.Context, playerID, eventType string) error
	}
	push interface {
		NotifyPetReturned(ctx context.Context, playerID, taskType string) error
	}
	inventory ItemGranter
	// contactRift guarantees an open rift near the player when onboarding
	// advances to contact_step (soft-lock guard; optional, nil in tests).
	contactRift func(ctx context.Context, lat, lng float64) error
}

// SetContactRiftEnsurer wires the contact_step rift guarantee (optional).
func (s *Service) SetContactRiftEnsurer(fn func(ctx context.Context, lat, lng float64) error) {
	s.contactRift = fn
}

// ItemGranter lets an evolving pet drop a resonance signature (D1). Optional
// (set via SetItemGranter); nil in tests.
type ItemGranter interface {
	Grant(ctx context.Context, playerID, itemType, variant string, delta int) (int, error)
}

// SetItemGranter wires the optional resonance-signature drop on pet evolution.
func (s *Service) SetItemGranter(g ItemGranter) { s.inventory = g }

// NewService creates a new pet service.
func NewService(pets Repository, towers tower.Repository, cells cell.Repository, players player.Repository, resources *player.ResourceService, quests interface {
	UpdateProgress(ctx context.Context, playerID, eventType string) error
}, push interface {
	NotifyPetReturned(ctx context.Context, playerID, taskType string) error
}) *Service {
	return &Service{pets: pets, towers: towers, cells: cells, players: players, resources: resources, quests: quests, push: push}
}

// GetByPlayer returns all pets for a player.
func (s *Service) GetByPlayer(ctx context.Context, playerID string) ([]Pet, error) {
	pets, err := s.pets.GetByPlayer(ctx, playerID)
	if err != nil {
		return nil, err
	}
	for i := range pets {
		pets[i].Decorate()
	}
	return pets, nil
}

// ClaimStarter grants the onboarding scout pet and hands off to the Contact
// bridge (advances onboarding to contact_step, not completed).
func (s *Service) ClaimStarter(ctx context.Context, playerID string) (*Pet, *player.Player, error) {
	p, err := s.players.GetByID(ctx, playerID)
	if err != nil {
		return nil, nil, httputil.NewInternal("failed to get player")
	}
	if p.OnboardingStep != canon.OnboardingPetUnlockStep && p.OnboardingStep != canon.OnboardingCompleted {
		return nil, nil, httputil.NewBadRequest("feature_locked", "starter pet is not available right now")
	}

	pets, err := s.pets.GetByPlayer(ctx, playerID)
	if err != nil {
		return nil, nil, httputil.NewInternal("failed to get pets")
	}

	var starter *Pet
	if len(pets) > 0 {
		starter = &pets[0]
	} else {
		starter = &Pet{
			PlayerID: playerID,
			Name:     "Scout",
			Type:     "scout",
			Level:    1,
			XP:       0,
			Status:   canon.PetStateIdle,
		}
		if err := s.pets.Create(ctx, starter); err != nil {
			return nil, nil, httputil.NewInternal("failed to grant starter pet")
		}
	}

	// Claiming the starter drone no longer ENDS onboarding — it hands off to the
	// Contact bridge (close a rift → become a temporary symbiont). Only the
	// final faction choice sets `completed`. See docs/feature/symbiont_onboarding.md.
	if p.OnboardingStep == canon.OnboardingPetUnlockStep {
		p.OnboardingStep = canon.OnboardingContactStep
		if err := s.players.Update(ctx, p); err != nil {
			return nil, nil, httputil.NewInternal("failed to advance onboarding")
		}
		// Contact needs an open rift to close; areas without a hive may have
		// none — guarantee one nearby (best-effort, never fails the claim).
		if s.contactRift != nil {
			if err := s.contactRift(ctx, p.Position.Lat, p.Position.Lng); err != nil {
				slog.Error("contact rift guarantee failed", "player_id", playerID, "error", err)
			}
		}
	}

	if starter != nil {
		starter.Decorate()
	}
	return starter, p, nil
}

// Send dispatches a pet on a task.
func (s *Service) Send(ctx context.Context, petID, playerID, taskType string) (*Pet, error) {
	p, err := s.pets.GetByID(ctx, petID)
	if err != nil {
		return nil, httputil.NewNotFound("not_found", "pet not found")
	}
	if p.PlayerID != playerID {
		return nil, httputil.NewForbidden("not_owner", "not your pet")
	}
	if p.Status != canon.PetStateIdle {
		return nil, httputil.NewBadRequest("not_idle", "pet is already on a task")
	}
	if taskType == "" {
		taskType = TaskTypeSearch
	}
	if taskType != TaskTypeSearch && taskType != TaskTypeScout {
		return nil, httputil.NewBadRequest("invalid_task", "pet task must be search or scout")
	}

	duration, ok := SearchDuration[p.Level]
	if !ok {
		duration = SearchDuration[1]
	}

	now := time.Now()
	task := &PetTask{
		Type:      taskType,
		StartedAt: now,
		ReturnsAt: now.Add(time.Duration(duration) * time.Minute),
		LootRoll:  rand.Float64(),
	}
	if taskType == TaskTypeScout {
		targetTowerID, err := s.selectScoutTower(ctx, playerID)
		if err != nil {
			return nil, err
		}
		task.TargetTowerID = targetTowerID
	}

	p.Status = canon.PetStateMissionActive
	p.Task = task

	if err := s.pets.Update(ctx, p); err != nil {
		return nil, fmt.Errorf("update pet: %w", err)
	}
	if s.quests != nil {
		_ = s.quests.UpdateProgress(ctx, playerID, "send_pet")
	}
	p.Decorate()
	return p, nil
}

// Recall is the player-initiated action on a pet that is out on a task:
//   - if the mission's timer has already elapsed → claim the full reward now
//     (covers the gap before the auto-claim worker runs);
//   - if it has NOT elapsed → bring the pet back EARLY, forfeiting the reward
//     (e.g. to free it for urgent beacon defence).
func (s *Service) Recall(ctx context.Context, petID, playerID string) (*Pet, *Loot, error) {
	p, err := s.pets.GetByID(ctx, petID)
	if err != nil {
		return nil, nil, httputil.NewNotFound("not_found", "pet not found")
	}
	if p.PlayerID != playerID {
		return nil, nil, httputil.NewForbidden("not_owner", "not your pet")
	}
	if p.Task == nil || p.Status != canon.PetStateMissionActive {
		return nil, nil, httputil.NewBadRequest("not_on_task", "pet is not on a task")
	}

	if time.Now().Before(p.Task.ReturnsAt) {
		// Early recall — no reward.
		p.Status = canon.PetStateIdle
		p.Task = nil
		if err := s.pets.Update(ctx, p); err != nil {
			return nil, nil, fmt.Errorf("update pet: %w", err)
		}
		p.Decorate()
		return p, &Loot{}, nil
	}

	// Timer elapsed — pay out in full.
	loot, err := s.claimMission(ctx, p)
	if err != nil {
		return nil, nil, err
	}
	p.Decorate()
	return p, loot, nil
}

// AutoClaimReturned settles every pet whose mission timer has elapsed:
// generates loot, credits resources, levels the pet and frees it. Runs on a
// schedule so rewards arrive without the player tapping anything.
func (s *Service) AutoClaimReturned(ctx context.Context) (int, error) {
	pets, err := s.pets.ListActiveMissions(ctx)
	if err != nil {
		return 0, fmt.Errorf("list active missions: %w", err)
	}
	now := time.Now()
	claimed := 0
	for i := range pets {
		p := &pets[i]
		if p.Task == nil || now.Before(p.Task.ReturnsAt) {
			continue
		}
		if _, err := s.claimMission(ctx, p); err != nil {
			// Non-fatal: log-and-continue so one bad pet doesn't stall the rest.
			continue
		}
		claimed++
	}
	return claimed, nil
}

// claimMission generates the reward for a completed task, credits the player,
// applies XP/level-up, frees the pet, and fires the "returned" push. Shared by
// the manual Recall (timer elapsed) and the auto-claim worker.
func (s *Service) claimMission(ctx context.Context, p *Pet) (*Loot, error) {
	loot := &Loot{
		Energy:    10 + rand.Intn(21), // 10-30
		Materials: 5 + rand.Intn(11),  // 5-15
	}
	if p.Task.Type == TaskTypeScout {
		scoutResult, err := s.resolveScout(ctx, p.Task)
		if err != nil {
			return nil, err
		}
		loot.Energy = 0
		loot.Materials = 0
		loot.Rare = false
		loot.Scout = scoutResult
	}

	rareChance, ok := RareChance[p.Level]
	if !ok {
		rareChance = RareChance[1]
	}
	if p.Task.Type == TaskTypeSearch && p.Task.LootRoll < rareChance {
		loot.Rare = true
		loot.Energy += 20
		loot.Materials += 10
	}

	// Credit the loot to the player (search tasks only — scout yields intel).
	// This was previously dropped on the floor: the reward never reached the
	// player's balance.
	if s.resources != nil && (loot.Energy > 0 || loot.Materials > 0) {
		if _, err := s.resources.Add(ctx, p.PlayerID, loot.Energy, loot.Materials); err != nil {
			return nil, fmt.Errorf("credit loot: %w", err)
		}
	}

	// Pet XP + level-up (and evolution-stage crossing).
	prevLevel := p.Level
	prevStage, _ := EvolutionStage(p.Level)
	p.XP += 10 + rand.Intn(21)
	if p.Level < MaxLevel {
		needed := XPForLevel(p.Level + 1)
		if p.XP >= needed {
			p.Level++
			p.XP -= needed
		}
	}
	if p.Level > prevLevel {
		loot.LeveledTo = p.Level
		if newStage, name := EvolutionStage(p.Level); newStage > prevStage {
			loot.Evolved = true
			loot.EvolvedTo = name
			// D1: pet evolution yields a resonance signature (alongside boss drops).
			if s.inventory != nil {
				spectrum := evolutionSpectrum(p.ID, newStage)
				if _, err := s.inventory.Grant(ctx, p.PlayerID, item.TypeResonanceSignature, spectrum, 1); err != nil {
					slog.Error("grant evolution signature failed", "pet_id", p.ID, "error", err)
				} else {
					loot.Signature = spectrum
					loot.SignatureRu = item.SpectrumRu(spectrum)
				}
			}
		}
	}

	taskType := p.Task.Type
	p.Status = canon.PetStateIdle
	p.Task = nil

	if err := s.pets.Update(ctx, p); err != nil {
		return nil, fmt.Errorf("update pet: %w", err)
	}
	if s.quests != nil {
		_ = s.quests.UpdateProgress(ctx, p.PlayerID, "recall_pet")
	}
	if s.push != nil {
		_ = s.push.NotifyPetReturned(ctx, p.PlayerID, taskType)
	}
	return loot, nil
}

func (s *Service) selectScoutTower(ctx context.Context, playerID string) (string, error) {
	p, err := s.players.GetByID(ctx, playerID)
	if err != nil {
		return "", httputil.NewInternal("failed to get player")
	}

	ownedTowers, err := s.towers.GetByOwner(ctx, playerID)
	if err != nil {
		return "", httputil.NewInternal("failed to get towers")
	}

	type candidate struct {
		id   string
		dist float64
	}
	candidates := make([]candidate, 0, len(ownedTowers))
	nearestSafeDist := math.MaxFloat64 // closest safe+active tower, ignoring the walk-up gate

	for _, tw := range ownedTowers {
		c, err := s.cells.GetByID(ctx, tw.CellID)
		if err != nil || c == nil {
			continue
		}
		if canon.InfectionBand(c.Infection) != canon.InfectionBandSafe {
			continue
		}

		overloadCount, err := s.towers.CountAllInRadius(ctx, c.Lat, c.Lng, canon.TowerOverloadRadiusM)
		if err == nil && canon.ResolveTowerState(overloadCount) != canon.TowerStateActive {
			continue
		}

		dist := geo.Haversine(p.Position.Lat, p.Position.Lng, c.Lat, c.Lng)
		if dist < nearestSafeDist {
			nearestSafeDist = dist
		}
		if dist > canon.TowerPlacementMaxDistanceM {
			continue
		}
		candidates = append(candidates, candidate{id: tw.ID, dist: dist})
	}

	if len(candidates) == 0 {
		// Actionable message: distinguish "you have a safe tower but you're too
		// far from it" (walk over there) from "you have no safe-zone tower yet".
		if nearestSafeDist != math.MaxFloat64 {
			return "", httputil.NewBadRequest("too_far_from_safe_tower",
				fmt.Sprintf("Слишком далеко от безопасного маяка: ближайший в %d м, подойди ближе (нужно ≤%d м).",
					int(nearestSafeDist), int(canon.TowerPlacementMaxDistanceM)))
		}
		return "", httputil.NewBadRequest("no_safe_tower",
			"Нужен свой маяк в безопасной зоне рядом — поставь маяк и дождись, пока зона очистится от заражения.")
	}

	sort.Slice(candidates, func(i, j int) bool { return candidates[i].dist < candidates[j].dist })
	return candidates[0].id, nil
}

func (s *Service) resolveScout(ctx context.Context, task *PetTask) (*ScoutResult, error) {
	result := &ScoutResult{
		TowerID:       task.TargetTowerID,
		CooldownUntil: task.StartedAt.Truncate(ScoutCooldownWindow).Add(ScoutCooldownWindow),
	}
	if task.TargetTowerID == "" {
		result.Status = "invalid_target"
		return result, nil
	}

	tw, err := s.towers.GetByID(ctx, task.TargetTowerID)
	if err != nil {
		return nil, httputil.NewInternal("failed to get scout tower")
	}
	towerCell, err := s.cells.GetByID(ctx, tw.CellID)
	if err != nil || towerCell == nil {
		return nil, httputil.NewInternal("failed to get scout cell")
	}
	if canon.InfectionBand(towerCell.Infection) != canon.InfectionBandSafe {
		result.Status = "unsafe"
		return result, nil
	}

	cellsInRange, err := s.cells.GetInRadius(ctx, towerCell.Lat, towerCell.Lng, tw.RadiusM)
	if err != nil {
		return nil, httputil.NewInternal("failed to get scout cells")
	}

	buildingCells := make([]cell.Cell, 0, len(cellsInRange))
	for _, c := range cellsInRange {
		if c.Terrain == "building" {
			buildingCells = append(buildingCells, c)
		}
	}
	if len(buildingCells) == 0 {
		result.Status = "no_buildings"
		return result, nil
	}

	windowStart := task.StartedAt.Truncate(ScoutCooldownWindow)
	discoveries := generateScoutSurvivors(task.TargetTowerID, buildingCells, windowStart)
	result.Survivors = discoveries
	if len(discoveries) == 0 {
		result.Status = "exhausted"
	} else {
		result.Status = "found_survivors"
	}
	return result, nil
}

func generateScoutSurvivors(towerID string, buildingCells []cell.Cell, windowStart time.Time) []survivor.Survivor {
	seedInput := fmt.Sprintf("%s_%d", towerID, windowStart.Unix())
	hash := sha256.Sum256([]byte(seedInput))
	seed := int64(hash[0])<<24 | int64(hash[1])<<16 | int64(hash[2])<<8 | int64(hash[3])
	rng := rand.New(rand.NewSource(seed))

	sort.Slice(buildingCells, func(i, j int) bool {
		if buildingCells[i].Lat == buildingCells[j].Lat {
			return buildingCells[i].Lng < buildingCells[j].Lng
		}
		return buildingCells[i].Lat < buildingCells[j].Lat
	})

	maxScans := int(math.Min(3, float64(len(buildingCells))))
	survivorsFound := make([]survivor.Survivor, 0, maxScans)
	for i := 0; i < maxScans; i++ {
		c := buildingCells[(i+rng.Intn(len(buildingCells)))%len(buildingCells)]
		if rng.Float64() > 0.6 {
			continue
		}
		sType := "fighter"
		if rng.Float64() < 0.35 {
			sType = "engineer"
		}
		idSeed := fmt.Sprintf("%s_%s_%d", towerID, c.ID, windowStart.Unix())
		survivorsFound = append(survivorsFound, survivor.Survivor{
			ID:   fmt.Sprintf("%x", sha256.Sum256([]byte(idSeed)))[:16],
			Lat:  c.Lat,
			Lng:  c.Lng,
			Type: sType,
		})
	}

	return survivorsFound
}
