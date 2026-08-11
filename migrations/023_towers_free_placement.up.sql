-- Free (off-grid) beacon placement: a tower stands at the exact point where
-- the player stood, not at its cell's centre. cell_id remains as the anchor
-- to the infection simulation grid (pressure/suppression bookkeeping).
ALTER TABLE towers
    ADD COLUMN lat  DOUBLE PRECISION,
    ADD COLUMN lng  DOUBLE PRECISION,
    ADD COLUMN geom GEOMETRY(Point, 4326);

UPDATE towers t
SET lat  = c.lat,
    lng  = c.lng,
    geom = ST_SetSRID(ST_MakePoint(c.lng, c.lat), 4326)
FROM cells c
WHERE c.id = t.cell_id;

ALTER TABLE towers
    ALTER COLUMN lat  SET NOT NULL,
    ALTER COLUMN lng  SET NOT NULL,
    ALTER COLUMN geom SET NOT NULL;

CREATE INDEX idx_towers_geog ON towers USING GIST ((geom::geography));
