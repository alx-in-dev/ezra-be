-- Symbiont entity roster (R4-2 roster, L2). The Resonance Pool materializes a
-- small set of bound entities; each is assigned to a hostile beacon or a rift
-- and acts autonomously on a server timer.
--   archetype   — assault | distortion | scout | absorber (canon archetypes).
--   attunement  — 1..3 quality rank (grows via use; L3 layer, default rank I).
--   status      — available | manifesting | assigned | recovering (state machine).
--   target_kind — tower | rift (NULL when unassigned).
--   target_id   — the assigned beacon / rift id (NULL when unassigned).
--   manifest_at — when manifesting completes → assigned (NULL otherwise).
--   recovery_at — when recovering completes → available (NULL otherwise).
CREATE TABLE IF NOT EXISTS symbiont_entities (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    player_id   UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    archetype   TEXT NOT NULL,
    attunement  INTEGER NOT NULL DEFAULT 1,
    status      TEXT NOT NULL DEFAULT 'available',
    target_kind TEXT,
    target_id   TEXT,
    manifest_at TIMESTAMPTZ,
    recovery_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_symbiont_entities_player ON symbiont_entities (player_id);
CREATE INDEX IF NOT EXISTS idx_symbiont_entities_status ON symbiont_entities (status);
