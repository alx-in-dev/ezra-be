-- Align battles table constraints with the actual code contract (canon.go +
-- battle.Service.Start). Migration 007 hardcoded constraints based on the
-- original MVP scope (target: rift or tower; status: ongoing/won/lost), but
-- the code has since added:
--   • target_type = "tutorial" (onboarding first battle)
--   • target_id can be a non-UUID slug for tutorial targets ("starter")
--   • 6 battle states from canon.BattleState*: pending_start, round_active,
--     awaiting_tactic, resolved_victory, resolved_defeat, resolved_retreat
--
-- Attempting to start a tutorial battle under the old schema blows up at
-- INSERT time with a CHECK constraint violation that surfaces to the client
-- as a generic HTTP 500 "failed to start battle" (observed in production
-- 2026-04-11).
--
-- NOTE: the accompanying down-migration is DESTRUCTIVE. It deletes every
-- row with target_type = 'tutorial' because target_id for tutorial battles
-- ("starter") is not a valid UUID and the column type cast back to UUID
-- would otherwise fail. Only run the downgrade on an empty table or
-- accept that onboarding battles for active players will be wiped.

-- target_type: add 'tutorial'
ALTER TABLE battles DROP CONSTRAINT battles_target_type_check;
ALTER TABLE battles ADD CONSTRAINT battles_target_type_check
    CHECK (target_type IN ('rift', 'tower', 'tutorial'));

-- target_id: relax from UUID to TEXT so tutorial targets ("starter") fit.
-- Rift / tower targets will still contain UUIDs, just typed as text now.
ALTER TABLE battles ALTER COLUMN target_id TYPE TEXT USING target_id::text;

-- status: use the full canon set. Existing rows with 'ongoing' are remapped
-- to 'awaiting_tactic' (matches what the code sets at Battle creation).
UPDATE battles SET status = 'awaiting_tactic' WHERE status = 'ongoing';
UPDATE battles SET status = 'resolved_victory' WHERE status = 'won';
UPDATE battles SET status = 'resolved_defeat'  WHERE status = 'lost';

ALTER TABLE battles DROP CONSTRAINT battles_status_check;
ALTER TABLE battles ALTER COLUMN status SET DEFAULT 'pending_start';
ALTER TABLE battles ADD CONSTRAINT battles_status_check
    CHECK (status IN (
        'pending_start',
        'round_active',
        'awaiting_tactic',
        'resolved_victory',
        'resolved_defeat',
        'resolved_retreat'
    ));
