#!/usr/bin/env bash
# Rebuilds the shared Vue SPA (repo-root frontend/) into the Wails embed dir.
# Usage: bash build-frontend.sh  (run from this directory, or via wails build)
set -euo pipefail
cd "$(dirname "$0")"

ROOT="$(cd .. && pwd)"
DESKTOP_VERSION="$(tr -d '[:space:]' < api/VERSION)"
[[ "$DESKTOP_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || {
  echo "Invalid Desktop version in api/VERSION: $DESKTOP_VERSION" >&2
  exit 1
}

if [ ! -d "$ROOT/frontend/node_modules" ]; then
  npm ci --prefix "$ROOT/frontend"
fi
VITE_GEMSNOTE_DESKTOP_VERSION="$DESKTOP_VERSION" npm run build --prefix "$ROOT/frontend"

rm -rf frontend/dist
mkdir -p frontend/dist
cp -r "$ROOT/frontend/dist/"* frontend/dist/
cp -r "$ROOT/public/tinymce" frontend/dist/tinymce
echo "frontend/dist updated for Desktop $DESKTOP_VERSION: $(du -sh frontend/dist | cut -f1)"
