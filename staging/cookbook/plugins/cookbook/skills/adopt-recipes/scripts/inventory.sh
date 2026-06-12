#!/usr/bin/env bash
# inventory.sh <target-repo> — deterministic pre-adoption inventory.
#
# Detects language mix, build system, CI, existing process docs, and any prior
# recipe lock, then prints a `key: value` report plus a suggested tier. The
# adopt-recipes skill reads this instead of re-deriving the facts ad hoc.
# Read-only; exit 0 always (a thin repo is a finding, not an error).
set -euo pipefail

target="${1:-.}"
[ -d "$target" ] || { echo "usage: inventory.sh <target-repo>" >&2; exit 2; }
cd "$target"

echo "# Repo inventory: $(pwd)"
echo

# --- git ---------------------------------------------------------------------
if git rev-parse --git-dir >/dev/null 2>&1; then
  echo "git: yes"
  db="$(git symbolic-ref -q --short refs/remotes/origin/HEAD 2>/dev/null | sed 's|^origin/||' || true)"
  db="${db:-$(git symbolic-ref -q --short HEAD 2>/dev/null || echo unknown)}"
  echo "default_branch: $db"
  files=$(git ls-files | wc -l | tr -d ' ')
else
  echo "git: NO (init one before adopting)"
  files=$(find . -type f -not -path '*/.git/*' | wc -l | tr -d ' ')
fi
echo "tracked_files: $files"

# --- language mix (top extensions among source files) -------------------------
echo "language_mix:"
{ git ls-files 2>/dev/null || find . -type f -not -path '*/.git/*' | sed 's|^\./||'; } \
  | grep -E '\.(go|py|js|jsx|ts|tsx|mjs|rs|rb|java|kt|c|cc|cpp|h|cs|swift|sh)$' \
  | grep -vE '(^|/)(vendor|node_modules|dist|build|target)/' \
  | sed 's/.*\.//' | sort | uniq -c | sort -rn | head -5 \
  | awk '{printf "  - %s: %s files\n", $2, $1}'

# --- build system --------------------------------------------------------------
bs="none"
[ -f Makefile ] && bs="make"
[ -f package.json ] && { [ "$bs" = "none" ] && bs="npm" || bs="$bs+npm"; }
[ -f pyproject.toml ] && bs="$bs+python"
[ -f go.mod ] && bs="$bs+go"
[ -f Cargo.toml ] && bs="$bs+cargo"
echo "build_system: $bs"
if [ -f Makefile ]; then
  echo "make_targets: $(grep -oE '^[a-zA-Z][a-zA-Z0-9_-]*:' Makefile | tr -d ':' | tr '\n' ' ')"
fi
if [ -f package.json ]; then
  echo "npm_scripts: $(python3 -c "import json;print(' '.join(json.load(open('package.json')).get('scripts',{})))" 2>/dev/null || echo unparseable)"
fi

# --- CI ------------------------------------------------------------------------
if [ -d .github/workflows ]; then
  echo "ci_workflows: $(ls .github/workflows | tr '\n' ' ')"
else
  echo "ci_workflows: none"
fi

# --- existing process docs (harvest sources; NEVER clobber these) ---------------
echo "existing_docs:"
for f in CLAUDE.md AGENTS.md CONTRIBUTING.md README.md docs/CONVENTIONS.md \
         docs/CONTEXT.md docs/CONTRACTS.md docs/REGRESSIONS.md docs/DEV_LOOP.md \
         docs/RELEASING.md docs/issues/INDEX.md docs/adr constellation.yaml; do
  [ -e "$f" ] && echo "  - $f"
done

# --- prior adoption --------------------------------------------------------------
if [ -f .recipes/lock.json ]; then
  echo "recipe_lock: present"
  python3 -c "
import json
lock = json.load(open('.recipes/lock.json'))
for r in lock.get('recipes', []):
    print(f\"  - {r['name']} v{r['version']} tier {r['tier']}\")" 2>/dev/null || echo "  (unreadable)"
else
  echo "recipe_lock: none (fresh adoption)"
fi

# --- tier suggestion --------------------------------------------------------------
if [ "$files" -lt 30 ]; then sug="S"
elif [ "$files" -lt 300 ]; then sug="M"
else sug="L"; fi
echo
echo "suggested_tier: $sug  (S<30, M<300, L>=300 tracked files — override by judgment)"
