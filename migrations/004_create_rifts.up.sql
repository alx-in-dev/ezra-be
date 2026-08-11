CREATE TABLE rifts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    cell_id TEXT NOT NULL REFERENCES cells(id),
    type TEXT NOT NULL DEFAULT 'minor' CHECK (type IN ('minor', 'medium', 'critical')),
    intensity INTEGER NOT NULL DEFAULT 1,
    radius_cells INTEGER NOT NULL DEFAULT 1,
    spirits_count INTEGER NOT NULL DEFAULT 3,
    opened_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    closed_at TIMESTAMPTZ
);

CREATE INDEX idx_rifts_cell_id ON rifts(cell_id);
CREATE INDEX idx_rifts_open ON rifts(closed_at) WHERE closed_at IS NULL;

ALTER TABLE cells ADD CONSTRAINT fk_cells_rift FOREIGN KEY (rift_id) REFERENCES rifts(id) ON DELETE SET NULL;
