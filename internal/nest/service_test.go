package nest

import (
	"context"
	"testing"
	"time"

	"github.com/ezra-game/server/internal/canon"
	"github.com/ezra-game/server/internal/cell"
)

// fakeRepo is an in-memory Repository for service-logic tests (no live DB).
type fakeRepo struct {
	live        *Nest
	everOwned   bool
	created     int
	relocated   int
	buffer      float64
	collapsed   []string
	domed       bool
	pierced     bool
	pocketAdded bool
}

func (f *fakeRepo) Create(_ context.Context, n *Nest) error {
	n.ID = "nest-1"
	n.SiegeHP = n.Config().SiegeHPMax
	n.SiegeState = SiegeHealthy
	n.SupportLevel = 100
	f.live = n
	f.everOwned = true
	f.created++
	return nil
}
func (f *fakeRepo) GetLiveByOwner(_ context.Context, _ string) (*Nest, error) { return f.live, nil }
func (f *fakeRepo) GetByID(_ context.Context, _ string) (*Nest, error)        { return f.live, nil }
func (f *fakeRepo) HasEverOwned(_ context.Context, _ string) (bool, error)    { return f.everOwned, nil }
func (f *fakeRepo) UpdateLocation(_ context.Context, n *Nest) error {
	now := time.Now()
	n.RelocatedAt = &now
	f.live = n
	f.relocated++
	return nil
}
func (f *fakeRepo) AccrueTrickle(_ context.Context, _ [canon.NestMaxLevel + 1]float64, _, _ float64) (int64, error) {
	return 0, nil
}
func (f *fakeRepo) ApplyDecay(_ context.Context, _, _ float64) (int64, error)  { return 0, nil }
func (f *fakeRepo) Feed(_ context.Context, _ string, _ float64) error          { return nil }
func (f *fakeRepo) CollectBuffer(_ context.Context, _ string) (float64, error) { return f.buffer, nil }
func (f *fakeRepo) UpdateSiege(_ context.Context, n *Nest) error               { f.live = n; return nil }
func (f *fakeRepo) ApplyCollapses(_ context.Context) ([]string, error)         { return f.collapsed, nil }
func (f *fakeRepo) CellPlacementStatus(_ context.Context, _ string) (bool, bool, error) {
	return f.domed, f.pierced, nil
}
func (f *fakeRepo) AddPocketCell(_ context.Context, _, _ string) error {
	f.pocketAdded = true
	return nil
}
func (f *fakeRepo) RefreshPocketCells(_ context.Context, _ int) (int64, error)   { return 0, nil }
func (f *fakeRepo) GetNearby(_ context.Context, _, _, _ float64) ([]Nest, error) { return nil, nil }

type fakeCells struct{}

func (fakeCells) GetByID(_ context.Context, id string) (*cell.Cell, error) {
	return &cell.Cell{ID: id, Lat: 53.13, Lng: 50.15}, nil
}

// countingSpender records how many times crystals were charged.
type countingSpender struct {
	charges int
	afford  bool
}

func (c *countingSpender) SpendCrystals(_ context.Context, _ string, _ int) (bool, error) {
	c.charges++
	return c.afford, nil
}

func TestOpenFirstNest_FreeThenRejectsSecond(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo, fakeCells{}, nil, nil)

	n, err := svc.OpenFirstNest(context.Background(), "p1", "c1")
	if err != nil {
		t.Fatalf("first nest should open free: %v", err)
	}
	if n.Level != 1 || n.AuraRadiusM == 0 {
		t.Fatalf("nest not decorated: level=%d aura=%.0f", n.Level, n.AuraRadiusM)
	}
	// A second OpenFirstNest must be refused (already owned).
	if _, err := svc.OpenFirstNest(context.Background(), "p1", "c2"); err == nil {
		t.Fatal("second OpenFirstNest should be rejected")
	}
}

func TestCreate_RebuildChargesCrystals(t *testing.T) {
	// everOwned + no live nest = rebuild after collapse → must charge.
	repo := &fakeRepo{everOwned: true}
	spender := &countingSpender{afford: true}
	svc := NewService(repo, fakeCells{}, spender, nil)

	if _, err := svc.Create(context.Background(), "p1", "c1"); err != nil {
		t.Fatalf("rebuild should succeed when affordable: %v", err)
	}
	if spender.charges != 1 {
		t.Fatalf("rebuild should charge once, got %d", spender.charges)
	}
}

func TestRelocate_BlockedUnderSiege(t *testing.T) {
	repo := &fakeRepo{live: &Nest{ID: "n1", OwnerID: "p1", Level: 1, SiegeState: SiegeUnderSiege}}
	svc := NewService(repo, fakeCells{}, nil, nil)

	if _, err := svc.Relocate(context.Background(), "p1", "c2"); err == nil {
		t.Fatal("relocation under siege must be blocked")
	}
	if repo.relocated != 0 {
		t.Fatal("no relocation should have been persisted under siege")
	}
}

func TestRelocate_FirstFreeThenPaid(t *testing.T) {
	repo := &fakeRepo{live: &Nest{ID: "n1", OwnerID: "p1", Level: 1, SiegeState: SiegeHealthy}}
	spender := &countingSpender{afford: true}
	svc := NewService(repo, fakeCells{}, spender, nil)

	// First relocation: RelocatedAt nil → free.
	if _, err := svc.Relocate(context.Background(), "p1", "c2"); err != nil {
		t.Fatalf("first relocation should be free: %v", err)
	}
	if spender.charges != 0 {
		t.Fatalf("first relocation must not charge, got %d", spender.charges)
	}
	// Second relocation: RelocatedAt now set → paid.
	if _, err := svc.Relocate(context.Background(), "p1", "c3"); err != nil {
		t.Fatalf("second relocation should succeed when affordable: %v", err)
	}
	if spender.charges != 1 {
		t.Fatalf("second relocation should charge once, got %d", spender.charges)
	}
}

// countingGranter records granted resonance.
type countingGranter struct{ granted int }

func (g *countingGranter) AddSymbiontResonance(_ context.Context, _ string, delta int) (int, error) {
	g.granted += delta
	return g.granted, nil
}

func TestCollect_FloorsBufferToProfile(t *testing.T) {
	repo := &fakeRepo{live: &Nest{ID: "n1", OwnerID: "p1", Level: 1}, buffer: 7.8}
	granter := &countingGranter{}
	svc := NewService(repo, fakeCells{}, nil, granter)

	got, err := svc.Collect(context.Background(), "p1")
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if got != 7 || granter.granted != 7 {
		t.Fatalf("expected floored 7 granted, got return=%d granted=%d", got, granter.granted)
	}
}

// fakePublisher captures the last telegraph.
type fakePublisher struct {
	lastType string
	lastData map[string]any
}

func (f *fakePublisher) PublishEvent(_, eventType string, data map[string]any) {
	f.lastType = eventType
	f.lastData = data
}

func TestOnAssaultVictory_ArmsTimerTelegraphsNeverCloses(t *testing.T) {
	n := &Nest{ID: "n1", OwnerID: "p1", Level: 1, SiegeState: SiegeHealthy, SiegeHP: 100}
	repo := &fakeRepo{live: n}
	pub := &fakePublisher{}
	svc := NewService(repo, fakeCells{}, nil, nil).WithEvents(pub)

	if err := svc.OnAssaultVictory(context.Background(), "n1", "attacker"); err != nil {
		t.Fatalf("assault: %v", err)
	}
	if repo.live.SiegeState != SiegeUnderSiege {
		t.Fatalf("first assault should arm under_siege, got %s", repo.live.SiegeState)
	}
	if repo.live.CollapseAt == nil {
		t.Fatal("collapse timer must be armed")
	}
	if repo.live.CollapsedAt != nil {
		t.Fatal("assault must NEVER close the nest (only tick does)")
	}
	if pub.lastType != "nest_under_attack" {
		t.Fatalf("expected telegraph, got %q", pub.lastType)
	}
	if _, ok := pub.lastData["eta_seconds"]; !ok {
		t.Fatal("telegraph must carry eta_seconds")
	}
}

func TestRepair_CancelsCollapse(t *testing.T) {
	soon := time.Now().Add(time.Hour)
	n := &Nest{ID: "n1", OwnerID: "p1", Level: 1, SiegeState: SiegeUnderSiege, SiegeHP: 20, CollapseAt: &soon}
	repo := &fakeRepo{live: n}
	svc := NewService(repo, fakeCells{}, nil, nil)

	got, err := svc.Repair(context.Background(), "n1")
	if err != nil {
		t.Fatalf("repair: %v", err)
	}
	if got.SiegeState != SiegeHealthy || got.CollapseAt != nil {
		t.Fatalf("repair must cancel collapse: state=%s collapseAt=%v", got.SiegeState, got.CollapseAt)
	}
}

func TestPlacement_RejectedUnderForeignDome(t *testing.T) {
	repo := &fakeRepo{domed: true, pierced: false}
	svc := NewService(repo, fakeCells{}, nil, nil)
	if _, err := svc.OpenFirstNest(context.Background(), "p1", "c1"); err == nil {
		t.Fatal("placement under a foreign dome (not pierced) must be rejected")
	}
}

func TestPlacement_AllowedInPocketRecordsCell(t *testing.T) {
	repo := &fakeRepo{domed: true, pierced: true} // carved pocket under a dome
	svc := NewService(repo, fakeCells{}, nil, nil)
	if _, err := svc.OpenFirstNest(context.Background(), "p1", "c1"); err != nil {
		t.Fatalf("placement in a carved pocket should succeed: %v", err)
	}
	if !repo.pocketAdded {
		t.Fatal("a pocket placement must be recorded for tick-refresh")
	}
}

// fakeGate stubs the faction gate.
type fakeGate struct{ allow bool }

func (g fakeGate) CanOwnNest(_ context.Context, _ string) (bool, error) { return g.allow, nil }

func TestOpenFirstNest_RejectedForNonSymbiont(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo, fakeCells{}, nil, nil).WithFactionGate(fakeGate{allow: false})
	if _, err := svc.OpenFirstNest(context.Background(), "human", "c1"); err == nil {
		t.Fatal("a non-Symbiont must not be able to open a nest")
	}
	if repo.created != 0 {
		t.Fatal("no nest should have been created for a non-Symbiont")
	}
}

func TestOpenFirstNest_AllowedForSymbiont(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo, fakeCells{}, nil, nil).WithFactionGate(fakeGate{allow: true})
	if _, err := svc.OpenFirstNest(context.Background(), "symb", "c1"); err != nil {
		t.Fatalf("a committed Symbiont should open a nest: %v", err)
	}
}

func TestRepairAlly_RejectsNonSymbiont(t *testing.T) {
	repo := &fakeRepo{live: &Nest{ID: "n1", OwnerID: "ally", Level: 1, SiegeState: SiegeUnderSiege}}
	svc := NewService(repo, fakeCells{}, nil, nil).WithFactionGate(fakeGate{allow: false})
	if _, err := svc.RepairAlly(context.Background(), "human", "n1"); err == nil {
		t.Fatal("a non-Symbiont must not repair an ally nest")
	}
}

func TestRepairAlly_AllowsSymbiont(t *testing.T) {
	repo := &fakeRepo{live: &Nest{ID: "n1", OwnerID: "ally", Level: 1, SiegeState: SiegeUnderSiege, SiegeHP: 10}}
	svc := NewService(repo, fakeCells{}, nil, nil).WithFactionGate(fakeGate{allow: true})
	if _, err := svc.RepairAlly(context.Background(), "symb", "n1"); err != nil {
		t.Fatalf("a Symbiont should repair an ally nest: %v", err)
	}
}
