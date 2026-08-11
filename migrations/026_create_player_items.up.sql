-- D5: persistent player inventory for epic-loot keys/items and the resonance
-- signatures dropped by critical-rift (III) shard bosses. The (type, variant)
-- shape lets a fungible key (variant='') and a collectible signature spectrum
-- (variant='ember' etc.) share one table; D1 reads the signature set from here.
CREATE TABLE IF NOT EXISTS player_items (
    player_id  UUID        NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    item_type  TEXT        NOT NULL,
    variant    TEXT        NOT NULL DEFAULT '',
    qty        INTEGER     NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (player_id, item_type, variant)
);

CREATE INDEX IF NOT EXISTS idx_player_items_player ON player_items (player_id);
