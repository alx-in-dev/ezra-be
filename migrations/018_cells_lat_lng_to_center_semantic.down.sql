-- Reverse migration 018: shift cells.lat/lng back from center to SW corner.
-- WARNING: this is lossy if any cells were created after 018 at grid
-- positions that don't exist in the old (pre-018) semantic. Fresh data
-- will be renumbered to "whatever minus 25m" which may not match any
-- canonical grid anchor.
--
-- Typical operator use: only run after an up-018 rollback window when the
-- table is still empty or contains only cells inserted while 018 was live.

UPDATE cells SET
    lat = lat - (25.0 / 111000.0),
    lng = lng - (25.0 / (111000.0 * COS(RADIANS(lat)))),
    geom = ST_SetSRID(ST_MakePoint(
        lng - (25.0 / (111000.0 * COS(RADIANS(lat)))),
        lat - (25.0 / 111000.0)
    ), 4326);
