CREATE TABLE squads (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    player_id UUID NOT NULL REFERENCES players(id),
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'idle' CHECK (status IN ('idle', 'on_mission')),
    mission JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_squads_player ON squads(player_id);
ALTER TABLE units ADD CONSTRAINT fk_units_squad FOREIGN KEY (squad_id) REFERENCES squads(id) ON DELETE SET NULL;
