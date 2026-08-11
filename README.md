# Ezra — Backend

Backend for **Ezra**, a geo-based MMO (real-world map, GPS-driven base building,
territory infection, PvP/PvE). This repo is the Go server only — the game
client lives in a separate repo.

## Stack

| Layer | Tech |
|---|---|
| Language | Go 1.25 |
| HTTP router | [chi](https://github.com/go-chi/chi) |
| Database | PostgreSQL + PostGIS |
| Cache / sessions / job queue | Redis |
| Background jobs | [asynq](https://github.com/hibiken/asynq) (in-process worker + scheduler) |
| Auth | Firebase ID tokens **or** login/password (bcrypt) |
| Push notifications | Firebase Cloud Messaging |
| Realtime | Server-Sent Events (`GET /events`) |
| Migrations | [golang-migrate](https://github.com/golang-migrate/migrate) |

One binary (`cmd/ezra`) serves HTTP, runs the asynq worker, and runs the
asynq scheduler — all as goroutines in the same process. There is no
separate worker binary.

## Quickstart (local dev)

Requires Docker and Go 1.25+.

```bash
# 1. Start Postgres + Redis + the app in containers
cp .env.example .env   # only needed if you run the app outside Docker
docker compose up -d

# 2. Run migrations (from your host, against the containerized Postgres)
export DATABASE_URL="postgres://ezra:ezra@localhost:5432/ezra?sslmode=disable"
make migrate-up

# 3. Check it's alive
curl http://localhost:8081/health
```

`docker-compose.yml` maps the app to host port `8081` and loosens the
anti-cheat / infection timers for fast local iteration (see
[docs/SETUP.md](docs/SETUP.md)).

To run the Go server directly on your host instead of in Docker:

```bash
export DATABASE_URL="postgres://ezra:ezra@localhost:5432/ezra?sslmode=disable"
export REDIS_URL="redis://localhost:6379"
make run   # go run ./cmd/ezra
```

No Firebase credentials are required to get a working local server — see
[docs/SETUP.md](docs/SETUP.md) for the login/password auth path, and only set
up Firebase if you specifically need to test that flow.

## Project layout

```
cmd/ezra/       entrypoint: wires every package and starts HTTP + workers
cmd/bot/        headless bot swarm for manual multiplayer testing (not shipped)
internal/       one package per game domain (handler → service → repository)
internal/canon/ central game-rule constants shared across packages
internal/platform/  infra wiring: Postgres, Redis, Firebase, asynq, Overpass
pkg/            small shared libraries (HTTP helpers, middleware, geo math)
migrations/     golang-migrate SQL migrations, one numbered pair per change
config/         balance.yaml — tunable game-balance numbers
```

## Documentation

Start here if you're new to the project:

1. [docs/DOMAIN_GLOSSARY.md](docs/DOMAIN_GLOSSARY.md) — what a Rift, Tower,
   Faction, Symbiont etc. actually are, in engineering terms.
2. [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — how the code is laid out,
   how a request flows through it, background jobs, realtime.
3. [docs/API.md](docs/API.md) — the full REST surface.
4. [docs/SETUP.md](docs/SETUP.md) — local dev environment in detail.
5. [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) — how this ships to production.

`docs/legacy/` holds older planning-era docs kept for historical context —
they predate large parts of the current implementation, so treat the docs
above as the source of truth when they disagree.

## Testing

```bash
make test   # go test ./...
```

Business logic is unit-tested with hand-written fakes for repository/
collaborator interfaces (no mocking framework, no test containers). See
`internal/*/service_test.go` for examples.
