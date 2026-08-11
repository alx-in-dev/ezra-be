-- Civic megastructure "Spire" (megastructure_spire.md, MVP slice). A regional
-- counter/flag layer ABOVE personal beacon networks — NOT a graph node: it never
-- enters the rooting tree and its state never gates anyone's network. When active
-- it grants extra infection suppression to all cores within its radius.
CREATE TABLE IF NOT EXISTS spires (
    id             UUID        NOT NULL PRIMARY KEY DEFAULT uuid_generate_v4(),
    region_key     TEXT        NOT NULL UNIQUE,   -- coarse grid key dedups one spire per region
    lat            DOUBLE PRECISION NOT NULL,
    lng            DOUBLE PRECISION NOT NULL,
    geom           GEOMETRY(Point, 4326) NOT NULL,
    state          TEXT        NOT NULL DEFAULT 'assembling', -- assembling | building | active
    fragments      INT         NOT NULL DEFAULT 0,  -- blueprint fragments turned in (need 3)
    build_progress INT         NOT NULL DEFAULT 0,  -- resources contributed
    build_target   INT         NOT NULL DEFAULT 6000,
    radius_m       INT         NOT NULL DEFAULT 10000,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    activated_at   TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_spires_geog ON spires USING GIST ((geom::geography));
CREATE INDEX IF NOT EXISTS idx_spires_active ON spires (state) WHERE state = 'active';

-- Per-player contribution log (drives contribution tiers / recognition).
CREATE TABLE IF NOT EXISTS spire_contributions (
    spire_id   UUID NOT NULL REFERENCES spires(id) ON DELETE CASCADE,
    player_id  UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    fragments  INT  NOT NULL DEFAULT 0,
    resources  INT  NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (spire_id, player_id)
);
