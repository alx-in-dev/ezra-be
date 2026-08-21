-- No-op: idx_towers_geog is owned by migration 023 (023_towers_free_placement).
-- This migration only ensures its existence; dropping it here would remove an
-- index another migration created, so the down leaves it in place.
SELECT 1;
