-- City Link & social (R4-4b, legacy_human_network.md / faction_war social layer).
-- A City Link is a mutual-support alliance between two players: they can repair
-- each other's beacons (incl. legacy networks) and see each other's defense
-- alerts. NO network/Ecap/income merge (owner decision 2026-06-17).
--
-- city_links stores each pair once, canonicalized player_low < player_high.
CREATE TABLE IF NOT EXISTS city_links (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    player_low  UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    player_high UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (player_low, player_high),
    CHECK (player_low < player_high)
);

-- link_requests is the consent flow: from_player asks to_player to link.
CREATE TABLE IF NOT EXISTS link_requests (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    from_player UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    to_player   UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    state       TEXT NOT NULL DEFAULT 'pending', -- pending | accepted | rejected | cancelled
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One pending request per ordered pair (re-asking is idempotent at the app layer).
CREATE UNIQUE INDEX IF NOT EXISTS idx_link_requests_pending_pair
    ON link_requests (from_player, to_player) WHERE state = 'pending';
CREATE INDEX IF NOT EXISTS idx_link_requests_inbox
    ON link_requests (to_player) WHERE state = 'pending';
