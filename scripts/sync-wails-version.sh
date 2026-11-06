#!/usr/bin/env bash
set -euo pipefail

desktop_dir="$(cd "$(dirname "$0")/.." && pwd)"
version="$(tr -d '[:space:]' < "$desktop_dir/api/VERSION")"
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || {
  echo "Invalid api/VERSION: $version" >&2
  exit 1
}

config="$desktop_dir/wails.json"
tmp="$(mktemp "${TMPDIR:-/tmp}/gemsnote-wails.XXXXXX")"
awk -v version="$version" '
  /"productVersion"[[:space:]]*:/ {
    sub(/"productVersion"[[:space:]]*:[[:space:]]*"[^"]*"/, "\"productVersion\": \"" version "\"")
    found++
  }
  { print }
  END { if (found != 1) exit 2 }
' "$config" > "$tmp" || {
  rm -f "$tmp"
  echo "Expected exactly one info.productVersion in $config" >&2
  exit 1
}
if ! cmp -s "$tmp" "$config"; then
  mv "$tmp" "$config"
  echo "Updated wails.json productVersion to $version"
else
  rm -f "$tmp"
fi
