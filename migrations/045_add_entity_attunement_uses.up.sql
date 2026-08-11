-- Symbiont entity Attunement progression (R4-2 roster, L3). Attunement grows
-- through USE, not passively over time: each successful autonomous tick on a
-- live target counts as one use, accumulating toward the next rank (I→II→III).
-- The rank itself lives in symbiont_entities.attunement (added in 044); this
-- column tracks the cumulative uses that drive it.
ALTER TABLE symbiont_entities ADD COLUMN IF NOT EXISTS attunement_uses INTEGER NOT NULL DEFAULT 0;
