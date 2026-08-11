CREATE TABLE pets (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    player_id UUID NOT NULL REFERENCES players(id),
    name TEXT NOT NULL DEFAULT 'Scout',
    type TEXT NOT NULL DEFAULT 'scout' CHECK (type IN ('scout', 'guardian', 'energy')),
    level INTEGER NOT NULL DEFAULT 1,
    xp INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'idle' CHECK (status IN ('idle', 'on_task', 'evolved')),
    task JSONB
);
CREATE INDEX idx_pets_player ON pets(player_id);
