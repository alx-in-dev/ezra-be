# Architecture

## Layering

Every game-domain package under `internal/` follows the same shape:

```
model.go        types / DTOs
repository.go   Postgres access — an interface + a Pg-backed implementation
service.go      business logic, talks to the repository (and other services)
handler.go      HTTP layer: decode request → call service → encode response
worker.go       (optional) asynq task handler(s) for background jobs
*_test.go       unit tests against hand-written fakes of the interfaces above
```

`internal/canon` is the one exception: pure constants and rule functions
(release-stage gating, battle/tower/network tuning) shared by other packages,
with no DB access of its own.

There's no dependency-injection framework. Everything is constructed by hand,
in order, in `cmd/ezra/main.go`:

```go
db, err := platform.NewPostgres(ctx, cfg.DatabaseURL)
...
towerRepo := tower.NewPgRepository(db)
towerSvc  := tower.NewService(towerRepo, ...)
towerHandler := tower.NewHandler(towerSvc)
...
r.Post("/towers", towerHandler.Create)
```

It's long (~585 lines) but linear and easy to trace — if you want to know
what a package depends on, read its constructor call in `main.go`. One
notable pattern: `cell` and `tower` would import each other, so `main.go`
defines a small `towerReaderAdapter` there to break the cycle instead of
merging the packages.

## HTTP

Routing is [chi](https://github.com/go-chi/chi). Everything lives under
`/api/v1`, split into two groups:

- **Public**: `GET /health`, `POST /auth/register`, `POST /auth/login`.
- **Protected**: everything else, wrapped in `r.Group(...)` with
  `auth.AuthMiddleware` applied. See [API.md](API.md) for the full route
  list.

Global middleware (`pkg/middleware`), applied in this order: panic
`Recovery` → request `Logging` → `CORS`. Rate limiting
(`middleware.RateLimit`, Redis-backed token bucket) is available for
per-route use but not applied globally.

Shared request/response helpers (`httputil.Decode`, `httputil.JSON`,
`httputil.Error`, typed error constructors like `NewBadRequest`/
`NewUnauthorized`) and the request-scoped player-ID context live in
`pkg/httputil`.

## Auth

Two independent login methods, both ending in the same session:

1. **Firebase**: client sends a Firebase ID token → `auth.Service.
   VerifyFirebaseToken` validates it via the Firebase Admin SDK
   (`platform.NewFirebase`) → resolves a `firebase_uid` → player is found or
   created by that UID.
2. **Login/password**: `login` + `password` → bcrypt-checked against the
   stored hash.

Either way, a successful `/auth/login` or `/auth/register` mints a random
session token (`auth.Service.CreateSession`), stores
`session:<token> → playerID` in Redis with a 7-day TTL, and returns it as
`session_token`. Every protected route requires
`Authorization: Bearer <token>`; `auth.AuthMiddleware` resolves it via Redis
and 401s on anything missing/invalid/expired. `POST /auth/logout` just
deletes the Redis key.

No Firebase project is required for local development — the login/password
path is fully self-contained. See [SETUP.md](SETUP.md).

## Anti-cheat

`internal/player` validates every `PATCH /player/position` update: given the
player's last known position and the newly claimed one, it computes implied
speed and rejects the update as `impossible_speed` above `MAX_SPEED_KMH`
(default 50 km/h). This exists to catch GPS-spoofed "teleports". Local/dev
environments raise the cap (see `docker-compose.yml`) so GPS-simulator tools
can move freely.

## Realtime

`internal/realtime` is a **Server-Sent Events** stream, not WebSocket:
`GET /events` upgrades to a long-lived SSE connection, backed by an
in-process, per-player pub/sub `Hub` (one Go channel per active
subscription). It currently pushes defense alerts (e.g.
`tower_under_attack`) so an owner finds out instantly instead of waiting for
the next poll. A 20-second heartbeat comment keeps proxies from closing the
connection; `POST /events/test` publishes a sample event to your own stream
for manual QA.

The hub is single-instance/in-memory — if the server ever scales
horizontally, this needs a Redis-backed (or similar) fan-out instead.

## Background jobs

One binary, `cmd/ezra`, runs three things as goroutines in the same process:
the HTTP server, an asynq **worker** (task-type → handler routing via
`asynq.ServeMux`), and an asynq **scheduler** (cron-style `@every`
registrations). Redis is the job broker. There's no separate worker binary
to deploy.

| Interval | Task | Package |
|---|---|---|
| 1m | `pet:auto_claim` | pet |
| 1m | `squad:complete_missions` | squad |
| 2m | `symbiont:drain_tick` | symbiont |
| 5m | `infection:recalculate` | infection |
| 10m | `rift:spawn_organic` | rift |
| 10m | `roster:entity_tick` | roster |
| 30m | `infection:tide_advance` | infection |
| 1h | `rift:expand` | rift |
| 1h | `hive:pulse` | hive |
| 1h | `tower:accrue_passive_income` | tower |
| 1h | `tower:pressure_tick` | tower |
| 1h | `legacy:degrade` | legacy |
| 6h | `spire:lifecycle` | spire |
| 6h | `station:lifecycle` | station |
| 6h | `unit:army_decay` | unit |
| 24h | `factionwar:settle` | factionwar |
| 24h | `shop:expire_subscriptions` | shop |

## Data layer

- **PostgreSQL + PostGIS**: connected via `pgx` (`internal/platform/
  postgres.go`); repositories hand-write SQL, no ORM. PostGIS is enabled in
  the first migration; `cells.geom` is a `GEOMETRY(Point, 4326)` column with
  a `GIST` index for spatial queries. `uuid-ossp` provides `uuid_generate_v4()`
  for primary keys.
- **Redis**: sessions (see Auth above) and the asynq broker/scheduler.
- **Migrations**: [golang-migrate](https://github.com/golang-migrate/migrate),
  `migrations/`, strict `NNN_description.up.sql` / `.down.sql` pairs (~96
  files as of this writing). Run via `make migrate-up` / `make migrate-down`.
  There's no ORM-driven auto-migration — every schema change is a new
  numbered pair.

## Package map

| Package | Owns | Worker |
|---|---|---|
| `achievement` | Metric-based achievement tracking/unlocks | — |
| `auth` | Login (Firebase + password), Redis sessions | — |
| `battle` | Turn-based combat resolution vs rifts/hives | — |
| `bestiary` | Discovered-enemy/tower/spectrum catalog | — |
| `canon` | Central game-rule constants & pure functions | — |
| `capture` | Lockpick / force tower capture | — |
| `cell` | Map grid cells, lazy Overpass-based world seeding | — |
| `citylink` | Player-to-player alliance links | — |
| `faction` | Faction contact/choice/status | — |
| `factionwar` | Seasonal faction scoring | daily settle |
| `hive` | Symbiont hive lifecycle | hourly pulse |
| `infection` | Cell infection recalculation, "tide" mechanic | 5m recalc, 30m tide |
| `item` | Inventory | — |
| `legacy` | Legacy (ex-owned) beacon degradation | hourly degrade |
| `network` | Core placement/relocate/upgrade, region fields | — |
| `pet` | Pet claim/send/recall | 1m auto-claim |
| `platform` | Infra: Postgres, Redis, Firebase, Mapbox, Overpass, asynq, balance config | — |
| `player` | Profile, resources, onboarding, position/anti-cheat, skills | — |
| `push` | FCM push notifications | send queue |
| `pvp` | PvP target listing + dome breach | — |
| `quest` | Daily quests, streak check-in | — |
| `realtime` | SSE event stream | — |
| `resonance` | Symbiont Resonance Level status/activation | — |
| `rift` | Rift lifecycle | hourly expand, 10m organic spawn |
| `roster` | Symbiont entity roster | 10m entity tick |
| `shop` | IAP catalog, purchases, subscriptions | 24h expire |
| `spire` | Endgame Spire | 6h lifecycle |
| `squad` | Squad create/send/recall, missions | 1m complete missions |
| `station` | Player-built power plants | 6h lifecycle |
| `survivor` | Recruitable NPCs | — |
| `symbiont` | Symbiont status/raise/overload/recon | 2m drain tick |
| `tower` | Tower build/repair/delete, income, pressure | hourly income, hourly pressure |
| `unit` | Army units | 6h decay |

`pkg/` holds infra-agnostic shared code: `pkg/httputil` (HTTP helpers),
`pkg/middleware` (CORS/logging/rate-limit/recovery), `pkg/geo` (distance
math).

## Testing

Business logic is unit-tested with hand-written in-memory fakes for each
package's repository/collaborator interfaces — no mocking framework, no
docker-compose-based integration suite. See any `internal/*/service_test.go`
for the pattern (e.g. `internal/achievement/service_test.go`). Run everything
with `make test`.

## Release gating

The backend is pinned to one `canon.ReleaseStage` (`mvp` / `v1.0` / `v1.1`)
at a time; features can declare a minimum stage and get centrally gated via
`canon.IsFeatureAvailable`. Check `internal/canon/canon.go` for what's
currently live before assuming a feature is reachable.
