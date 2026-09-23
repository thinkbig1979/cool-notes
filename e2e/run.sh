#!/usr/bin/env bash
# Runs the tui-goggles smoke script against a fresh notes file.
# Needs tui-goggles (the tui-capture skill binary) on PATH or at $TG.
set -euo pipefail
cd "$(dirname "$0")/.."
TG=${TG:-$(command -v tui-goggles || echo ~/.claude/skills/tui-capture/bin/tui-goggles)}
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
go build -o "$tmp/cool-note" .
"$TG" -trim -delay 800ms -env COOL_NOTE_CONFIG_DIR="$tmp/cfg" \
	-script e2e/smoke.script -- "$tmp/cool-note" --file "$tmp/notes.txt"
echo "--- notes file"
cat "$tmp/notes.txt"
