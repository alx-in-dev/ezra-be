DROP INDEX IF EXISTS idx_cells_power_source;

ALTER TABLE cells
    DROP COLUMN IF EXISTS has_nearby_building,
    DROP COLUMN IF EXISTS has_nearby_road,
    DROP COLUMN IF EXISTS nearest_road_class;
