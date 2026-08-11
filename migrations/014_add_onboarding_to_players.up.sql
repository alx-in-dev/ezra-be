ALTER TABLE players ADD COLUMN onboarding_step TEXT NOT NULL DEFAULT 'not_started';
ALTER TABLE players ADD COLUMN username_is_custom BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE players ADD COLUMN starter_beacon_available BOOLEAN NOT NULL DEFAULT FALSE;
