-- Roll back to the migration 007 constraints. Destructive for rows that
-- were written under the canon state set — they'll fail the old CHECK.
-- Only safe if the table is empty or holds only legacy-shape rows.

-- Remap status values back to the old three-state model.
UPDATE battles SET status = 'ongoing' WHERE status IN ('pending_start', 'round_active', 'awaiting_tactic');
UPDATE battles SET status = 'won'     WHERE status = 'resolved_victory';
UPDATE battles SET status = 'lost'    WHERE status IN ('resolved_defeat', 'resolved_retreat');

ALTER TABLE battles DROP CONSTRAINT battles_status_check;
ALTER TABLE battles ALTER COLUMN status SET DEFAULT 'ongoing';
ALTER TABLE battles ADD CONSTRAINT battles_status_check
    CHECK (status IN ('ongoing', 'won', 'lost'));

-- Purge tutorial battles (UUID cast below will fail for slug target_id).
DELETE FROM battles WHERE target_type = 'tutorial';

ALTER TABLE battles ALTER COLUMN target_id TYPE UUID USING target_id::uuid;

ALTER TABLE battles DROP CONSTRAINT battles_target_type_check;
ALTER TABLE battles ADD CONSTRAINT battles_target_type_check
    CHECK (target_type IN ('rift', 'tower'));
