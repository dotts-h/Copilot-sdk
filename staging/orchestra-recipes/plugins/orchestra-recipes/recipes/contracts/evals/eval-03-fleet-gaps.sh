#!/usr/bin/env bash
# eval-03 (contracts): fleet-doctor fails loud on an unsatisfied consume, a
# double provider, and a missing member checkout — naming each offender.
set -euo pipefail
PLUGIN_ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
W="$(mktemp -d)"; trap 'rm -rf "$W"' EXIT
fail() { echo "EVAL FAIL: $*" >&2; exit 1; }

for r in api web worker; do
  git init -q -b main "$W/$r"
  "$PLUGIN_ROOT/scripts/install-recipe.sh" \
    --recipe "$PLUGIN_ROOT/recipes/contracts" --target "$W/$r" --tier S \
    --answer "project_name=$r" --answer fleet_member=true --answer fleet_name=acme >/dev/null
done

python3 - "$W" <<'PY'
import sys, re, pathlib
w = pathlib.Path(sys.argv[1])
def add(repo, section, bullet):
    p = w / repo / "docs" / "CONTRACTS.md"
    txt = p.read_text().splitlines()
    out, inside = [], False
    for ln in txt:
        if re.match(rf"## {section}\b", ln): inside = True
        elif inside and ln.startswith("## "): inside = False
        if inside and ln.strip() == "- *(none yet)*":
            out.append(bullet); inside = False; continue
        out.append(ln)
    p.write_text("\n".join(out) + "\n")
# web consumes something nobody provides; api+worker both provide the same id.
add("web", "Consumes", "- `acme.billing.events-v2` — needed by the dashboard")
add("api", "Provides", "- `acme.queue.jobs-v1` — job queue schema")
add("worker", "Provides", "- `acme.queue.jobs-v1` — job queue schema (duplicate!)")
(w / "web" / "constellation.yaml").write_text(
    "fleet: acme\nrepos:\n"
    "  - name: web\n    path: .\n"
    "  - name: api\n    path: ../api\n"
    "  - name: worker\n    path: ../worker\n"
    "  - name: ghost\n    path: ../ghost\n")
PY

set +e
out="$(cd "$W/web" && bash scripts/fleet-doctor.sh constellation.yaml 2>&1)"
rc=$?
set -e
[ "$rc" -eq 1 ] || fail "fleet-doctor should exit 1 on gaps (got $rc)"
echo "$out" | grep -q 'GAP: `acme.billing.events-v2` is consumed by .web. but provided by NO fleet member' \
  || fail "unsatisfied consume not named"
echo "$out" | grep -q 'GAP: contract `acme.queue.jobs-v1` is provided by multiple repos: api, worker' \
  || fail "double provider not named"
echo "$out" | grep -q "GAP: member 'ghost' path not found" || fail "missing member checkout not named"

set +e
"$PLUGIN_ROOT/recipes/contracts/scripts/doctor.sh" "$W/web" --tier S >/dev/null 2>&1
drc=$?
set -e
[ "$drc" -eq 1 ] || fail "contracts doctor should surface fleet gaps (got $drc)"

echo "PASS: contracts eval-03 (fleet gaps named: no-provider, double-provider, missing member)"
