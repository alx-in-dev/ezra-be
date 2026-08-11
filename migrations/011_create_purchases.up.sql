CREATE TABLE purchases (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    player_id UUID NOT NULL REFERENCES players(id),
    item_type TEXT NOT NULL,
    item_id TEXT NOT NULL,
    crystals_spent INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_purchases_player ON purchases(player_id);

CREATE TABLE subscriptions (
    player_id UUID PRIMARY KEY REFERENCES players(id),
    type TEXT NOT NULL DEFAULT 'energy_pack',
    active BOOLEAN NOT NULL DEFAULT TRUE,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    receipt TEXT
);

-- Add crystals to players
ALTER TABLE players ADD COLUMN IF NOT EXISTS crystals INTEGER NOT NULL DEFAULT 0;
