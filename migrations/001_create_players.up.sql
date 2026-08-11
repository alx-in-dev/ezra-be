CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "postgis";

CREATE TABLE players (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    firebase_uid TEXT UNIQUE NOT NULL,
    username TEXT UNIQUE NOT NULL,
    level INTEGER NOT NULL DEFAULT 1,
    xp INTEGER NOT NULL DEFAULT 0,
    energy INTEGER NOT NULL DEFAULT 200,
    materials INTEGER NOT NULL DEFAULT 50,
    army_limit INTEGER NOT NULL DEFAULT 50,
    skills JSONB NOT NULL DEFAULT '{"defender": 0, "commander": 0, "energist": 0}',
    position JSONB NOT NULL DEFAULT '{"lat": 0, "lng": 0, "updated_at": "2026-01-01T00:00:00Z"}',
    last_active TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
