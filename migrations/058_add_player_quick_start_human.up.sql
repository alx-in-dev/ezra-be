-- Onboarding quick-start (docs/feature/onboarding_quick_start.md): a fresh
-- player may skip the narrative chain and place only their first beacon
-- manually (the one real GPS action left) — the rest (2nd survivor, tutorial
-- battle rewards, starter pet) is granted automatically once they do.
ALTER TABLE players ADD COLUMN IF NOT EXISTS quick_start_human BOOLEAN NOT NULL DEFAULT false;
