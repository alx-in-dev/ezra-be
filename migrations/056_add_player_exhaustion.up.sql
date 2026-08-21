-- N2 (T-865): a spirit touch in the field applies a temporary "Истощение"
-- debuff. exhausted_until is the soft, venue-safe effect window (reduced
-- regen/income while active) — never a loss. Read by income/verb code.
ALTER TABLE players ADD COLUMN IF NOT EXISTS exhausted_until TIMESTAMPTZ;
