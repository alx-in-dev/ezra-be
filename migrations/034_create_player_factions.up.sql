-- E3 faction onboarding (faction_onboarding.md): everyone starts Human; the
-- choice is earned via "Contact" (collapsing a Hive — reaching the Эзра source).
-- Separate table to avoid touching the players scan paths.
CREATE TABLE IF NOT EXISTS player_factions (
    player_id  UUID        NOT NULL PRIMARY KEY REFERENCES players(id) ON DELETE CASCADE,
    faction    TEXT        NOT NULL DEFAULT 'human',   -- 'human' | 'symbiont'
    contacted  BOOLEAN     NOT NULL DEFAULT false,     -- has experienced Contact (choice unlocked)
    chosen     BOOLEAN     NOT NULL DEFAULT false,     -- has made an explicit allegiance choice
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
