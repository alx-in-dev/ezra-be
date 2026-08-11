-- Power plants (energy pillar, T-777): a player-built generator that raises the
-- local beacon capacity, letting people play where the OSM grid is sparse
-- (off-grid solo). Anti-geography-nullification is enforced in code: a per-radius
-- station cap + spacing + the global HARD_CAP, so a barren area improves but
-- never overtakes a dense centre for free. geom mirrors lat/lng for spatial
-- queries (same pattern as towers/cores).
CREATE TABLE IF NOT EXISTS power_plants (
    id             UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    owner_id       UUID        NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    lat            DOUBLE PRECISION NOT NULL,
    lng            DOUBLE PRECISION NOT NULL,
    geom           GEOMETRY(Point, 4326) NOT NULL,
    capacity_bonus INTEGER     NOT NULL DEFAULT 2,
    state          TEXT        NOT NULL DEFAULT 'active' CHECK (state IN ('active', 'degrading', 'ruins')),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_power_plants_geog ON power_plants USING GIST ((geom::geography));
CREATE INDEX IF NOT EXISTS idx_power_plants_owner ON power_plants (owner_id);
