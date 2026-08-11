-- Bestiary (retention, T-660): records which encounter-based catalog entries a
-- player has discovered (spirit & rift types — seen in battle). Tower types and
-- resonance spectra are derived live from existing tables, so only the
-- encounter discoveries need persisting here.
CREATE TABLE IF NOT EXISTS player_discoveries (
    player_id     UUID        NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    discovery_key TEXT        NOT NULL,
    discovered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (player_id, discovery_key)
);
