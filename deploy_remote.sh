#!/bin/zsh
# deploy_remote.sh [--build]
# Syncs the Ezra server source to the production host and (optionally) rebuilds.
#   ssh cactus@94.228.120.50 -p 5378  →  /data/ezra
# Usage:
#   ./deploy_remote.sh           # rsync code only (then restart app if no Dockerfile/dep change)
#   ./deploy_remote.sh --build   # rsync + docker compose up -d --build
set -e
HOST=cactus@94.228.120.50
PORT=5378
DEST=/data/ezra
SRC="$(cd "$(dirname "$0")" && pwd)/"

echo "→ rsync $SRC → $HOST:$DEST"
rsync -az --delete \
  --exclude='.tmp/' --exclude='/ezra' --exclude='/seed' --exclude='/bin/' \
  --exclude='.git/' --exclude='*.bak' --exclude='*.bak.*' --exclude='ezra.dump' \
  -e "ssh -p $PORT" \
  "$SRC" "$HOST:$DEST/"

if [[ "$1" == "--build" ]]; then
  echo "→ rebuild + restart on remote"
  ssh -p $PORT $HOST "cd $DEST && docker compose up -d --build"
else
  echo "→ restart app on remote (no rebuild)"
  ssh -p $PORT $HOST "cd $DEST && docker compose restart app"
fi
echo "→ health:"
curl -s -m 8 http://94.228.120.50:8081/health; echo
