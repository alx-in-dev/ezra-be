-- Add proximity-to-infrastructure flags for tower placement validation.
-- Populated from Mapbox Tilequery during seed, used by tower.Service.Place.
-- Replaces strict "cell.terrain in {road, building}" rule with "has building
-- or road within 30m". Lore: towers need electricity, which is only available
-- near buildings and roads (power lines, street lights, substations).

ALTER TABLE cells
    ADD COLUMN has_nearby_building BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN has_nearby_road     BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN nearest_road_class  TEXT;

-- Backfill for existing rows (pre-Tilequery seed). Old `terrain` was random
-- but at least gives SOME approximation until re-seed runs. Re-seed with
-- Mapbox data is strongly recommended after this migration.
UPDATE cells
SET
    has_nearby_building = (terrain = 'building'),
    has_nearby_road     = (terrain = 'road');

CREATE INDEX idx_cells_power_source ON cells (has_nearby_building, has_nearby_road)
    WHERE has_nearby_building OR has_nearby_road;
