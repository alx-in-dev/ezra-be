-- D1: the resonance counter-wave permanently stabilizes a region. Stabilized
-- cells have their infection forced to 0 by the infection recalculation and
-- never regrow (a permanent, world-visible cleanse — the film's "counter-wave"
-- endgame payoff). Distinct from domed_cells (player-network suppression that
-- only decays infection while the dome holds).
CREATE TABLE IF NOT EXISTS stabilized_cells (
    cell_id    TEXT        NOT NULL PRIMARY KEY REFERENCES cells(id) ON DELETE CASCADE,
    player_id  UUID        REFERENCES players(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_stabilized_cells_player ON stabilized_cells (player_id);

-- Each launched counter-wave, for attribution / the "Resonance" seasonal title
-- (a player's wave count is their standing in the endgame).
CREATE TABLE IF NOT EXISTS resonance_waves (
    id         BIGSERIAL   PRIMARY KEY,
    player_id  UUID        NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    lat        DOUBLE PRECISION NOT NULL,
    lng        DOUBLE PRECISION NOT NULL,
    radius_m   INTEGER     NOT NULL,
    cells_stabilized INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_resonance_waves_player ON resonance_waves (player_id);
