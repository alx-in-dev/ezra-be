ALTER TABLE pets DROP CONSTRAINT pets_status_check;
ALTER TABLE pets ADD CONSTRAINT pets_status_check CHECK (status = ANY (ARRAY[
    'idle'::text,
    'on_task'::text,
    'evolved'::text
]));
