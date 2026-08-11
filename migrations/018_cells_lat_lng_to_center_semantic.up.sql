-- Shift cells.lat/lng semantic from "SW corner of 50m grid square" to
-- "geographic center" (decision C-02 from ai/review.md 2026-04-11).
--
-- Background: historical seed code (cmd/seed and original cell.Seeder) wrote
-- cell.lat/lng as the SW-corner grid intersect from the enumerator loop.
-- All downstream users (tower.Service.Place haversine, client rendering,
-- infection recalc) were treating it as "where the cell is" which implicitly
-- means its center. Max drift: ~35m diagonal. Concrete user-visible symptom:
-- a player standing exactly in the geographic center of a cell gets
-- "too_far" on tower placement because haversine(player, cellSwCorner) > 30m.
--
-- This migration updates all existing rows in place by +halfStepLat
-- (25 / 111000 ≈ 0.000225°) and +halfStepLng (25 / (111000 * cos(lat))).
-- The ID column is NOT touched — IDs remain derived from the SW corner
-- which keeps them stable as opaque identifiers across the semantic change.
-- The geom column is regenerated to match the new lat/lng so PostGIS
-- spatial queries (ST_DWithin, ST_Within) see the same coords.
--
-- PostgreSQL evaluates all SET expressions on the OLD row state before
-- applying any, so the geom expression referring to lat/lng uses
-- pre-update values — which is what we want (base old-SW-corner + offset).
--
-- After this migration, new cells inserted by cell.Seeder.bulkInsertCells
-- must be written with CENTER semantics (see seeder.go change in the same
-- commit). The two halves (data migration + code change) must ship
-- together.

UPDATE cells SET
    lat = lat + (25.0 / 111000.0),
    lng = lng + (25.0 / (111000.0 * COS(RADIANS(lat)))),
    geom = ST_SetSRID(ST_MakePoint(
        lng + (25.0 / (111000.0 * COS(RADIANS(lat)))),
        lat + (25.0 / 111000.0)
    ), 4326);
