-- Ephemeral Hearth (T-805, symbiont_geo_playstyle.md §9): a temporary Symbiont
-- muster point for a coordinated raid on one dome. It buffs Symbionts within
-- range while active, then expires on its own (TTL) — NOT a stationary base
-- or a graph root (RED LINE #1 stays intact). cap=1 active + a cooldown per
-- owner are enforced in code via (owner_id, created_at).
CREATE TABLE IF NOT EXISTS ephemeral_hearths (
    id         UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    owner_id   UUID        NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    lat        DOUBLE PRECISION NOT NULL,
    lng        DOUBLE PRECISION NOT NULL,
    geom       GEOMETRY(Point, 4326) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ephemeral_hearths_geog ON ephemeral_hearths USING GIST ((geom::geography));
CREATE INDEX IF NOT EXISTS idx_ephemeral_hearths_owner_created ON ephemeral_hearths (owner_id, created_at DESC);
