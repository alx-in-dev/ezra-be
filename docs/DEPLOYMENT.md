# Deployment

## Image

`Dockerfile` is a two-stage build:

1. `golang:1.25-alpine` compiles a static binary:
   `CGO_ENABLED=0 GOOS=linux go build -o /ezra ./cmd/ezra`.
2. `alpine:3.19` runtime image carries just the binary plus `migrations/`
   and `config/`, exposes port `8080`, entrypoint `ezra`.

Migrations are **not** applied automatically on container start — they ship
with the image so you can run them explicitly (see below), but nothing
inside the container calls `migrate` for you.

## Production topology

Same `docker-compose.yml` used for local dev is used in production: `app` +
`postgres` (PostGIS) + `redis`, `app` published on host port `8081`. Swap the
dev-only env overrides (`MAX_SPEED_KMH`, `DOME_SUPPRESSION_PER_HOUR`) back to
production values before/while deploying — they currently live directly in
`docker-compose.yml`, so check that file rather than assuming this doc is
current.

## Current production host

`deploy_remote.sh` syncs this repo to a single VPS and restarts it there:

```bash
./deploy_remote.sh           # rsync code, restart the app container (no rebuild)
./deploy_remote.sh --build   # rsync code, docker compose up -d --build
```

- Target: `cactus@94.228.120.50:5378` (custom SSH port) → `/data/ezra`
- Excludes build artifacts and `.git` from the sync
  (`.tmp/`, `/ezra`, `/seed`, `/bin/`, backups)
- Ends with a health check: `curl http://94.228.120.50:8081/health`

You need SSH access to that host to deploy. This is a single-host setup —
there's no orchestration layer (no k8s, no load balancer) as of this
writing.

## Manual deploy checklist

1. `./deploy_remote.sh --build` from your machine (or CI, if that's set up
   later — nothing automated exists yet).
2. If the change includes a new migration, SSH in and run it against the
   production database before/after the app restarts, depending on whether
   it's backward-compatible with the currently-running binary:
   ```bash
   ssh -p 5378 cactus@94.228.120.50
   cd /data/ezra
   DATABASE_URL="<prod dsn>" migrate -path migrations -database "$DATABASE_URL" up
   ```
3. Confirm `curl http://94.228.120.50:8081/health` returns `{"status":"ok"}`
   (the script does this automatically, but re-check after a migration).

## Secrets on the server

`firebase-credentials.json` must exist on the production host at the repo
root (mounted read-only into `app` per `docker-compose.yml`). It's gitignored
— never committed — but `deploy_remote.sh` does **not** exclude it from the
rsync, so whatever copy sits in your local `server/` directory when you run
the script overwrites the one on the host. Keep the production credentials
file only on machines/hosts that should hold it, and double-check you're not
about to sync a dev/empty version over a working production one. If it's
missing or invalid, Firebase login fails but login/password auth keeps
working.
