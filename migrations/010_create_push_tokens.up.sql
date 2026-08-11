CREATE TABLE push_tokens (
    player_id UUID PRIMARY KEY REFERENCES players(id),
    fcm_token TEXT NOT NULL,
    platform  TEXT NOT NULL DEFAULT 'android',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
