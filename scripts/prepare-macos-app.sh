#!/usr/bin/env bash
# Complete all bundle mutations before sealing the app for DMG packaging.
set -euo pipefail

[[ $# -eq 1 ]] || { echo "Usage: $0 /path/to/gemsnote.app" >&2; exit 2; }
app="$1"
[[ -d "$app/Contents/MacOS" && -f "$app/Contents/Info.plist" ]] || {
  echo "macOS application bundle not found: $app" >&2
  exit 1
}
command -v codesign >/dev/null 2>&1 || { echo "Required tool not found: codesign" >&2; exit 1; }

resources="$app/Contents/Resources"
mkdir -p "$resources/en.lproj" "$resources/zh-Hans.lproj" "$resources/zh-Hant.lproj"
printf '%s\n' '"CFBundleDisplayName" = "Gemsnote";' '"CFBundleName" = "Gemsnote";' > "$resources/en.lproj/InfoPlist.strings"
printf '%s\n' '"CFBundleDisplayName" = "珠玑笔记";' '"CFBundleName" = "珠玑笔记";' > "$resources/zh-Hans.lproj/InfoPlist.strings"
cp "$resources/zh-Hans.lproj/InfoPlist.strings" "$resources/zh-Hant.lproj/InfoPlist.strings"

# Wails has already ad-hoc signed the bundle. Adding resources invalidates that
# seal, so re-sign after localization and fail packaging if verification fails.
# This is integrity sealing only, NOT Developer ID signing or notarization.
codesign --force --sign - --timestamp=none "$app"
codesign --verify --deep --strict --verbose=2 "$app"
