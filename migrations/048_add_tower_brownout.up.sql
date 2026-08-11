-- Brownout becomes a real effect (beacon_network_dome.md §5): over-budget
-- networks brown out their deepest nodes — halved suppression and income.
-- The flag is recomputed by network recompute; SQL consumers (infection
-- batch, income worker) read it directly.
ALTER TABLE towers ADD COLUMN brownout boolean NOT NULL DEFAULT false;
