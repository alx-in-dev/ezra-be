-- N4 (spirit_world_and_symbiont_nest.md §6, T-872): geysers — a rare POI that
-- erupts class-V wild spirits. High-class spirits must come from special places,
-- not anywhere (venue-safety + "высшие классы только в своих локациях/RL"):
-- geysers gate class V, high-level hives gate class IV. Geysers are seeded slowly
-- by the spirit tick in deep Symbiont territory (near hives), kept sparse by a
-- minimum spacing. NOTE: the design's other class-IV source — "дикие зоны"
-- (Фаза 5.3) — is not built yet; until it lands, class IV comes from max-level
-- hives (see impl_notes Auto-deviations, T-872).
CREATE TABLE IF NOT EXISTS geysers (
    id         UUID             NOT NULL PRIMARY KEY DEFAULT uuid_generate_v4(),
    geom       GEOMETRY(Point, 4326) NOT NULL,
    created_at TIMESTAMPTZ      NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_geysers_geog ON geysers USING GIST ((geom::geography));
