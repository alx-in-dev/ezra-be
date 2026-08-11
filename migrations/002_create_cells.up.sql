CREATE TABLE cells (
    id TEXT PRIMARY KEY,
    lat DOUBLE PRECISION NOT NULL,
    lng DOUBLE PRECISION NOT NULL,
    infection REAL NOT NULL DEFAULT 0,
    terrain TEXT NOT NULL DEFAULT 'open' CHECK (terrain IN ('road', 'building', 'open')),
    tower_id UUID,
    rift_id UUID,
    geom GEOMETRY(Point, 4326) NOT NULL,
    last_calculated TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_cells_geom ON cells USING GIST (geom);
CREATE INDEX idx_cells_infection ON cells (infection) WHERE infection > 0;
