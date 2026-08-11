-- E1: the Symbiont "Очаг" (Hive) — the mirror of the human Core. It is the
-- root of an infection region: it pulses infection into its radius and spawns
-- rifts in its zone (the source the D2 tide channels toward player networks).
-- Closing it (future E2/PvP) collapses the region's infection source.
CREATE TABLE IF NOT EXISTS hives (
    id         UUID            NOT NULL PRIMARY KEY DEFAULT uuid_generate_v4(),
    cell_id    TEXT            NOT NULL REFERENCES cells(id),
    lat        DOUBLE PRECISION NOT NULL,
    lng        DOUBLE PRECISION NOT NULL,
    geom       GEOMETRY(Point, 4326) NOT NULL,
    level      INT             NOT NULL DEFAULT 1,
    intensity  INT             NOT NULL DEFAULT 100, -- 0..100 vitality; drops as the region is cleared
    created_at TIMESTAMPTZ     NOT NULL DEFAULT now(),
    closed_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_hives_geog ON hives USING GIST ((geom::geography));
CREATE INDEX IF NOT EXISTS idx_hives_open ON hives (closed_at) WHERE closed_at IS NULL;
