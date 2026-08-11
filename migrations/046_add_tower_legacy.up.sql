-- Legacy human network (R4-4a, legacy_human_network.md). When a beacon's owner
-- sides with the Symbionts the beacon becomes "legacy" infrastructure: it keeps
-- doming for a while, then fades, then goes offline. legacy_since stamps when it
-- became legacy (NULL = active human-owned). The degradation state is derived
-- from (tower level, now - legacy_since) by canon.LegacyState; an active human
-- nearby resets legacy_since (sustain).
ALTER TABLE towers ADD COLUMN IF NOT EXISTS legacy_since TIMESTAMPTZ;
