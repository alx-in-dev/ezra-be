CREATE TABLE towers (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    cell_id TEXT NOT NULL REFERENCES cells(id),
    owner_id UUID NOT NULL REFERENCES players(id),
    level INTEGER NOT NULL DEFAULT 1,
    type TEXT NOT NULL DEFAULT 'standard' CHECK (type IN ('standard', 'amplified', 'relay', 'symbiont')),
    hp INTEGER NOT NULL DEFAULT 100,
    hp_max INTEGER NOT NULL DEFAULT 100,
    radius_m REAL NOT NULL DEFAULT 100,
    effect_per_hour REAL NOT NULL DEFAULT 3.0,
    installed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    upgraded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_towers_cell_id ON towers(cell_id);
CREATE INDEX idx_towers_owner_id ON towers(owner_id);

-- Update cells to reference towers
ALTER TABLE cells ADD CONSTRAINT fk_cells_tower FOREIGN KEY (tower_id) REFERENCES towers(id) ON DELETE SET NULL;
