-- Spire rewrite to owner decisions (megastructure_spire.md): escrow build window
-- + population-derived requirement (P_zone) + contribution tiers.
-- build_deadline: escrow window end; if target not met by then → refund + reset.
-- p_zone: active-population snapshot frozen when the build opens (drives target).
-- tier: per-player contribution recognition, set on activation.
ALTER TABLE spires ADD COLUMN IF NOT EXISTS build_deadline TIMESTAMPTZ;
ALTER TABLE spires ADD COLUMN IF NOT EXISTS p_zone INT NOT NULL DEFAULT 0;
ALTER TABLE spire_contributions ADD COLUMN IF NOT EXISTS tier TEXT;
