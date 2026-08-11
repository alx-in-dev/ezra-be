-- Power-plant lifecycle (T-778): stations decay without upkeep. last_upkeep_at
-- drives the active→degrading→ruins worker; building or servicing a plant resets
-- it. Ruins contribute no capacity (StationBonusInRadius sums only 'active').
ALTER TABLE power_plants ADD COLUMN IF NOT EXISTS last_upkeep_at TIMESTAMPTZ NOT NULL DEFAULT now();
