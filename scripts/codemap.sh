#!/usr/bin/env bash
# codemap.sh — generate docs/CODEMAP.md, a per-package symbol index.
#
# A session should read CODEMAP.md to learn the layout (which file holds which
# type/func) instead of opening source files to find out. Regenerate with
# `make codemap` after adding/moving/renaming top-level declarations.
#
# Pure text extraction (top-level `type`/`func` decls per non-test .go file);
# no build, no network.
set -euo pipefail

cd "$(dirname "$0")/.."
out="docs/CODEMAP.md"

{
	echo "# CODEMAP — generated, do not edit by hand"
	echo
	echo "> Regenerate with \`make codemap\`. A per-package index of every top-level"
	echo "> \`type\`/\`func\` (with its file and line count) so a session can learn the"
	echo "> layout from this one file instead of opening source to find a symbol. Read"
	echo "> this first; jump straight to \`file:symbol\`. The source is the source of"
	echo "> truth — if this looks stale, re-run \`make codemap\`."
	echo
	echo "_Last generated: $(date -u +%Y-%m-%d) (UTC)._"
	echo

	# Stable, sorted list of packages (dirs holding non-test .go files).
	pkgs=$(find cmd internal -name '*.go' ! -name '*_test.go' -exec dirname {} \; | sort -u)
	for pkg in $pkgs; do
		echo "## $pkg"
		echo
		for f in $(find "$pkg" -maxdepth 1 -name '*.go' ! -name '*_test.go' | sort); do
			loc=$(wc -l <"$f" | tr -d ' ')
			base=$(basename "$f")
			# Top-level type/func declarations, signature only (strip trailing " {").
			decls=$(grep -nE '^(type|func) ' "$f" 2>/dev/null | sed -E 's/[[:space:]]*\{[[:space:]]*$//' || true)
			echo "### $base ($loc LOC)"
			if [ -n "$decls" ]; then
				# Render "  - L<line>: <signature>"
				echo "$decls" | sed -E 's/^([0-9]+):(.*)$/- L\1: `\2`/'
			else
				echo "- _(no top-level type/func declarations)_"
			fi
			echo
		done
	done
} >"$out"

echo "wrote $out"
