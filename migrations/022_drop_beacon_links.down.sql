CREATE TABLE beacon_links (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    player_id  UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    from_id    TEXT NOT NULL,
    to_id      TEXT NOT NULL,
    length_m   DOUBLE PRECISION NOT NULL,
    cost       INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (from_id, to_id)
);
CREATE INDEX idx_beacon_links_player ON beacon_links (player_id);
