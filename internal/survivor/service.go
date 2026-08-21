package survivor

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"time"

	"github.com/ezra-game/server/internal/canon"
	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/internal/unit"
	"github.com/ezra-game/server/pkg/geo"
	"github.com/ezra-game/server/pkg/httputil"
)

// Service handles survivor spawning and recruitment.
type Service struct {
	units   unit.Repository
	players player.Repository
	domes   DomeReader
	// faction gates recruitment to Humans (T-800). nil-safe: without it the
	// gate is skipped.
	faction FactionChecker
}

// FactionChecker reports a player's faction for the human-toolkit exclusion
// gate (T-800: Symbionts don't recruit an army).
type FactionChecker interface {
	IsSymbiont(ctx context.Context, playerID string) (bool, error)
}

// NewService creates a new survivor service.
func NewService(units unit.Repository, players player.Repository) *Service {
	return &Service{units: units, players: players}
}

// WithDomes wires the dome reader so survivors are discovered inside the
// player's dome (canon channel). Nil-safe: without it the service falls back to
// free nearby spawning (tests / no dome data).
func (s *Service) WithDomes(d DomeReader) *Service {
	s.domes = d
	return s
}

// WithFaction wires the optional faction gate. Kept off the constructor so
// existing callers/tests stay unchanged.
func (s *Service) WithFaction(f FactionChecker) *Service {
	s.faction = f
	return s
}

// SpawnNearby generates 0-3 survivors within 80m of the given position.
// Uses a seed based on position + truncated time for semi-deterministic results.
func (s *Service) SpawnNearby(lat, lng float64) []Survivor {
	// Seed based on position grid + 5-minute window for determinism
	window := time.Now().Unix() / 300
	seed := fmt.Sprintf("%.4f_%.4f_%d", lat, lng, window)
	h := sha256.Sum256([]byte(seed))
	rng := rand.New(rand.NewSource(int64(h[0])<<24 | int64(h[1])<<16 | int64(h[2])<<8 | int64(h[3])))

	count := rng.Intn(4) // 0 to 3 survivors
	survivors := make([]Survivor, 0, count)

	for i := 0; i < count; i++ {
		// Random offset within 80m
		angle := rng.Float64() * 2 * math.Pi
		dist := 10 + rng.Float64()*70 // 10-80m
		dLat := (dist / 111320.0) * math.Cos(angle)
		dLng := (dist / (111320.0 * math.Cos(lat*math.Pi/180))) * math.Sin(angle)

		sType := pickType(rng)
		id := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s_%d_%s", seed, i, sType))))[:16]

		survivors = append(survivors, Survivor{
			ID:   id,
			Lat:  lat + dLat,
			Lng:  lng + dLng,
			Type: sType,
		})
	}

	return survivors
}

// onboardingGuaranteedSurvivors is how many survivors are guaranteed to be
// available while the player is on the survivor onboarding step, so the
// tutorial can never stall on a run of empty RNG spawns (softlock S4).
const onboardingGuaranteedSurvivors = 2

// domeScanRadiusM is how far around the player we look for domed building cells
// to populate with survivors.
const domeScanRadiusM = 200.0

// maxSurvivorsUnderDome caps how many survivors a single scan surfaces.
const maxSurvivorsUnderDome = 6

// SpawnForPlayer returns the survivors discoverable around (lat,lng). With a
// dome reader wired (production), survivors appear only inside the player's
// domed building cells (canon channel, docs §2.6); without it the service falls
// back to free nearby spawning (tests). The onboarding step always guarantees a
// minimum so the tutorial can start before the player has a dome (A2).
func (s *Service) SpawnForPlayer(ctx context.Context, playerID string, lat, lng float64) []Survivor {
	var survivors []Survivor
	if s.domes != nil {
		cells, err := s.domes.DomedBuildingCells(ctx, playerID, lat, lng, domeScanRadiusM)
		if err != nil {
			slog.Warn("survivor: domed building cells lookup failed", "player_id", playerID, "error", err)
		} else {
			survivors = spawnInDomedCells(cells)
		}
	} else {
		survivors = s.SpawnNearby(lat, lng)
	}

	if playerID == "" || s.players == nil {
		return survivors
	}
	p, err := s.players.GetByID(ctx, playerID)
	if err != nil || p == nil || p.OnboardingStep != canon.OnboardingSurvivorStep {
		return survivors
	}
	return ensureGuaranteedSurvivors(lat, lng, survivors, onboardingGuaranteedSurvivors)
}

// spawnInDomedCells places one survivor in each domed building cell (capped),
// at the cell centre, with a type chosen deterministically per cell/5-min
// window so repeated reads return a stable set the player can walk to.
func spawnInDomedCells(cells []BuildingCell) []Survivor {
	window := time.Now().Unix() / 300
	out := make([]Survivor, 0, len(cells))
	for _, c := range cells {
		if len(out) >= maxSurvivorsUnderDome {
			break
		}
		seed := fmt.Sprintf("dome_%.5f_%.5f_%d", c.Lat, c.Lng, window)
		h := sha256.Sum256([]byte(seed))
		rng := rand.New(rand.NewSource(int64(h[0])<<24 | int64(h[1])<<16 | int64(h[2])<<8 | int64(h[3])))
		sType := pickType(rng)
		id := fmt.Sprintf("%x", sha256.Sum256([]byte(seed+"_id")))[:16]
		out = append(out, Survivor{ID: id, Lat: c.Lat, Lng: c.Lng, Type: sType})
	}
	return out
}

// ensureGuaranteedSurvivors tops the list up to min fighters using the same
// per-position/5-minute-window determinism as SpawnNearby, so repeated reads in
// a window return a stable set the player can walk to and recruit.
func ensureGuaranteedSurvivors(lat, lng float64, survivors []Survivor, min int) []Survivor {
	window := time.Now().Unix() / 300
	for i := len(survivors); i < min; i++ {
		seed := fmt.Sprintf("guaranteed_%.4f_%.4f_%d_%d", lat, lng, window, i)
		h := sha256.Sum256([]byte(seed))
		rng := rand.New(rand.NewSource(int64(h[0])<<24 | int64(h[1])<<16 | int64(h[2])<<8 | int64(h[3])))

		angle := rng.Float64() * 2 * math.Pi
		dist := 10 + rng.Float64()*40 // 10-50m: comfortably inside the 100m recruit range
		dLat := (dist / 111320.0) * math.Cos(angle)
		dLng := (dist / (111320.0 * math.Cos(lat*math.Pi/180))) * math.Sin(angle)

		id := fmt.Sprintf("%x", sha256.Sum256([]byte(seed+"_id")))[:16]
		survivors = append(survivors, Survivor{
			ID:   id,
			Lat:  lat + dLat,
			Lng:  lng + dLng,
			Type: "fighter", // useful toward both the tutorial and the rift gate
		})
	}
	return survivors
}

// Recruit creates a unit from a survivor. Validates distance from player position.
func (s *Service) Recruit(ctx context.Context, playerID, survivorType string, sLat, sLng float64) (*unit.Unit, error) {
	if s.faction != nil {
		if isSymbiont, err := s.faction.IsSymbiont(ctx, playerID); err == nil && isSymbiont {
			return nil, httputil.NewForbidden("symbiont_no_human_toolkit", "симбионт не может набирать отряд")
		}
	}
	if survivorType != "fighter" && survivorType != "engineer" {
		return nil, httputil.NewBadRequest("invalid_type", "type must be fighter or engineer in MVP")
	}
	stats, ok := unit.BaseStats[survivorType]
	if !ok {
		return nil, httputil.NewBadRequest("invalid_type", "type must be fighter or engineer in MVP")
	}

	p, err := s.players.GetByID(ctx, playerID)
	if err != nil {
		return nil, httputil.NewInternal("failed to get player")
	}

	// Validate distance: must be within 100m of survivor
	dist := geo.Haversine(p.Position.Lat, p.Position.Lng, sLat, sLng)
	if dist > 100 {
		return nil, httputil.NewBadRequest("too_far", "survivor is too far away")
	}

	// Check army limit
	count, err := s.units.CountByPlayer(ctx, playerID)
	if err != nil {
		return nil, httputil.NewInternal("failed to count units")
	}
	if count >= p.ArmyLimit {
		return nil, httputil.NewBadRequest("army_limit_reached", "you have reached your army limit")
	}

	u := &unit.Unit{
		PlayerID: playerID,
		Type:     survivorType,
		Level:    1,
		XP:       0,
		HP:       stats.HP,
		ATK:      stats.ATK,
		Status:   "idle",
	}

	if err := s.units.Create(ctx, u); err != nil {
		return nil, httputil.NewInternal("failed to create unit")
	}

	newCount := count + 1
	if p.OnboardingStep == canon.OnboardingSurvivorStep && newCount >= 2 {
		p.OnboardingStep = canon.OnboardingTutorialBattle
		if err := s.players.Update(ctx, p); err != nil {
			return nil, httputil.NewInternal("failed to advance onboarding")
		}
	}

	return u, nil
}

// pickType selects a survivor type based on weighted probabilities.
func pickType(rng *rand.Rand) string {
	total := 0
	for _, w := range TypeWeights {
		total += w.Weight
	}
	roll := rng.Intn(total)
	for _, w := range TypeWeights {
		roll -= w.Weight
		if roll < 0 {
			return w.Type
		}
	}
	return "fighter"
}
