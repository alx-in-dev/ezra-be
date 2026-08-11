CREATE TABLE battles (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    attacker_id UUID NOT NULL REFERENCES players(id),
    squad_id UUID NOT NULL REFERENCES squads(id),
    target_type TEXT NOT NULL CHECK (target_type IN ('rift', 'tower')),
    target_id UUID NOT NULL,
    rounds JSONB NOT NULL DEFAULT '[]',
    status TEXT NOT NULL DEFAULT 'ongoing' CHECK (status IN ('ongoing', 'won', 'lost')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_battles_attacker ON battles(attacker_id);
