-- T-824: rollback safety net for the N1 world localization. Before the one-off
-- destructive migration worker (T-825, wave 2) clears infection outside source
-- radius, the full per-cell infection column is snapshotted here so the live
-- prod world can be restored if localization goes wrong.
--
-- PK (cell_id, snapshot_at) lets the same cell be snapshotted more than once
-- (e.g. a dry-run snapshot then the real pre-migration snapshot); restore reads
-- the latest snapshot_at. No FK to cells on purpose: a recovery table must stay
-- decoupled from its source so cell churn can never cascade-delete the backup.
--
-- IMPORTANT (scope of this wave): this migration creates the table ONLY. The
-- actual snapshot is taken by the T-825 worker as its guarded first step,
-- immediately before the destructive UPDATE — NOT at migrate time — because a
-- migrate-time snapshot would be stale by the hour/day T-825 runs. T-825 must
-- refuse to run if it has not just written a fresh snapshot (row-count guard).
--
-- Reproducible rollback path (documented; also in ai/impl_notes.md):
--
--   -- 1. Snapshot (run by T-825 right before clearing):
--   INSERT INTO infection_snapshots (cell_id, infection, snapshot_at)
--   SELECT id, infection, now() FROM cells;
--
--   -- 2. Restore the whole world from the most recent snapshot:
--   UPDATE cells c
--   SET infection = s.infection, last_calculated = now()
--   FROM (
--       SELECT DISTINCT ON (cell_id) cell_id, infection
--       FROM infection_snapshots
--       ORDER BY cell_id, snapshot_at DESC
--   ) s
--   WHERE s.cell_id = c.id;
--
-- Validity window: the snapshot is authoritative for the `infection` column
-- only (rifts/hives/towers are grandfathered by T-825, not destroyed). Restore
-- is meant to run within the same maintenance window, before many @5m
-- BatchRecalculate ticks have diverged the world from the snapshot.
CREATE TABLE IF NOT EXISTS infection_snapshots (
    cell_id     TEXT             NOT NULL,
    infection   DOUBLE PRECISION NOT NULL,
    snapshot_at TIMESTAMPTZ      NOT NULL DEFAULT now(),
    PRIMARY KEY (cell_id, snapshot_at)
);

-- Restore and "latest snapshot" lookups filter/order by snapshot_at.
CREATE INDEX IF NOT EXISTS idx_infection_snapshots_at
    ON infection_snapshots (snapshot_at DESC);
