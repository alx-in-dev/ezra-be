CREATE TABLE daily_quests (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    player_id UUID NOT NULL REFERENCES players(id),
    type TEXT NOT NULL,
    description TEXT NOT NULL,
    progress INTEGER NOT NULL DEFAULT 0,
    target INTEGER NOT NULL,
    reward JSONB NOT NULL DEFAULT '{}',
    date DATE NOT NULL DEFAULT CURRENT_DATE,
    claimed BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE INDEX idx_quests_player_date ON daily_quests(player_id, date);

CREATE TABLE streaks (
    player_id UUID PRIMARY KEY REFERENCES players(id),
    days INTEGER NOT NULL DEFAULT 0,
    last_checkin DATE
);
