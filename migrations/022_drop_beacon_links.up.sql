-- Perimeter model: links are derived from node positions on every recompute
-- (closest-connected-parent over overlapping energy disks) and never stored.
-- The beacon_links table was write-only since the disk-overlap port — drop it.
DROP TABLE IF EXISTS beacon_links;
