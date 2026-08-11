-- Beacon network / dome (Perimeter × Ingress). See docs/feature/beacon_network_dome.md.
-- Personal Core = root of the energy network; beacon_links = auto-formed edges
-- between nodes (core or towers); domed_cells = cells enclosed by a player's
-- network loop (set by network recompute, read by the infection job).

CREATE TABLE cores (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    player_id    UUID NOT NULL UNIQUE REFERENCES players(id) ON DELETE CASCADE,
    cell_id      TEXT NOT NULL REFERENCES cells(id),
    lat          DOUBLE PRECISION NOT NULL,
    lng          DOUBLE PRECISION NOT NULL,
    geom         GEOMETRY(Point, 4326) NOT NULL,
    level        INT NOT NULL DEFAULT 1,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    relocated_at TIMESTAMPTZ
);
CREATE INDEX idx_cores_geog ON cores USING GIST ((geom::geography));

-- A node id (from_id / to_id) is either a core.id or a tower.id; stored as TEXT
-- so a single edge table can reference both node kinds without a polymorphic FK.
CREATE TABLE beacon_links (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    player_id  UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    from_id    TEXT NOT NULL,
    to_id      TEXT NOT NULL,
    length_m   DOUBLE PRECISION NOT NULL,
    cost       INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (from_id, to_id)
);
CREATE INDEX idx_beacon_links_player ON beacon_links (player_id);

-- A cell is domed if any player's network encloses it. Per-player rows so a
-- collapse for one player does not clear a cell another player still domes.
CREATE TABLE domed_cells (
    player_id UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    cell_id   TEXT NOT NULL REFERENCES cells(id) ON DELETE CASCADE,
    PRIMARY KEY (player_id, cell_id)
);
CREATE INDEX idx_domed_cells_cell ON domed_cells (cell_id);
