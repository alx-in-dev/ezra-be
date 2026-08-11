-- Baseless Symbiont progression (#6, T-763): Resonance is a PORTABLE stat on the
-- player (not tied to a cell or graph) — it can't be destroyed by removing a
-- base because the Symbiont has none. Increments from field actions (dome breach,
-- hive empower, …). Non-transferable like the other progression stats.
ALTER TABLE players ADD COLUMN IF NOT EXISTS symbiont_resonance INTEGER NOT NULL DEFAULT 0;
