-- Spire lifecycle (megastructure_spire.md §3): an active Spire must be kept up
-- by ongoing regional contribution. last_activity_at tracks the most recent
-- upkeep; the lifecycle job degrades/ruins a Spire that goes stale.
ALTER TABLE spires ADD COLUMN IF NOT EXISTS last_activity_at TIMESTAMPTZ NOT NULL DEFAULT now();
