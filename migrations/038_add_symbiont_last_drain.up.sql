-- Soft-drain throttle (#6, T-761): when a Symbiont stands under a hostile dome,
-- their energy bleeds — but at most once per drain interval regardless of how
-- often the client polls /symbiont/status. This timestamp gates that.
ALTER TABLE players ADD COLUMN IF NOT EXISTS symbiont_last_drain_at TIMESTAMPTZ;
