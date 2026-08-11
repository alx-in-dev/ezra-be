package spire

import (
	"context"
	"log/slog"
	"time"

	"github.com/ezra-game/server/internal/item"
	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/pkg/httputil"
)

// ItemStore reads/consumes the player's blueprint fragments and grants rewards.
type ItemStore interface {
	ListByPlayer(ctx context.Context, playerID string) ([]item.Item, error)
	Grant(ctx context.Context, playerID, itemType, variant string, delta int) (int, error)
}

// ResourceSpender charges build resources and refunds escrow on a failed build.
type ResourceSpender interface {
	Spend(ctx context.Context, playerID string, energy, materials int) (*player.Player, error)
	Add(ctx context.Context, playerID string, energy, materials int) (*player.Player, error)
}

type Service struct {
	repo      Repository
	items     ItemStore
	resources ResourceSpender
}

func NewService(repo Repository, items ItemStore, resources ResourceSpender) *Service {
	return &Service{repo: repo, items: items, resources: resources}
}

// ensureSpire returns the region's Spire, lazily creating it (assembling) if the
// region has none yet (like on-demand region seeding).
func (s *Service) ensureSpire(ctx context.Context, lat, lng float64) (*Spire, error) {
	key := RegionKeyFor(lat, lng)
	sp, err := s.repo.GetByRegion(ctx, key)
	if err != nil {
		return nil, err
	}
	if sp != nil {
		return sp, nil
	}
	clat, clng := RegionCenter(lat, lng)
	sp = &Spire{RegionKey: key, Lat: clat, Lng: clng, State: "assembling", RadiusM: DefaultRadiusM}
	if err := s.repo.Create(ctx, sp); err != nil {
		return nil, err
	}
	// Re-read in case of a concurrent create (ON CONFLICT kept the existing row).
	return s.repo.GetByRegion(ctx, key)
}

func (s *Service) playerFragments(ctx context.Context, playerID string) (int, error) {
	items, err := s.items.ListByPlayer(ctx, playerID)
	if err != nil {
		return 0, err
	}
	for _, it := range items {
		if it.ItemType == item.TypeBlueprintFragment {
			return it.Qty, nil
		}
	}
	return 0, nil
}

func (s *Service) status(ctx context.Context, playerID string, sp *Spire) (*Status, error) {
	frags, err := s.playerFragments(ctx, playerID)
	if err != nil {
		return nil, err
	}
	_, contributed, tier, err := s.repo.Contribution(ctx, sp.ID, playerID)
	if err != nil {
		return nil, err
	}
	return &Status{
		Spire:           sp,
		FragmentsToOpen: FragmentsToOpen,
		YourFragments:   frags,
		YourContributed: contributed,
		YourTier:        tier,
		ResourceChunk:   ResourceChunk,
	}, nil
}

func (s *Service) Status(ctx context.Context, playerID string, lat, lng float64) (*Status, error) {
	sp, err := s.ensureSpire(ctx, lat, lng)
	if err != nil {
		return nil, err
	}
	return s.status(ctx, playerID, sp)
}

// ContributeFragment turns in one blueprint fragment toward opening the build.
// The third fragment opens an escrow build window: the target is sized from the
// zone's active population (P_zone, frozen now) and a deadline is set.
// A ruined Spire is first rebuilt (reset to assembling) by the contribution.
func (s *Service) ContributeFragment(ctx context.Context, playerID string, lat, lng float64) (*Status, error) {
	sp, err := s.ensureSpire(ctx, lat, lng)
	if err != nil {
		return nil, err
	}
	if sp.State == "ruins" {
		if err := s.repo.Rebuild(ctx, sp.ID); err != nil {
			return nil, err
		}
		sp.State = "assembling"
		sp.Fragments = 0
	}
	if sp.State != "assembling" {
		return nil, httputil.NewBadRequest("not_assembling", "сбор чертежа уже завершён")
	}
	have, err := s.playerFragments(ctx, playerID)
	if err != nil {
		return nil, err
	}
	if have < 1 {
		return nil, httputil.NewBadRequest("no_fragment", "нет осколка чертежа")
	}
	if _, err := s.items.Grant(ctx, playerID, item.TypeBlueprintFragment, "", -1); err != nil {
		return nil, err
	}
	total, err := s.repo.AddFragment(ctx, sp.ID, playerID)
	if err != nil {
		return nil, err
	}
	if total >= FragmentsToOpen {
		if err := s.openBuild(ctx, sp); err != nil {
			return nil, err
		}
	}
	sp2, err := s.repo.GetByRegion(ctx, sp.RegionKey)
	if err != nil {
		return nil, err
	}
	return s.status(ctx, playerID, sp2)
}

// openBuild sizes the build target from active population (P_zone) and opens an
// escrow window with a deadline (owner decisions: P_zone + escrow).
func (s *Service) openBuild(ctx context.Context, sp *Spire) error {
	pZone, err := s.repo.CountActivePlayers(ctx, sp.Lat, sp.Lng, sp.RadiusM, ActiveWindowDays)
	if err != nil {
		// Don't block the build on a count failure — fall back to the floor.
		slog.Warn("spire P_zone count failed; using floor", "error", err, "spire", sp.ID)
		pZone = 0
	}
	target := TargetForPopulation(pZone)
	deadline := time.Now().Add(BuildWindow)
	return s.repo.OpenBuild(ctx, sp.ID, target, pZone, deadline)
}

// ContributeResources spends a fixed resource chunk (escrowed). The server (not
// the client) sets the amount. While building it fills the bar; reaching the
// target before the deadline activates the Spire and grants contribution tiers.
// While active/degrading it is upkeep (Поддержать) that revives a degrading Spire.
func (s *Service) ContributeResources(ctx context.Context, playerID string, lat, lng float64) (*Status, error) {
	sp, err := s.ensureSpire(ctx, lat, lng)
	if err != nil {
		return nil, err
	}
	if sp.State != "building" && sp.State != "active" && sp.State != "degrading" {
		return nil, httputil.NewBadRequest("not_buildable", "сейчас Шпилю нельзя вкладывать ресурсы")
	}
	if s.resources != nil {
		if _, err := s.resources.Spend(ctx, playerID, ResourceChunk, 0); err != nil {
			return nil, err
		}
	}
	if sp.State == "building" {
		progress, err := s.repo.AddResources(ctx, sp.ID, playerID, ResourceChunk)
		if err != nil {
			return nil, err
		}
		if progress >= sp.BuildTarget {
			if err := s.repo.Activate(ctx, sp.ID); err != nil {
				return nil, err
			}
			s.awardTiers(ctx, sp)
		}
	} else {
		// active / degrading → upkeep.
		if err := s.repo.Maintain(ctx, sp.ID, playerID, ResourceChunk); err != nil {
			return nil, err
		}
	}
	sp2, err := s.repo.GetByRegion(ctx, sp.RegionKey)
	if err != nil {
		return nil, err
	}
	return s.status(ctx, playerID, sp2)
}

// awardTiers grants a contribution tier (bronze/silver/gold) to every resource
// contributor on activation, by their share relative to the fair share.
func (s *Service) awardTiers(ctx context.Context, sp *Spire) {
	contribs, err := s.repo.ListContributions(ctx, sp.ID)
	if err != nil {
		slog.Warn("spire award tiers: list contributions failed", "error", err, "spire", sp.ID)
		return
	}
	for _, c := range contribs {
		tier := TierFor(c.Resources, sp.BuildTarget, sp.PZone)
		if tier == "" {
			continue
		}
		if err := s.repo.SetTier(ctx, sp.ID, c.PlayerID, tier); err != nil {
			slog.Warn("spire set tier failed", "error", err, "player", c.PlayerID)
		}
		if _, err := s.items.Grant(ctx, c.PlayerID, item.TypeSeasonTitle, "spire_"+tier, 1); err != nil {
			slog.Warn("spire grant tier title failed", "error", err, "player", c.PlayerID)
		}
	}
}

// LifecycleTick refunds escrow on builds that missed their deadline, then
// degrades/ruins active Spires whose upkeep lapsed.
func (s *Service) LifecycleTick(ctx context.Context) error {
	now := time.Now()
	expired, err := s.repo.ExpiredBuilds(ctx, now)
	if err != nil {
		slog.Warn("spire lifecycle: list expired builds failed", "error", err)
	}
	for i := range expired {
		s.refundAndReset(ctx, &expired[i])
	}
	_, err = s.repo.TransitionStale(ctx, now.Add(-DegradeAfter), now.Add(-RuinsAfter))
	return err
}

// refundAndReset returns every contributor's escrowed resources and fragments,
// then resets the Spire to assembling (escrow Variant А: nothing burned on fail).
func (s *Service) refundAndReset(ctx context.Context, sp *Spire) {
	contribs, err := s.repo.ListContributions(ctx, sp.ID)
	if err != nil {
		slog.Warn("spire refund: list contributions failed", "error", err, "spire", sp.ID)
		return
	}
	for _, c := range contribs {
		if c.Resources > 0 && s.resources != nil {
			if _, err := s.resources.Add(ctx, c.PlayerID, c.Resources, 0); err != nil {
				slog.Warn("spire refund energy failed", "error", err, "player", c.PlayerID)
			}
		}
		if c.Fragments > 0 {
			if _, err := s.items.Grant(ctx, c.PlayerID, item.TypeBlueprintFragment, "", c.Fragments); err != nil {
				slog.Warn("spire refund fragments failed", "error", err, "player", c.PlayerID)
			}
		}
	}
	if err := s.repo.ResetForRefund(ctx, sp.ID); err != nil {
		slog.Warn("spire refund reset failed", "error", err, "spire", sp.ID)
	}
}
