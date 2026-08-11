-- Distinguish sealed-dome (closed-contour area) cells from radius/disk-field
-- cells so area income (B3) is credited only for sealed coverage — no double
-- counting against per-beacon passive income. See beacon_network_dome.md §6.
ALTER TABLE domed_cells ADD COLUMN sealed BOOLEAN NOT NULL DEFAULT false;
