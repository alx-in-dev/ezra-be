CREATE TABLE units (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    player_id UUID NOT NULL REFERENCES players(id),
    type TEXT NOT NULL CHECK (type IN ('fighter', 'engineer', 'scout', 'medic')),
    level INTEGER NOT NULL DEFAULT 1,
    xp INTEGER NOT NULL DEFAULT 0,
    hp INTEGER NOT NULL DEFAULT 50,
    atk INTEGER NOT NULL DEFAULT 10,
    squad_id UUID,
    status TEXT NOT NULL DEFAULT 'idle' CHECK (status IN ('idle', 'on_mission', 'lost'))
);
CREATE INDEX idx_units_player ON units(player_id);
CREATE INDEX idx_units_squad ON units(squad_id) WHERE squad_id IS NOT NULL;
