-- Shop expansion items need persistent per-player state:
-- storage_500/storage_2000 raise the energy cap, pet_slot adds pet capacity.
ALTER TABLE players
    ADD COLUMN storage_bonus_energy INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN pet_slots INTEGER NOT NULL DEFAULT 1;
