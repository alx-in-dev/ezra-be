-- Symbiont Resonance progression (R4-2 roster, L1). Three columns on the player:
--   resonance_pool  — one-time conversion of human progression at the moment the
--                     player sides with the Symbionts (units/beacons/pets), the
--                     "резонансный след" that seeds the starter roster. Set once.
--   resonance_level — RL1..RL5: control cap + verb/archetype unlocks (derived
--                     from resonance_xp via canon thresholds; cached here).
--   resonance_xp    — accrues from symbiont field actions (overload/breach/raise)
--                     and drives the Resonance Level.
-- Distinct from symbiont_resonance (the portable spendable currency, migr 037).
ALTER TABLE players ADD COLUMN IF NOT EXISTS resonance_pool  INTEGER NOT NULL DEFAULT 0;
ALTER TABLE players ADD COLUMN IF NOT EXISTS resonance_level INTEGER NOT NULL DEFAULT 1;
ALTER TABLE players ADD COLUMN IF NOT EXISTS resonance_xp    INTEGER NOT NULL DEFAULT 0;
