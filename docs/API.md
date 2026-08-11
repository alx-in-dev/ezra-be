# API Reference

Базовый путь: `/api/v1`. Все роуты ниже требуют `Authorization: Bearer
<session_token>`, **кроме** явно помеченных как публичные. Доменные термины
объяснены в [DOMAIN_GLOSSARY.md](DOMAIN_GLOSSARY.md); как всё собрано — в
[ARCHITECTURE.md](ARCHITECTURE.md). Это карта поверхности API, а не полная
схема запросов/ответов — за точным payload'ом смотри соответствующий handler.

`GET /health` (публичный, вне `/api/v1`) — проверка живости.

## Auth — `internal/auth`

| Метод | Путь | Публичный? | Handler |
|---|---|---|---|
| POST | `/auth/register` | да | `authHandler.Register` |
| POST | `/auth/login` | да | `authHandler.Login` |
| POST | `/auth/logout` | | `authHandler.Logout` |

И register, и login принимают в теле либо `{firebase_token, username}`,
либо `{login, password}` — см. [ARCHITECTURE.md#авторизация](ARCHITECTURE.md#авторизация).

## Игрок и профиль — `internal/player`

| Метод | Путь | Handler |
|---|---|---|
| GET | `/player` | `playerHandler.GetPlayer` |
| GET | `/profile` | `playerHandler.GetProfile` |
| PATCH | `/profile/username` | `playerHandler.UpdateUsername` |
| GET | `/onboarding` | `playerHandler.GetOnboarding` |
| POST | `/onboarding/advance` | `playerHandler.AdvanceOnboarding` |
| GET | `/skills` | `playerHandler.GetSkills` |
| POST | `/skills/{id}/unlock` | `playerHandler.UnlockSkill` |
| PATCH | `/player/position` | `playerHandler.UpdatePosition` (проверка скорости — античит) |
| POST | `/player/push-token` | `pushHandler.RegisterToken` |

## Карта — `internal/cell`, `internal/network`

| Метод | Путь | Handler |
|---|---|---|
| GET | `/map/cells` | `cellHandler.GetCells` |
| GET | `/map/fields` | `networkHandler.GetRegionFields` |

## Network / Core — `internal/network`

| Метод | Путь | Handler |
|---|---|---|
| POST | `/core` | `networkHandler.PlaceCore` |
| POST | `/core/relocate` | `networkHandler.RelocateCore` |
| POST | `/core/upgrade` | `networkHandler.UpgradeCore` |
| GET | `/network` | `networkHandler.GetNetwork` |

## Башни — `internal/tower`, `internal/capture`

| Метод | Путь | Handler |
|---|---|---|
| POST | `/towers` | `towerHandler.Create` |
| GET | `/towers/mine` | `towerHandler.GetMine` |
| PATCH | `/towers/{id}/upgrade` | `towerHandler.Upgrade` |
| POST | `/towers/{id}/repair` | `towerHandler.Repair` |
| DELETE | `/towers/{id}` | `towerHandler.Delete` |
| POST | `/towers/{id}/capture/lockpick` | `captureHandler.Lockpick` |
| POST | `/towers/{id}/capture/force` | `captureHandler.ForceCapture` (запускает бой) |

## Станции — `internal/station`

| Метод | Путь | Handler |
|---|---|---|
| GET | `/stations` | `stationHandler.List` |
| POST | `/stations` | `stationHandler.Build` |
| POST | `/stations/upkeep` | `stationHandler.Upkeep` |

## City Links — `internal/citylink`

| Метод | Путь | Handler |
|---|---|---|
| GET | `/city-links` | `cityLinkHandler.List` |
| GET | `/city-links/search` | `cityLinkHandler.Search` |
| POST | `/city-links/request` | `cityLinkHandler.Request` |
| GET | `/city-links/requests` | `cityLinkHandler.Inbox` |
| POST | `/city-links/requests/{id}/accept` | `cityLinkHandler.Accept` |
| POST | `/city-links/requests/{id}/reject` | `cityLinkHandler.Reject` |
| POST | `/city-links/{id}/remove` | `cityLinkHandler.Remove` |

## Реалтайм — `internal/realtime`

| Метод | Путь | Handler |
|---|---|---|
| GET | `/events` | `realtimeHandler.Stream` (SSE, долгоживущее соединение) |
| POST | `/events/test` | `realtimeHandler.SelfTest` (публикует тестовое событие тебе же) |

## Faction & FactionWar — `internal/faction`, `internal/factionwar`

| Метод | Путь | Handler |
|---|---|---|
| GET | `/faction` | `factionHandler.Status` |
| POST | `/faction/choose` | `factionHandler.Choose` |
| GET | `/faction-war` | `factionWarHandler.Status` |
| POST | `/faction-war/settle` | `factionWarHandler.Settle` |

## PvP — `internal/pvp`

| Метод | Путь | Handler |
|---|---|---|
| GET | `/pvp/targets` | `pvpHandler.Targets` |
| POST | `/pvp/breach` | `pvpHandler.Breach` |

## Symbiont — `internal/symbiont`, `internal/roster`, `internal/resonance`

| Метод | Путь | Handler |
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

## Ульи (Hives) — `internal/hive`

| Метод | Путь | Handler |
|---|---|---|
| GET | `/hives` | `hiveHandler.List` |
| POST | `/hives/{id}/empower` | `hiveHandler.Empower` |

## Spire — `internal/spire`

| Метод | Путь | Handler |
|---|---|---|
| GET | `/spire` | `spireHandler.Status` |
| POST | `/spire/contribute-fragment` | `spireHandler.ContributeFragment` |
| POST | `/spire/contribute` | `spireHandler.ContributeResources` |

## Армия: юниты, отряды, бои — `internal/unit`, `internal/squad`, `internal/battle`

| Метод | Путь | Handler |
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
| POST | `/battles/{id}/overcharge` | `battleHandler.Overcharge` (v1.0+, см. гейтинг canon) |

## Питомцы и survivors — `internal/pet`, `internal/survivor`

| Метод | Путь | Handler |
|---|---|---|
| GET | `/pets` | `petHandler.GetPets` |
| POST | `/pets/starter-claim` | `petHandler.ClaimStarter` |
| POST | `/pets/{id}/send` | `petHandler.Send` |
| POST | `/pets/{id}/recall` | `petHandler.Recall` |
| GET | `/survivors` | `survivorHandler.Spawn` |
| POST | `/survivors/recruit` | `survivorHandler.Recruit` |

## Прогрессия: предметы, достижения, бестиарий, квесты — `internal/item`, `internal/achievement`, `internal/bestiary`, `internal/quest`

| Метод | Путь | Handler |
|---|---|---|
| GET | `/items` | `itemHandler.List` |
| GET | `/achievements` | `achievementHandler.Get` |
| GET | `/bestiary` | `bestiaryHandler.Get` |
| GET | `/quests` | `questHandler.GetQuests` |
| POST | `/quests/{id}/claim` | `questHandler.ClaimQuest` |
| POST | `/streaks/checkin` | `questHandler.CheckIn` |

## Магазин — `internal/shop`

| Метод | Путь | Handler |
|---|---|---|
| GET | `/shop/catalog` | `shopHandler.GetCatalog` |
| POST | `/shop/buy` | `shopHandler.Buy` |
| POST | `/shop/crystals` | `shopHandler.AddCrystals` |
| GET | `/shop/subscription` | `shopHandler.GetSubscription` |
| POST | `/shop/subscription/activate` | `shopHandler.ActivateSubscription` |

---

Источник истины по роутам: `cmd/ezra/main.go`. Если эта страница и код
разошлись — прав код, пришли, пожалуйста, фикс.
