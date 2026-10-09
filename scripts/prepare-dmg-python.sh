#!/usr/bin/env bash
# Print only the ready-to-use interpreter path on stdout; diagnostics go to stderr.
set -euo pipefail
script_dir="$(cd "$(dirname "$0")" && pwd)"
requirements="$script_dir/dmg-requirements.txt"

supported() {
  "$1" -c 'import sys; sys.exit(sys.version_info < (3, 10))' >/dev/null 2>&1
}
ready() {
  "$1" - "$requirements" <<'PY' >/dev/null 2>&1
import sys
from importlib.metadata import version
import dmgbuild
for line in open(sys.argv[1]):
    line = line.strip()
    if line and not line.startswith('#'):
        name, expected = line.split('==')
        if version(name) != expected:
            sys.exit(1)
PY
}

if [[ -n "${DMG_PYTHON:-}" ]]; then
  if ! supported "$DMG_PYTHON" || ! ready "$DMG_PYTHON"; then
    echo "DMG_PYTHON must point to Python 3.10+ with scripts/dmg-requirements.txt installed." >&2
    echo "Install dependencies in that environment, or unset DMG_PYTHON to use automatic setup." >&2
    exit 1
  fi
  command -v "$DMG_PYTHON"
  exit 0
fi

venv="$script_dir/../build/dmg-venv"
if [[ -x "$venv/bin/python" ]] && supported "$venv/bin/python"; then
  python="$venv/bin/python"
else
  base=""
  for candidate in python3 python3.14 python3.13 python3.12 python3.11 python3.10 \
      /opt/homebrew/bin/python3 /usr/local/bin/python3; do
    if supported "$candidate"; then
      base="$candidate"
      break
    fi
  done
  if [[ -z "$base" ]]; then
    echo "DMG packaging requires Python 3.10+. Install Python (for example: brew install python) and retry." >&2
    exit 1
  fi
  echo "Preparing isolated DMG environment: $venv ($base)" >&2
  "$base" -m venv "$venv" >&2
  python="$venv/bin/python"
fi

if ! ready "$python"; then
  echo "Installing pinned DMG dependencies into $venv" >&2
  "$python" -m pip install --disable-pip-version-check -r "$requirements" >&2 || {
    echo "DMG dependency installation failed. Check network/pip mirror settings and retry; system Python was not modified." >&2
    exit 1
  }
fi
ready "$python" || { echo "DMG environment validation failed: $python" >&2; exit 1; }
printf '%s\n' "$python"
