#!/bin/zsh
# deploy_remote.sh [--build]
# Syncs the Ezra server source to the production host and (optionally) rebuilds.
#   Real prod (2026-08-20+): NAS ssh cactus@192.168.0.100 → /home/cactus/docker/projects/ezra
#   Public entry: 94.228.120.50:8081 → nginx relay (ezra-relay project, NOT this
#   directory) → Tailscale → 100.64.0.2:8988 → this NAS. The OLD /data/ezra on
#   94.228.120.50 is a stale, non-live leftover — do not deploy there.
# Usage:
#   ./deploy_remote.sh           # rsync code only (then restart app if no Dockerfile/dep change)
#   ./deploy_remote.sh --build   # rsync + docker compose up -d --build
set -e
HOST=cactus@192.168.0.100
DEST=/home/cactus/docker/projects/ezra
SRC="$(cd "$(dirname "$0")" && pwd)/"

echo "→ rsync $SRC → $HOST:$DEST"
# firebase-credentials.json is a server-only secret, never present in this
# local checkout (not committed) — --delete would otherwise remove it on the
# remote every deploy (this happened once, 2026-08-20, caused an outage).
rsync -az --delete \
  --exclude='.tmp/' --exclude='/ezra' --exclude='/seed' --exclude='/bin/' \
  --exclude='.git/' --exclude='*.bak' --exclude='*.bak.*' --exclude='ezra.dump' \
  --exclude='/firebase-credentials.json' \
  --exclude='/public_builds/' \
  -e "ssh" \
  "$SRC" "$HOST:$DEST/"

if [[ "$1" == "--build" ]]; then
  echo "→ rebuild + restart on remote"
  ssh $HOST "cd $DEST && docker compose up -d --build"
else
  echo "→ restart app on remote (no rebuild)"
  ssh $HOST "cd $DEST && docker compose restart app"
fi
echo "→ health:"
curl -s -m 8 http://94.228.120.50:8081/health; echo
