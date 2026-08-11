DROP INDEX IF EXISTS idx_cells_pierced_until;
ALTER TABLE cells DROP COLUMN IF EXISTS pierced_until;
