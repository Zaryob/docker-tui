#!/bin/sh
# Copyright (C) 2026 Suleyman Poyraz
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Renders the README screenshots from fixtures. Needs freeze
# (go install github.com/charmbracelet/freeze@latest) and, on macOS, sips
# for downscaling. Run from the repository root:
#
#	sh docs/screenshots/render.sh
set -eu
dir=$(cd "$(dirname "$0")" && pwd)
DOCKER_TUI_SCREENSHOTS="$dir" go test -count=1 -run '^TestScreenshots$' ./internal/ui/
for f in "$dir"/*.ansi; do
	name=$(basename "$f" .ansi)
	# The bundled font has no emoji; stand in with glyphs of the same width.
	sed -e 's/📦/◆ /g' -e 's/🔧/◇ /g' -e 's/🔍/» /g' -e 's/📋/≡ /g' \
		-e 's/💾/▽ /g' -e 's/📥/↓ /g' -e 's/ⓘ/i/g' -e 's/⇄/±/g' "$f" >"$f.tmp"
	freeze "$f.tmp" --language ansi -o "$dir/$name.png" --window --padding 16,24 \
		--border.radius 10 --font.size 13 --background '#0d1117' --margin 0
	rm -f "$f.tmp"
	if command -v sips >/dev/null; then
		sips -Z 1600 "$dir/$name.png" --out "$dir/$name.png" >/dev/null
	fi
done
