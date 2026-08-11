# Domain Glossary

Ezra's gameplay vocabulary shows up everywhere in the code — package names,
handler names, DB columns, asynq task types. This page translates it into
plain engineering terms so the rest of the docs (and the code) read
naturally. Sourced from `internal/canon/` and each package's `model.go`,
not from old design/lore docs, so it should match what's actually running.

## Map & world

- **Cell** — a fixed tile of the real-world map grid (`internal/cell`), keyed
  by `"{lat}_{lng}"`, backed by a PostGIS point (`geom`). The atomic unit
  everything else (towers, rifts, infection) attaches to. Cells are lazily
  created the first time a player's `/map/cells` request touches a new
  ~0.1°×0.1° region — the server queries the Overpass API (OpenStreetMap) to
  seed terrain flags (`has_nearby_building`, `has_nearby_road`) for that area.
- **Infection** — a per-cell corruption value (0–100), recalculated by a
  scheduled worker (`internal/infection`, every 5 minutes) from nearby rifts,
  hives, towers and neighboring cells. Crossing a threshold can spawn a rift.
- **Rift** (`internal/rift`) — a hostile spawn point on the map. Has a tier
  (`minor` / `medium` / `critical`), an intensity, and a spirit count; expands
  hourly, spawns organically every 10 minutes, and is closed by winning a
  battle against it.
- **Hive** (`internal/hive`) — the Symbiont-side counterpart to a player's
  Core: the root of an infection region. Pulses infection into its radius and
  seeds rifts hourly.

## Player structures & network

- **Tower** ("beacon" in a lot of comments/UI text) — a player-owned
  structure (`internal/tower`) placed at its own lat/lng, anchored to a cell
  for infection bookkeeping. Has HP, a level, an upkeep cost, and projects a
  suppression radius that fights nearby infection. Accrues passive income
  hourly and takes hourly "pressure" damage from surrounding infection.
- **Core / Network** (`internal/network`) — a player's personal root node
  (`Core`) and the graph of `Link` edges connecting it to their towers. The
  Core has an energy-capacity ("Ecap") budget that must cover the sum of
  every tower's upkeep plus every link's cost; links cost
  `ceil(length_m / 100) * 3`.
- **Station** (`internal/station`) — a player-built power plant that raises
  local beacon capacity in areas with a sparse OSM grid. Has its own
  active → degrading → ruins lifecycle (6h ticks) unless serviced.
- **Legacy network** (`internal/legacy`) — what happens to a player's human
  towers/Core after they defect to the Symbiont faction: instead of vanishing,
  they decay `legacy_stable → legacy_fading → legacy_offline` over 24–48h
  (hourly worker), unless a different active human player sustains them by
  being nearby.
- **City Link** (`internal/citylink`) — an opt-in mutual-support alliance
  between two players: they can repair each other's towers and see each
  other's defense alerts. No resource/network merge, purely cooperative.
- **Spire** (`internal/spire`) — a large regional endgame structure players
  jointly fund (via fragments + resource contributions) and maintain; grants
  an infection-suppression bonus to its whole radius. 6-hour lifecycle ticks.

## Factions & PvP

- **Faction** (`internal/faction`) — a player is `human` or `symbiont`.
  Players "contact" the opposing side first, then "choose" explicitly;
  switching sides has a 24h cooldown and forfeits legacy income to whoever
  they abandoned.
- **FactionWar** (`internal/factionwar`) — a scored, seasonal competition
  between the two factions (closing rifts, collapsing hives, overloading
  beacons, breaching domes all award points), settled once a day by a worker.
- **PvP / Dome Breach** (`internal/pvp`) — a Symbiont can scout and "breach"
  an enemy player's defensive dome, forcing infection onto that cell and
  opening a rift under it.
- **Capture** (`internal/capture`) — taking an enemy tower: `lockpick`
  (stealthy, no fight) or `force` (starts a battle for it).

## Symbiont meta-progression

- **Symbiont** (`internal/symbiont`) — the Symbiont-faction geo-playstyle:
  standing under a hostile human player's dome slowly drains a Symbiont's
  energy (a scheduled "drain tick" every 2 minutes), easing off as they push
  the cell's infection back up toward carving out their own pocket.
- **Resonance** (`internal/resonance`) — the Symbiont's meta-progression
  track: a Resonance Level (RL1–RL5) unlocks more "entity control slots".
- **Roster / Entity** (`internal/roster`) — a Resonance Pool materializes a
  small number of autonomous **entities** (archetypes: `assault`,
  `distortion`, `scout`, `absorber`) that a player assigns to a hostile tower
  or a rift; each ticks on its own server-side timer through a
  `manifesting → assigned → recovering` state machine (10-minute tick).

## Army & combat

- **Unit** (`internal/unit`) — a single army member (type, level, XP, HP,
  ATK); decays if left idle too long (6h worker).
- **Squad** (`internal/squad`) — a named group of units sent on a mission
  (patrol / attack); missions complete via a 1-minute worker.
- **Battle** (`internal/battle`) — turn-based combat resolution against a
  rift or hive. Has a risky "overcharge" burst action (2.5× damage dealt,
  1.15× damage taken, 25% chance to misfire) gated behind release stage v1.0.
- **Survivor** (`internal/survivor`) — a recruitable NPC that spawns near the
  player and can be turned into a unit.
- **Pet** (`internal/pet`) — a companion that can be sent on search/patrol
  tasks and auto-claims its results via a 1-minute worker.

## Progression & meta

- **Player** (`internal/player`) — the account: level/XP, `energy` +
  `materials` resources, army limit, skill points (defender / commander /
  energist branches), last known GPS position (used for the anti-cheat speed
  check — see [ARCHITECTURE.md](ARCHITECTURE.md#anti-cheat)), onboarding
  state.
- **Achievement** (`internal/achievement`) — unlocks derived live from
  existing tables (beacon count, Core level, battles won, spectra collected,
  player level) — no separate stat-tracking system.
- **Bestiary** (`internal/bestiary`) — a collectible catalog of enemy types,
  rift tiers, tower types and resonance spectra the player has discovered.
- **Item** (`internal/item`) — inventory stacks; resonance "signatures" use
  the `Variant` field to record which spectrum they belong to (the
  collectible set for the endgame).
- **Quest** (`internal/quest`) — daily quests plus a login-streak check-in.
- **Shop** (`internal/shop`) — the IAP catalog: crystal purchases and
  subscriptions, with a 24h worker to expire lapsed subscriptions.

## Release gating

`internal/canon.CurrentReleaseStage` pins the whole backend to one of
`mvp → v1.0 → v1.1`. Some features declare a minimum stage (e.g. battle
overcharge and tower levels 4–5 need `v1.0`; `tower_pressure` needs `v1.1`)
and `canon.IsFeatureAvailable(feature)` gates them centrally instead of each
package re-implementing the check.
