-- Achievements (retention, roadmap #9): records which milestone achievements a
-- player has unlocked, so the one-time reward + toast fire exactly once. Current
-- progress itself is derived live from existing tables (towers, battles,
-- player_items, cores, players) — only the unlock event is persisted here.
CREATE TABLE IF NOT EXISTS player_achievements (
    player_id      UUID        NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    achievement_id TEXT        NOT NULL,
    unlocked_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (player_id, achievement_id)
);
