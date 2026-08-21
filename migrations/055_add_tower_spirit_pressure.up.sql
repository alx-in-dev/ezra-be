-- N2 (T-864, spirit_world_and_symbiont_nest.md §4.2): the brownout-cascade.
-- A strong spirit wave can force a PERIMETER beacon into brownout without any
-- spirit entering the dome. spirit_pressure_until is a time-boxed cause of
-- brownout, separate from network.Recompute's energy-budget verdict (so the two
-- never fight): a beacon is treated as brownout while now < spirit_pressure_until
-- — it drops from the powered contour (dome shrinks) and its суппрессия halves,
-- exactly like an energy-brownout beacon. It recovers on its own when the window
-- lapses (venue-safety: impulse, not continuous).
ALTER TABLE towers ADD COLUMN IF NOT EXISTS spirit_pressure_until TIMESTAMPTZ;
