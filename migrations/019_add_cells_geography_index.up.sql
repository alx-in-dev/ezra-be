-- All radius queries cast geom to geography (ST_DWithin(geom::geography, ...)),
-- which cannot use the plain geometry GiST index from 002. Add a functional
-- index so neighbor/rift/tower lookups and the batch infection recalc are
-- index-assisted.
CREATE INDEX idx_cells_geog ON cells USING GIST ((geom::geography));
