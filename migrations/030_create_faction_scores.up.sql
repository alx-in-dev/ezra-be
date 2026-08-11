-- E2 (Faction War slice): per-player Human contribution to the faction war,
-- partitioned by weekly season. The regional Human-vs-Symbiont balance itself is
-- computed live from world state; this table tracks who contributed how much.
CREATE TABLE IF NOT EXISTS faction_scores (
    player_id  UUID        NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    season     TEXT        NOT NULL,
    points     INTEGER     NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (player_id, season)
);

CREATE INDEX IF NOT EXISTS idx_faction_scores_season ON faction_scores (season, points DESC);
