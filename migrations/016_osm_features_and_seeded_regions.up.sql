-- Lazy region seed infrastructure (decision: lazy on-demand, Overpass-driven).
--
-- When a player first hits /map/cells in a region that hasn't been seeded,
-- the server calls Overpass API for OSM buildings + roads in a 0.1° grid
-- cell, stores the raw features in osm_features, generates the 50m cell grid,
-- and populates has_nearby_building / has_nearby_road from the new features
-- via ST_DWithin. seeded_regions tracks which 0.1° grid cells have been
-- processed so we never hit Overpass twice for the same region.

-- Raw OSM features (buildings + filtered highways) retrieved from Overpass.
-- Source of truth for tower placement validation's proximity checks.
CREATE TABLE osm_features (
    osm_id     BIGINT      NOT NULL,
    type       TEXT        NOT NULL CHECK (type IN ('building','road')),
    road_class TEXT,        -- only for type='road': motorway/primary/residential/...
    geom       GEOMETRY    NOT NULL,  -- Polygon for building, LineString for road, SRID 4326
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (osm_id, type)
);

CREATE INDEX idx_osm_features_geom ON osm_features USING GIST (geom);
CREATE INDEX idx_osm_features_type ON osm_features (type);

-- Regions already processed by EnsureRegion. Key format: "grid_{latIdx}_{lngIdx}"
-- where latIdx = floor(lat*10), lngIdx = floor(lng*10), i.e. 0.1° grid cells.
-- Used both as a dedup cache (skip Overpass if region already seeded) and as
-- a statistics table for admin/ops visibility.
CREATE TABLE seeded_regions (
    key            TEXT             PRIMARY KEY,
    center_lat     DOUBLE PRECISION NOT NULL,
    center_lng     DOUBLE PRECISION NOT NULL,
    bbox           GEOMETRY         NOT NULL,  -- Polygon, SRID 4326
    cell_count     INT              NOT NULL DEFAULT 0,
    feature_count  INT              NOT NULL DEFAULT 0,
    seeded_at      TIMESTAMPTZ      NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_seeded_regions_bbox ON seeded_regions USING GIST (bbox);
