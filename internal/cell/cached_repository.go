package cell

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const cellCacheTTL = 30 * time.Second

// CachedRepository wraps a Repository with Redis caching.
type CachedRepository struct {
	inner Repository
	rdb   *redis.Client
}

// NewCachedRepository creates a cache-through wrapper around a cell Repository.
func NewCachedRepository(repo Repository, rdb *redis.Client) *CachedRepository {
	return &CachedRepository{inner: repo, rdb: rdb}
}

func (c *CachedRepository) GetAll(ctx context.Context) ([]Cell, error) {
	return c.inner.GetAll(ctx)
}

func (c *CachedRepository) GetInRadius(ctx context.Context, lat, lng, radiusM float64) ([]Cell, error) {
	key := fmt.Sprintf("cells:radius:%.6f:%.6f:%.0f", lat, lng, radiusM)

	data, err := c.rdb.Get(ctx, key).Bytes()
	if err == nil {
		var cells []Cell
		if json.Unmarshal(data, &cells) == nil {
			return cells, nil
		}
	}

	cells, err := c.inner.GetInRadius(ctx, lat, lng, radiusM)
	if err != nil {
		return nil, err
	}

	if encoded, err := json.Marshal(cells); err == nil {
		c.rdb.Set(ctx, key, encoded, cellCacheTTL)
	}
	return cells, nil
}

func (c *CachedRepository) GetByID(ctx context.Context, id string) (*Cell, error) {
	key := "cell:" + id

	data, err := c.rdb.Get(ctx, key).Bytes()
	if err == nil {
		var cell Cell
		if json.Unmarshal(data, &cell) == nil {
			return &cell, nil
		}
	}

	cell, err := c.inner.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if encoded, err := json.Marshal(cell); err == nil {
		c.rdb.Set(ctx, key, encoded, cellCacheTTL)
	}
	return cell, nil
}

func (c *CachedRepository) GetNeighbors(ctx context.Context, cellID string) ([]Cell, error) {
	key := "cell:neighbors:" + cellID

	data, err := c.rdb.Get(ctx, key).Bytes()
	if err == nil {
		var cells []Cell
		if json.Unmarshal(data, &cells) == nil {
			return cells, nil
		}
	}

	cells, err := c.inner.GetNeighbors(ctx, cellID)
	if err != nil {
		return nil, err
	}

	if encoded, err := json.Marshal(cells); err == nil {
		c.rdb.Set(ctx, key, encoded, cellCacheTTL)
	}
	return cells, nil
}

func (c *CachedRepository) Update(ctx context.Context, id string, infection float64) error {
	// Invalidate cache on write
	c.rdb.Del(ctx, "cell:"+id)
	return c.inner.Update(ctx, id, infection)
}

// CountBuildingCellsInRadius delegates to the inner repository when it supports
// the optional counter interface (the production PgRepository does). Returns 0
// when unsupported so callers fall back to the flat capacity.
func (c *CachedRepository) CountBuildingCellsInRadius(ctx context.Context, lat, lng, radiusM float64) (int, error) {
	type counter interface {
		CountBuildingCellsInRadius(ctx context.Context, lat, lng, radiusM float64) (int, error)
	}
	cc, ok := c.inner.(counter)
	if !ok {
		return 0, nil
	}
	return cc.CountBuildingCellsInRadius(ctx, lat, lng, radiusM)
}

// RaiseInfectionInRadius delegates to the inner repository when it supports the
// optional raiser interface (the production PgRepository does). Like
// ClearInfectionInRadius, this skips per-cell cache invalidation — the radius
// and by-ID caches (cellCacheTTL) go stale for at most 30s, which an hourly
// rift tick tolerates fine.
func (c *CachedRepository) RaiseInfectionInRadius(ctx context.Context, lat, lng, radiusM, delta float64) (int64, error) {
	type raiser interface {
		RaiseInfectionInRadius(ctx context.Context, lat, lng, radiusM, delta float64) (int64, error)
	}
	rr, ok := c.inner.(raiser)
	if !ok {
		return 0, nil
	}
	return rr.RaiseInfectionInRadius(ctx, lat, lng, radiusM, delta)
}

// ClearInfectionInRadius delegates to the inner repository when it supports the
// optional clearer interface (the production PgRepository does). The radius cell
// cache is short-lived (cellCacheTTL); a one-time soft-start clear tolerates it.
func (c *CachedRepository) ClearInfectionInRadius(ctx context.Context, lat, lng, radiusM, target float64) (int64, error) {
	type clearer interface {
		ClearInfectionInRadius(ctx context.Context, lat, lng, radiusM, target float64) (int64, error)
	}
	cl, ok := c.inner.(clearer)
	if !ok {
		return 0, nil
	}
	return cl.ClearInfectionInRadius(ctx, lat, lng, radiusM, target)
}

func (c *CachedRepository) SetTowerID(ctx context.Context, cellID string, towerID *string) error {
	c.rdb.Del(ctx, "cell:"+cellID)
	return c.inner.SetTowerID(ctx, cellID, towerID)
}

func (c *CachedRepository) SetRiftID(ctx context.Context, cellID string, riftID *string) error {
	c.rdb.Del(ctx, "cell:"+cellID)
	return c.inner.SetRiftID(ctx, cellID, riftID)
}
