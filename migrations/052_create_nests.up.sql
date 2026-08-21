-- N3 (spirit_world_and_symbiont_nest.md §5): the Symbiont home Nest — the
-- mirror of the human Core, giving the faction Ownership (CD4) and Loss (CD8).
-- Unlike a world-owned Hive (seeded by a worker, collapses in ≤2 assaults), a
-- Nest is player-owned, opened at onboarding, relocatable, feeds/decays, and
-- defends via a soft-timer siege that a raid can NEVER close in one blow —
-- collapse is applied only by nest:tick (single-writer). See architecture.md
-- ADR-N3-1..11. RED LINE #2 stays intact: collapse never touches profile
-- Resonance/roster/Attunement — only the row's accrued_resonance buffer.
CREATE TABLE IF NOT EXISTS nests (
    id                UUID             NOT NULL PRIMARY KEY DEFAULT uuid_generate_v4(),
    owner_id          UUID             NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    cell_id           TEXT             NOT NULL REFERENCES cells(id),
    lat               DOUBLE PRECISION NOT NULL,
    lng               DOUBLE PRECISION NOT NULL,
    geom              GEOMETRY(Point, 4326) NOT NULL,
    level             INT              NOT NULL DEFAULT 1,

    -- Trickle buffer (ADR-N3-7): nest:tick accrues Resonance HERE, never into
    -- the profile. POST /nest/collect moves buffer→profile; collapse zeroes it.
    -- This is the ONLY thing lost on collapse (the Executable-Loss field).
    accrued_resonance DOUBLE PRECISION NOT NULL DEFAULT 0,

    -- Feed/decay (T-834): support 0..100 vitality (mirror of hive.intensity).
    -- Feeding restores it; neglect decays it, degrading OUTPUT (aura radius /
    -- trickle), never existence. Floor + newbie protection live in code.
    support_level     DOUBLE PRECISION NOT NULL DEFAULT 100,

    -- Siege state-machine (ADR-N3-4): healthy → under_siege → collapsing →
    -- collapsed. siege_hp drains on assault victory; collapse_at is the honest
    -- ETA floor; nest:tick applies collapse when now ≥ collapse_at.
    siege_hp          DOUBLE PRECISION NOT NULL DEFAULT 100,
    siege_state       TEXT             NOT NULL DEFAULT 'healthy'
                          CHECK (siege_state IN ('healthy','under_siege','collapsing','collapsed')),
    collapse_at       TIMESTAMPTZ,
    siege_attacker_id UUID             REFERENCES players(id),

    -- Relocation (ADR-N3-6, mirror of cores.relocated_at): nil until the first
    -- (free) move; doubles as the "has relocated before" flag for pricing.
    relocated_at      TIMESTAMPTZ,

    created_at        TIMESTAMPTZ      NOT NULL DEFAULT now(),
    -- Terminal: a collapsed nest keeps its row as history (rebuild-cooldown +
    -- grandfathering read "has ANY nest row ever existed"). Live nest = NULL.
    collapsed_at      TIMESTAMPTZ
);

-- Spatial: aura / nearby lookups (geography GiST, per gotchas ::geography rule).
CREATE INDEX IF NOT EXISTS idx_nests_geog ON nests USING GIST ((geom::geography));

-- Cap 1 LIVE nest per player (ADR-N3-11): collapsed history rows do not conflict.
CREATE UNIQUE INDEX IF NOT EXISTS idx_nests_owner_live ON nests (owner_id) WHERE collapsed_at IS NULL;

-- The nest:tick / aura / budget queries all scan live nests.
CREATE INDEX IF NOT EXISTS idx_nests_live ON nests (collapsed_at) WHERE collapsed_at IS NULL;
