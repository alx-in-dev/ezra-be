package rift

import (
	"context"
	"math/rand"

	"github.com/ezra-game/server/internal/canon"
	"github.com/ezra-game/server/internal/cell"
	"github.com/ezra-game/server/pkg/geo"
	"github.com/ezra-game/server/pkg/httputil"
)

type Service struct {
	rifts Repository
	cells cell.Repository
	push  interface {
		NotifyNearbyRift(ctx context.Context, lat, lng float64, riftType string) error
	}
}

func NewService(rifts Repository, cells cell.Repository, push interface {
	NotifyNearbyRift(ctx context.Context, lat, lng float64, riftType string) error
}) *Service {
	return &Service{rifts: rifts, cells: cells, push: push}
}

func (s *Service) GetByID(ctx context.Context, id string) (*Rift, error) {
	return s.rifts.GetByID(ctx, id)
}

// AmplifyRift raises an open rift's intensity by delta (a Symbiont entity's
// autonomous "усиление разлома", R4-2 L2), capped at canon.RiftAmplifyCap so a
// parked entity can't run a rift to infinity. Returns open=false when the rift
// is closed/missing, so the caller disengages the entity.
func (s *Service) AmplifyRift(ctx context.Context, riftID string, delta int) (open bool, intensity int, err error) {
	r, err := s.rifts.GetByID(ctx, riftID)
	if err != nil || r == nil || r.ClosedAt != nil {
		return false, 0, nil
	}
	r.Intensity += delta
	if r.Intensity > canon.RiftAmplifyCap {
		r.Intensity = canon.RiftAmplifyCap
	}
	if err := s.rifts.Update(ctx, r); err != nil {
		return false, 0, err
	}
	return true, r.Intensity, nil
}

// NearestOpenRift returns an open rift within radius of (lat,lng), or nil if
// none — the target an entity auto-acquires when dispatched to "rift" duty
// (R4-2 L2). The spatial filter already bounds the set; the first match is fine.
func (s *Service) NearestOpenRift(ctx context.Context, lat, lng, radiusM float64) (*Rift, error) {
	rifts, err := s.rifts.GetNearby(ctx, lat, lng, radiusM)
	if err != nil {
		return nil, err
	}
	if len(rifts) == 0 {
		return nil, nil
	}
	return &rifts[0], nil
}

// MaybeSpawn checks if a rift should spawn at a cell.
// Chance = (infection - 75) * 2% per hour. Since called every 5 min, divide by 12.
func (s *Service) MaybeSpawn(ctx context.Context, cellID string, infection float64) (*Rift, error) {
	if infection < 75 {
		return nil, nil
	}

	// Check cell doesn't already have a rift
	c, err := s.cells.GetByID(ctx, cellID)
	if err != nil {
		return nil, err
	}
	if c.RiftID != nil {
		return nil, nil
	}

	chance := (infection - 75) * 0.02 / 12 // per 5 min
	if rand.Float64() > chance {
		return nil, nil
	}

	// Determine type based on infection level
	riftType := "minor"
	if infection >= 90 {
		riftType = "critical"
	} else if infection >= 82 {
		riftType = "medium"
	}

	cfg := TypeConfigs[riftType]
	spirits := cfg.MinSpirits + rand.Intn(cfg.MaxSpirits-cfg.MinSpirits+1)

	r := &Rift{
		CellID:       cellID,
		Type:         riftType,
		Intensity:    1,
		RadiusCells:  cfg.RadiusCells,
		SpiritsCount: spirits,
	}

	if err := s.rifts.Create(ctx, r); err != nil {
		return nil, err
	}

	// Link rift to cell
	if err := s.cells.SetRiftID(ctx, cellID, &r.ID); err != nil {
		return nil, err
	}
	if s.push != nil {
		_ = s.push.NotifyNearbyRift(ctx, c.Lat, c.Lng, r.Type)
	}

	return r, nil
}

// Organic spawn tuning: canon says a ≥75% cell rolls (infection−75)×2%/hour
// (docs/03_world_system.md). The batch world runs at 95–100%, so an unscoped
// roll would flood the map — density is capped instead: spawns keep
// organicSpacingM from any open rift and at most organicSpawnPerTick open per
// tick. Spacing is the real limiter; the per-tick cap just smooths the ramp.
const (
	organicMinInfection = 75.0
	organicSpacingM     = 500.0
	organicSpawnPerTick = 10
	organicTicksPerHour = 6 // rift:spawn_organic runs @every 10m
)

// riftTypeForInfection picks the canon rift tier for a cell's infection level.
func riftTypeForInfection(infection float64) string {
	switch {
	case infection >= 90:
		return "critical"
	case infection >= 82:
		return "medium"
	default:
		return "minor"
	}
}

// SpawnOrganic opens rifts on hot cells world-wide (the canon "infection>75%
// breeds rifts" loop, previously only wired to hive pulses). Density-capped by
// the repository query; each sampled candidate still rolls the canon chance.
func (s *Service) SpawnOrganic(ctx context.Context) (spawned int, err error) {
	lister, ok := s.rifts.(interface {
		OrganicCandidates(ctx context.Context, minInfection, spacingM float64, limit int) ([]SpawnCandidate, error)
	})
	if !ok {
		return 0, nil // test fake without organic support
	}
	candidates, err := lister.OrganicCandidates(ctx, organicMinInfection, organicSpacingM, organicSpawnPerTick*3)
	if err != nil {
		return 0, err
	}
	var opened []SpawnCandidate
	for _, c := range candidates {
		if spawned >= organicSpawnPerTick {
			break
		}
		chance := (c.Infection - organicMinInfection) * 0.02 / organicTicksPerHour
		if rand.Float64() > chance {
			continue
		}
		// The repo query enforces spacing against pre-existing rifts; keep
		// same-tick spawns apart from each other too.
		tooClose := false
		for _, o := range opened {
			if geo.Haversine(c.Lat, c.Lng, o.Lat, o.Lng) < organicSpacingM {
				tooClose = true
				break
			}
		}
		if tooClose {
			continue
		}
		if _, err := s.ForceSpawn(ctx, c.CellID, riftTypeForInfection(c.Infection)); err != nil {
			return spawned, err
		}
		opened = append(opened, c)
		spawned++
	}
	return spawned, nil
}

// EnsureContactRift guarantees the onboarding Contact beat is playable: if no
// open rift exists near the player, force-spawn a minor one on the most
// infected rift-free cell nearby. Without this, a player whose area has no
// hive (the only organic rift source) is soft-locked on contact_step forever.
// Returns the rift satisfying the guarantee (existing or spawned), or nil when
// the area has no eligible cells at all.
func (s *Service) EnsureContactRift(ctx context.Context, lat, lng float64) (*Rift, error) {
	const contactRiftSearchM = 500
	open, err := s.rifts.GetNearby(ctx, lat, lng, contactRiftSearchM)
	if err != nil {
		return nil, err
	}
	if len(open) > 0 {
		return &open[0], nil
	}

	cells, err := s.cells.GetInRadius(ctx, lat, lng, contactRiftSearchM)
	if err != nil {
		return nil, err
	}
	var best *cell.Cell
	for i := range cells {
		c := &cells[i]
		if c.RiftID != nil || c.TowerID != nil {
			continue
		}
		if best == nil || c.Infection > best.Infection {
			best = c
		}
	}
	if best == nil {
		return nil, nil
	}
	return s.ForceSpawn(ctx, best.ID, "minor")
}

// ForceSpawn unconditionally opens a rift of the given type at a cell (no
// infection/chance gate). Used by the Symbiont Dome Breach (PvP). No-op if the
// cell already has a rift.
func (s *Service) ForceSpawn(ctx context.Context, cellID, riftType string) (*Rift, error) {
	c, err := s.cells.GetByID(ctx, cellID)
	if err != nil {
		return nil, err
	}
	if c.RiftID != nil {
		return nil, nil // already breached
	}
	cfg, ok := TypeConfigs[riftType]
	if !ok {
		cfg = TypeConfigs["medium"]
		riftType = "medium"
	}
	spirits := cfg.MinSpirits + rand.Intn(cfg.MaxSpirits-cfg.MinSpirits+1)
	r := &Rift{CellID: cellID, Type: riftType, Intensity: 1, RadiusCells: cfg.RadiusCells, SpiritsCount: spirits}
	if err := s.rifts.Create(ctx, r); err != nil {
		return nil, err
	}
	if err := s.cells.SetRiftID(ctx, cellID, &r.ID); err != nil {
		return nil, err
	}
	if s.push != nil {
		_ = s.push.NotifyNearbyRift(ctx, c.Lat, c.Lng, r.Type)
	}
	return r, nil
}

// Close marks a rift as closed. Called after successful battle.
func (s *Service) Close(ctx context.Context, riftID string) error {
	r, err := s.rifts.GetByID(ctx, riftID)
	if err != nil {
		return httputil.NewNotFound("not_found", "rift not found")
	}

	if err := s.rifts.Close(ctx, riftID); err != nil {
		return err
	}

	// Unlink from cell
	return s.cells.SetRiftID(ctx, r.CellID, nil)
}
