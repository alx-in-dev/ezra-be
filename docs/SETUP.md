# Local Development Setup

## Prerequisites

- Go 1.25+
- Docker + Docker Compose (for Postgres/Redis, or the whole stack)
- `golang-migrate` CLI if you want to run migrations from your host
  (`brew install golang-migrate` or see the
  [migrate docs](https://github.com/golang-migrate/migrate)) — not needed if
  you only ever run the app in Docker, since migrations aren't applied
  automatically by the container either way; you still run `make migrate-up`
  against the exposed Postgres port.

## Option A — everything in Docker

```bash
docker compose up -d
```

This starts three containers:

- `app` — builds from the local `Dockerfile`, exposed on host port `8081`
  (container listens on `8080`).
- `postgres` — `postgis/postgis:16-3.4`, exposed on `5432`.
- `redis` — `redis:7-alpine`, exposed on `6379`.

`app` waits for both dependencies to pass their healthchecks before starting.
It also sets two dev-friendly overrides you should know about:

- `MAX_SPEED_KMH=100000` — the anti-cheat speed cap is effectively disabled,
  so GPS-simulator tools ("FakeGPS") can teleport your test position without
  tripping `impossible_speed`.
- `DOME_SUPPRESSION_PER_HOUR=600` — domes clear infection in minutes instead
  of the production ~30%/hour, so you don't wait around to see suppression
  work.

Then run migrations against the exposed Postgres port:

```bash
export DATABASE_URL="postgres://ezra:ezra@localhost:5432/ezra?sslmode=disable"
make migrate-up
```

`docker-compose.yml` also mounts `./firebase-credentials.json` read-only into
the container — you don't need to provide this file unless you're testing the
Firebase login path (see [Auth](#auth) below); without it, the server logs
"firebase auth is not configured" and the login/password path still works
fine.

## Option B — Go on your host, infra in Docker

```bash
docker compose up -d postgres redis
export DATABASE_URL="postgres://ezra:ezra@localhost:5432/ezra?sslmode=disable"
export REDIS_URL="redis://localhost:6379"
make migrate-up
make run   # go run ./cmd/ezra, listens on :8080 by default
```

## Environment variables

All have working defaults except where noted; only set what you need.

| Var | Default | Purpose |
|---|---|---|
| `DATABASE_URL` | `postgres://ezra:ezra@localhost:5432/ezra?sslmode=disable` | Postgres DSN |
| `REDIS_URL` | `redis://localhost:6379` | Redis — sessions + asynq broker |
| `PORT` | `8080` | HTTP listen port |
| `FIREBASE_CREDENTIALS_FILE` | `firebase-credentials.json` | Firebase Admin SDK service-account file (only needed for the Firebase login path) |
| `MAPBOX_TOKEN` | `""` | Mapbox, server-side use |
| `OVERPASS_ENDPOINTS` | built-in defaults | Comma-separated Overpass (OSM) API URLs used for lazy cell/region seeding |
| `MAX_SPEED_KMH` | `50.0` | Anti-cheat position-update speed cap (km/h) |
| `DOME_SUPPRESSION_PER_HOUR` | canon default (`30.0`) | Infection suppression rate under an active dome |
| `EZRA_ALLOW_DEV_IAP` | unset | If `"1"`, `/shop/buy` accepts a fake `"dev"` purchase receipt for local IAP testing |

`cmd/bot` (the debug bot swarm, not the game server) has its own env vars —
see `cmd/bot/README.md`.

## Auth

You don't need a Firebase project to develop locally. `POST /auth/register`
with a `login`/`password` body creates a password-based player:

```bash
curl -s -X POST http://localhost:8081/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"login":"alice","password":"hunter22"}'
# → { "player": {...}, "session_token": "..." }

curl -s http://localhost:8081/api/v1/player \
  -H 'Authorization: Bearer <session_token>'
```

If you specifically need to exercise the Firebase login path (e.g. testing
client integration), set up a Firebase project, place its service-account
JSON as `firebase-credentials.json` at the repo root (already gitignored —
never commit it), and set `FIREBASE_CREDENTIALS_FILE` if you put it
somewhere else.

## Seeding the map

There's no seed script — cells are created lazily. The first
`GET /map/cells?lat=...&lng=...&radius_km=...` for a new ~0.1° region
triggers `cell.Seeder` to query Overpass and populate that area. To warm up
a region manually:

```bash
curl 'http://localhost:8081/api/v1/map/cells?lat=53.13&lng=50.15&radius_km=1.5' \
  -H 'Authorization: Bearer <session_token>'
```

## Exercising multiplayer features

`cmd/bot` is a headless bot swarm that plays the game over the same public
REST API a real client uses — useful for generating traffic to test
faction war, PvP, capture, or general crowd density without multiple real
devices. See `cmd/bot/README.md` for usage; it's a separate `go run`
target, not part of the production server.

## Tests

```bash
make test
```
