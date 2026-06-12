#!/usr/bin/env bash
# run-doctors.sh <target-repo> [--all] — aggregate conformance report.
#
# Runs each installed recipe's doctor.sh (per .recipes/lock.json, with the
# locked tier) against the target and aggregates the results. With --all (or
# when no lock exists), runs every recipe's doctor at default tier instead —
# useful as a pre-adoption survey.
#
# Exit: 0 all green · 1 any doctor reported gaps · 2 usage error.
set -uo pipefail

PLUGIN_ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
target="${1:-}"; shift || true
all=0
while [ $# -gt 0 ]; do case "$1" in
  --all) all=1; shift;;
  *) echo "unknown arg: $1" >&2; exit 2;;
esac; done
[ -n "$target" ] && [ -d "$target" ] || { echo "usage: run-doctors.sh <target-repo> [--all]" >&2; exit 2; }

declare -a names tiers
if [ "$all" -eq 0 ] && [ -f "$target/.recipes/lock.json" ]; then
  while IFS=$'\t' read -r n t; do
    names+=("$n"); tiers+=("$t")
  done < <(python3 - "$target/.recipes/lock.json" <<'PY'
import json, sys
lock = json.load(open(sys.argv[1]))
for r in lock.get("recipes", []):
    print(f"{r['name']}\t{r.get('tier','M')}")
PY
)
  [ ${#names[@]} -gt 0 ] || { echo "lock has no recipes — nothing to check (try --all)"; exit 0; }
  echo "== recipe-doctor: ${#names[@]} installed recipe(s) per .recipes/lock.json"
else
  for d in "$PLUGIN_ROOT"/recipes/*/; do
    names+=("$(basename "$d")"); tiers+=("")
  done
  echo "== recipe-doctor: no lock used — running ALL ${#names[@]} recipe doctors (survey mode)"
fi

failed=()
for i in "${!names[@]}"; do
  n="${names[$i]}"; t="${tiers[$i]}"
  doctor="$PLUGIN_ROOT/recipes/$n/scripts/doctor.sh"
  if [ ! -f "$doctor" ]; then
    echo "WARN: no doctor for recipe '$n' (not in this plugin version?)"
    continue
  fi
  echo
  if [ -n "$t" ]; then
    bash "$doctor" "$target" --tier "$t"
  else
    bash "$doctor" "$target"
  fi
  rc=$?
  if [ "$rc" -eq 1 ]; then failed+=("$n")
  elif [ "$rc" -ge 2 ]; then echo "WARN: doctor for '$n' errored (rc=$rc)"; failed+=("$n")
  fi
done

echo
if [ ${#failed[@]} -gt 0 ]; then
  echo "== recipe-doctor: GAPS in: ${failed[*]}"
  exit 1
fi
echo "== recipe-doctor: all green"
