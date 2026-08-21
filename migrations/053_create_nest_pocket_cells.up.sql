-- N3 (ADR-N3-8, spirit_world_and_symbiont_nest.md §5.4): a Nest seeded into a
-- carved "pocket" (a currently dome-pierced cell) holds those pierced cells
-- open WITHOUT a TTL for as long as the Nest lives — nest:tick refreshes their
-- cells.pierced_until. This table records which cells a Nest holds, both to
-- refresh them each tick and to release them (back to normal TTL) on collapse
-- or relocation. Self-healing: when the Nest dies the refresh simply stops and
-- the cells expire on their own — no infinity-sentinel to unwind.
CREATE TABLE IF NOT EXISTS nest_pocket_cells (
    nest_id UUID NOT NULL REFERENCES nests(id) ON DELETE CASCADE,
    cell_id TEXT NOT NULL REFERENCES cells(id),
    PRIMARY KEY (nest_id, cell_id)
);

CREATE INDEX IF NOT EXISTS idx_nest_pocket_cells_cell ON nest_pocket_cells (cell_id);
