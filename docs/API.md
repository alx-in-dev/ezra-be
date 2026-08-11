# API Reference

Base path: `/api/v1`. All routes below require `Authorization: Bearer
<session_token>` **except** the ones explicitly marked public. Domain terms
are explained in [DOMAIN_GLOSSARY.md](DOMAIN_GLOSSARY.md); the wiring is in
[ARCHITECTURE.md](ARCHITECTURE.md). This is a map of the surface, not a full
request/response schema reference — read the linked handler for exact
payloads.

`GET /health` (public, outside `/api/v1`) — liveness check.

## Auth — `internal/auth`

| Method | Path | Public? | Handler |
|---|---|---|---|
| POST | `/auth/register` | yes | `authHandler.Register` |
| POST | `/auth/login` | yes | `authHandler.Login` |
| POST | `/auth/logout` | | `authHandler.Logout` |

Both register and login accept either `{firebase_token, username}` or
`{login, password}` in the body — see [ARCHITECTURE.md#auth](ARCHITECTURE.md#auth).

## Player & profile — `internal/player`

| Method | Path | Handler |
|---|---|---|
| GET | `/player` | `playerHandler.GetPlayer` |
| GET | `/profile` | `playerHandler.GetProfile` |
| PATCH | `/profile/username` | `playerHandler.UpdateUsername` |
| GET | `/onboarding` | `playerHandler.GetOnboarding` |
| POST | `/onboarding/advance` | `playerHandler.AdvanceOnboarding` |
| GET | `/skills` | `playerHandler.GetSkills` |
| POST | `/skills/{id}/unlock` | `playerHandler.UnlockSkill` |
| PATCH | `/player/position` | `playerHandler.UpdatePosition` (anti-cheat speed check) |
| POST | `/player/push-token` | `pushHandler.RegisterToken` |

## Map — `internal/cell`, `internal/network`

| Method | Path | Handler |
|---|---|---|
| GET | `/map/cells` | `cellHandler.GetCells` |
| GET | `/map/fields` | `networkHandler.GetRegionFields` |

## Network / Core — `internal/network`

| Method | Path | Handler |
|---|---|---|
| POST | `/core` | `networkHandler.PlaceCore` |
| POST | `/core/relocate` | `networkHandler.RelocateCore` |
| POST | `/core/upgrade` | `networkHandler.UpgradeCore` |
| GET | `/network` | `networkHandler.GetNetwork` |

## Towers — `internal/tower`, `internal/capture`

| Method | Path | Handler |
|---|---|---|
| POST | `/towers` | `towerHandler.Create` |
| GET | `/towers/mine` | `towerHandler.GetMine` |
| PATCH | `/towers/{id}/upgrade` | `towerHandler.Upgrade` |
| POST | `/towers/{id}/repair` | `towerHandler.Repair` |
| DELETE | `/towers/{id}` | `towerHandler.Delete` |
| POST | `/towers/{id}/capture/lockpick` | `captureHandler.Lockpick` |
| POST | `/towers/{id}/capture/force` | `captureHandler.ForceCapture` (starts a battle) |

## Stations — `internal/station`

| Method | Path | Handler |
|---|---|---|
| GET | `/stations` | `stationHandler.List` |
| POST | `/stations` | `stationHandler.Build` |
| POST | `/stations/upkeep` | `stationHandler.Upkeep` |

## City Links — `internal/citylink`

| Method | Path | Handler |
|---|---|---|
| GET | `/city-links` | `cityLinkHandler.List` |
| GET | `/city-links/search` | `cityLinkHandler.Search` |
| POST | `/city-links/request` | `cityLinkHandler.Request` |
| GET | `/city-links/requests` | `cityLinkHandler.Inbox` |
| POST | `/city-links/requests/{id}/accept` | `cityLinkHandler.Accept` |
| POST | `/city-links/requests/{id}/reject` | `cityLinkHandler.Reject` |
| POST | `/city-links/{id}/remove` | `cityLinkHandler.Remove` |

## Realtime — `internal/realtime`

| Method | Path | Handler |
|---|---|---|
| GET | `/events` | `realtimeHandler.Stream` (SSE, long-lived) |
| POST | `/events/test` | `realtimeHandler.SelfTest` (publishes a sample event to yourself) |

## Faction & FactionWar — `internal/faction`, `internal/factionwar`

| Method | Path | Handler |
|---|---|---|
| GET | `/faction` | `factionHandler.Status` |
| POST | `/faction/choose` | `factionHandler.Choose` |
| GET | `/faction-war` | `factionWarHandler.Status` |
| POST | `/faction-war/settle` | `factionWarHandler.Settle` |

## PvP — `internal/pvp`

| Method | Path | Handler |
|---|---|---|
| GET | `/pvp/targets` | `pvpHandler.Targets` |
| POST | `/pvp/breach` | `pvpHandler.Breach` |

## Symbiont — `internal/symbiont`, `internal/roster`, `internal/resonance`

| Method | Path | Handler |
|---|---|---|
| GET | `/symbiont/status` | `symbiontHandler.Status` |
| POST | `/symbiont/raise` | `symbiontHandler.Raise` |
| POST | `/symbiont/overload` | `symbiontHandler.Overload` |
| GET | `/symbiont/recon` | `symbiontHandler.Recon` |
| GET | `/symbiont/entities` | `symbiontHandler.Entities` |
| POST | `/symbiont/entities/assign` | `symbiontHandler.AssignEntity` |
| POST | `/symbiont/entities/recall` | `symbiontHandler.RecallEntity` |
| GET | `/resonance` | `resonanceHandler.Status` |
| POST | `/resonance/activate` | `resonanceHandler.Activate` |

## Hives — `internal/hive`

| Method | Path | Handler |
|---|---|---|
| GET | `/hives` | `hiveHandler.List` |
| POST | `/hives/{id}/empower` | `hiveHandler.Empower` |

## Spire — `internal/spire`

| Method | Path | Handler |
|---|---|---|
| GET | `/spire` | `spireHandler.Status` |
| POST | `/spire/contribute-fragment` | `spireHandler.ContributeFragment` |
| POST | `/spire/contribute` | `spireHandler.ContributeResources` |

## Army: units, squads, battles — `internal/unit`, `internal/squad`, `internal/battle`

| Method | Path | Handler |
|---|---|---|
| GET | `/units` | `unitHandler.List` |
| POST | `/units` | `unitHandler.Recruit` |
| GET | `/squads` | `squadHandler.List` |
| POST | `/squads` | `squadHandler.Create` |
| PATCH | `/squads/{id}` | `squadHandler.Update` |
| POST | `/squads/{id}/send` | `squadHandler.Send` |
| POST | `/squads/{id}/recall` | `squadHandler.Return` |
| DELETE | `/squads/{id}` | `squadHandler.Disband` |
| POST | `/battles/start` | `battleHandler.Start` |
| POST | `/battles/{id}/action` | `battleHandler.Action` |
| POST | `/battles/{id}/overcharge` | `battleHandler.Overcharge` (v1.0+, see canon gating) |

## Pets & survivors — `internal/pet`, `internal/survivor`

| Method | Path | Handler |
|---|---|---|
| GET | `/pets` | `petHandler.GetPets` |
| POST | `/pets/starter-claim` | `petHandler.ClaimStarter` |
| POST | `/pets/{id}/send` | `petHandler.Send` |
| POST | `/pets/{id}/recall` | `petHandler.Recall` |
| GET | `/survivors` | `survivorHandler.Spawn` |
| POST | `/survivors/recruit` | `survivorHandler.Recruit` |

## Progression: items, achievements, bestiary, quests — `internal/item`, `internal/achievement`, `internal/bestiary`, `internal/quest`

| Method | Path | Handler |
|---|---|---|
| GET | `/items` | `itemHandler.List` |
| GET | `/achievements` | `achievementHandler.Get` |
| GET | `/bestiary` | `bestiaryHandler.Get` |
| GET | `/quests` | `questHandler.GetQuests` |
| POST | `/quests/{id}/claim` | `questHandler.ClaimQuest` |
| POST | `/streaks/checkin` | `questHandler.CheckIn` |

## Shop — `internal/shop`

| Method | Path | Handler |
|---|---|---|
| GET | `/shop/catalog` | `shopHandler.GetCatalog` |
| POST | `/shop/buy` | `shopHandler.Buy` |
| POST | `/shop/crystals` | `shopHandler.AddCrystals` |
| GET | `/shop/subscription` | `shopHandler.GetSubscription` |
| POST | `/shop/subscription/activate` | `shopHandler.ActivateSubscription` |

---

Route source of truth: `cmd/ezra/main.go`. If this page and the code
disagree, the code wins — please send a fix.
