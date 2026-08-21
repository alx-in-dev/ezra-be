-- N2 (spirit_world_and_symbiont_nest.md §4): wild spirits — the wandering
-- энергетические существа from the lore. One element, three roles: a threat to
-- humans (wave-drain their beacons), the Symbiont's army (tamed → roster), and
-- the visible embodiment of "духи среди нас". Modelled as "рейсы" (N0 verdict):
-- a spirit flies origin(source)→dest at a fixed speed; its live position is
-- INTERPOLATED from (spawn_ts, speed_mps, dist_m, now) at read time — zero
-- per-tick position writes. The tick only handles discrete events (arrival,
-- expiry). See architecture ADR-N2 / perf_spike_N0.md §6.
CREATE TABLE IF NOT EXISTS wild_spirits (
    id             UUID             NOT NULL PRIMARY KEY DEFAULT uuid_generate_v4(),
    class          INT              NOT NULL DEFAULT 1,       -- I..V (1..5); higher = rarer/stronger

    origin_lat     DOUBLE PRECISION NOT NULL,
    origin_lng     DOUBLE PRECISION NOT NULL,
    origin_geom    GEOMETRY(Point, 4326) NOT NULL,
    dest_lat       DOUBLE PRECISION NOT NULL,
    dest_lng       DOUBLE PRECISION NOT NULL,
    dest_geom      GEOMETRY(Point, 4326) NOT NULL,

    spawn_ts       TIMESTAMPTZ      NOT NULL DEFAULT now(),
    speed_mps      DOUBLE PRECISION NOT NULL,                 -- metres/second along the route
    dist_m         DOUBLE PRECISION NOT NULL,                 -- origin→dest distance
    arrive_at      TIMESTAMPTZ      NOT NULL,                 -- spawn_ts + dist/speed (event time)

    -- Lifecycle: wandering (in flight) → weakened (a Symbiont softened it for
    -- taming) → tamed (moved into the roster) / expired (arrived & discharged, or
    -- timed out). Discrete transitions only.
    state          TEXT             NOT NULL DEFAULT 'wandering'
                       CHECK (state IN ('wandering','weakened','tamed','expired')),
    weakened_pct   DOUBLE PRECISION NOT NULL DEFAULT 0,       -- 0..100 progress toward tameable
    tamed_by       UUID             REFERENCES players(id),

    -- The human beacon this spirit is inbound to (wave-drain target). NULL for a
    -- pure wander spirit (atmosphere only).
    target_tower_id UUID,

    -- Regional bucket for the population budget (mirror of the source budget).
    region_key     TEXT,

    created_at     TIMESTAMPTZ      NOT NULL DEFAULT now()
);

-- Spatial: nearby lookups (map + proximity/touch) by geography GiST on both ends.
CREATE INDEX IF NOT EXISTS idx_wild_spirits_origin_geog ON wild_spirits USING GIST ((origin_geom::geography));
CREATE INDEX IF NOT EXISTS idx_wild_spirits_dest_geog ON wild_spirits USING GIST ((dest_geom::geography));
-- The tick scans live (wandering/weakened) spirits and due arrivals.
CREATE INDEX IF NOT EXISTS idx_wild_spirits_live ON wild_spirits (state, arrive_at)
    WHERE state IN ('wandering','weakened');
CREATE INDEX IF NOT EXISTS idx_wild_spirits_region ON wild_spirits (region_key)
    WHERE state IN ('wandering','weakened');
