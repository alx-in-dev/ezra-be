-- Dome pierced pocket (#6, T-762): a Symbiont who raises a domed cell's
-- infection past the pierce threshold punches a temporary hole in the dome —
-- while pierced_until is in the future the dome stops suppressing that cell, so
-- the pocket holds and the soft-drain lifts. The owner reclaims it when the TTL
-- lapses (the dome reasserts). No polygon/graph recompute — just this timestamp.
ALTER TABLE cells ADD COLUMN IF NOT EXISTS pierced_until TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_cells_pierced_until ON cells (pierced_until) WHERE pierced_until IS NOT NULL;
