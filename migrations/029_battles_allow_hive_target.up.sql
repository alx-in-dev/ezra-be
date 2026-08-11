-- E1 deepening: squads can assault Symbiont hives. Allow target_type = 'hive'.
ALTER TABLE battles DROP CONSTRAINT battles_target_type_check;
ALTER TABLE battles ADD CONSTRAINT battles_target_type_check
    CHECK (target_type IN ('rift', 'tower', 'tutorial', 'hive'));
