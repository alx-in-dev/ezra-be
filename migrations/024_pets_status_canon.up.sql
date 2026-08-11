-- The pets.status CHECK still allowed the pre-canon vocabulary
-- (idle/on_task/evolved); the code writes the canon states (canon.go:94-98),
-- so every pet send died with 23514. Align the constraint with the canon.
ALTER TABLE pets DROP CONSTRAINT pets_status_check;
ALTER TABLE pets ADD CONSTRAINT pets_status_check CHECK (status = ANY (ARRAY[
    'idle'::text,
    'mission_active'::text,
    'return_ready'::text,
    'supporting_tower'::text,
    'retired'::text
]));
