package hive

import (
	"context"
	"testing"

	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/internal/rift"
	"github.com/stretchr/testify/assert"
)

type fakeRepo struct {
	open      []Hive
	pulsed    int
	created   []Hive
	seedOK    bool
	hotOK     bool
	intensity int
	cleansed  int
	empowered int
	closed    int
}

func (f *fakeRepo) Create(ctx context.Context, h *Hive) error {
	h.ID = "new-hive"
	f.created = append(f.created, *h)
	return nil
}
func (f *fakeRepo) GetByID(ctx context.Context, id string) (*Hive, error) {
	return &Hive{ID: id, Level: 1, Intensity: f.intensity}, nil
}
func (f *fakeRepo) ReduceIntensity(ctx context.Context, id string, delta int) (int, error) {
	f.intensity -= delta
	if f.intensity < 0 {
		f.intensity = 0
	}
	return f.intensity, nil
}
func (f *fakeRepo) CleanseRegion(ctx context.Context, h *Hive, toLevel float64) (int, error) {
	f.cleansed++
	return 7, nil
}
func (f *fakeRepo) GetOpen(ctx context.Context) ([]Hive, error) { return f.open, nil }
func (f *fakeRepo) GetNearby(ctx context.Context, lat, lng, radiusM float64) ([]Hive, error) {
	return f.open, nil
}
func (f *fakeRepo) Close(ctx context.Context, id string) error { f.closed++; return nil }
func (f *fakeRepo) PulseInfection(ctx context.Context, h *Hive, amount float64) (int, error) {
	f.pulsed++
	return 5, nil
}
func (f *fakeRepo) Empower(ctx context.Context, hiveID string, delta int) (int, int, error) {
	f.empowered += delta
	return 50 + f.empowered, 2, nil
}
func (f *fakeRepo) HottestCellInZone(ctx context.Context, h *Hive, minInfection float64) (string, float64, float64, float64, bool, error) {
	if f.hotOK {
		return "cell-1", 54.7, 20.5, 92.0, true, nil
	}
	return "", 0, 0, 0, false, nil
}
func (f *fakeRepo) SeedCandidate(ctx context.Context, minInfection, excludeM float64) (string, float64, float64, bool, error) {
	if f.seedOK {
		return "seed-cell", 54.8, 20.6, true, nil
	}
	return "", 0, 0, false, nil
}

type fakeSpawner struct{ calls int }

func (f *fakeSpawner) MaybeSpawn(ctx context.Context, cellID string, infection float64) (*rift.Rift, error) {
	f.calls++
	return &rift.Rift{ID: "r1", CellID: cellID, Type: "medium"}, nil
}

func TestPulse_PulsesEachHiveAndSeedsRift(t *testing.T) {
	repo := &fakeRepo{open: []Hive{{ID: "h1", Level: 1, Intensity: 100}}, hotOK: true}
	sp := &fakeSpawner{}
	svc := NewService(repo, sp)
	err := svc.Pulse(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 1, repo.pulsed)
	assert.Equal(t, 1, sp.calls, "seeds a rift on the hottest in-zone cell")
}

func TestPulse_SeedsNewHiveWhenCandidate(t *testing.T) {
	repo := &fakeRepo{open: nil, seedOK: true}
	svc := NewService(repo, &fakeSpawner{})
	err := svc.Pulse(context.Background())
	assert.NoError(t, err)
	assert.Len(t, repo.created, 1)
	assert.Equal(t, 1, repo.created[0].Level)
	assert.Equal(t, "seed-cell", repo.created[0].CellID)
}

func TestPulse_RespectsHiveCap(t *testing.T) {
	full := make([]Hive, maxOpenHives)
	for i := range full {
		full[i] = Hive{ID: "h", Level: 1, Intensity: 100}
	}
	repo := &fakeRepo{open: full, seedOK: true}
	svc := NewService(repo, &fakeSpawner{})
	_ = svc.Pulse(context.Background())
	assert.Empty(t, repo.created, "no new hive seeded once at the cap")
}

func TestConfig_ClampsUnknownLevel(t *testing.T) {
	assert.Equal(t, levelConfigs[1], Config(99))
	assert.Equal(t, "critical", Config(3).RiftType)
}

func TestOnAssaultVictory_WeakensThenCollapses(t *testing.T) {
	ctx := context.Background()
	// intensity 100, damage 55 → first assault leaves 45 (weakened, not closed).
	repo := &fakeRepo{intensity: 100}
	svc := NewService(repo, &fakeSpawner{})
	closed, err := svc.OnAssaultVictory(ctx, "h1")
	assert.NoError(t, err)
	assert.False(t, closed)
	assert.Equal(t, 45, repo.intensity)
	assert.Equal(t, 0, repo.closed)

	// second assault → 0 → collapse (close + cleanse).
	closed, err = svc.OnAssaultVictory(ctx, "h1")
	assert.NoError(t, err)
	assert.True(t, closed)
	assert.Equal(t, 1, repo.closed)
	assert.Equal(t, 1, repo.cleansed)
}

type fakeFaction struct{ symbiont bool }

func (f fakeFaction) IsSymbiont(ctx context.Context, p string) (bool, error) { return f.symbiont, nil }

type fakeSpend struct{}

func (fakeSpend) Spend(ctx context.Context, p string, e, m int) (*player.Player, error) {
	return &player.Player{}, nil
}

func TestEmpower_GatedToSymbiont(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{intensity: 40}
	svc := NewService(repo, &fakeSpawner{})

	// Human → forbidden.
	svc.SetEmpowerDeps(fakeFaction{symbiont: false}, fakeSpend{})
	_, err := svc.Empower(ctx, "p1", "h1")
	assert.Error(t, err)

	// Symbiont → empowers (intensity rises, repo.empowered tracked).
	svc.SetEmpowerDeps(fakeFaction{symbiont: true}, fakeSpend{})
	h, err := svc.Empower(ctx, "p1", "h1")
	assert.NoError(t, err)
	assert.Equal(t, EmpowerIntensity, repo.empowered)
	assert.Equal(t, 2, h.Level)
}
