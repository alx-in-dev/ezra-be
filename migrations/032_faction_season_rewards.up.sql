-- Faction War season settlement (R4-5 weekly rewards): records which seasons
-- have been paid out (idempotency) and each player's reward for a season.
CREATE TABLE IF NOT EXISTS faction_season_settlements (
    season     TEXT        NOT NULL PRIMARY KEY,
    settled_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS faction_season_rewards (
    player_id  UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    season     TEXT NOT NULL,
    rank       INT  NOT NULL,
    points     INT  NOT NULL,
    crystals   INT  NOT NULL,
    claimed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (player_id, season)
);
