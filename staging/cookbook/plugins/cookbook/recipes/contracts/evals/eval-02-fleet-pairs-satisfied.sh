#!/usr/bin/env bash
# eval-02 (contracts): fleet-doctor passes on a two-repo fleet whose
# provider/consumer pairs are satisfied, and reports the pairing.
set -euo pipefail
PLUGIN_ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
W="$(mktemp -d)"; trap 'rm -rf "$W"' EXIT
fail() { echo "EVAL FAIL: $*" >&2; exit 1; }

# Two sibling checkouts: api provides, web consumes.
for r in api web; do
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
add("api", "Provides", "- `acme.users.api-v1` — REST users API · openapi/users.yaml")
add("web", "Consumes", "- `acme.users.api-v1` — fetched by the web client")
# One constellation listing both members (run from web).
(w / "web" / "constellation.yaml").write_text(
    "fleet: acme\nrepos:\n  - name: web\n    path: .\n  - name: api\n    path: ../api\n")
PY

out="$(cd "$W/web" && bash scripts/fleet-doctor.sh constellation.yaml)" \
  || fail "fleet-doctor should pass on satisfied pairs"
echo "$out" | grep -q 'ok: `acme.users.api-v1` — api -> web' || fail "pairing not reported: $out"

# The contracts doctor folds the fleet check in.
"$PLUGIN_ROOT/recipes/contracts/scripts/doctor.sh" "$W/web" --tier S >/dev/null \
  || fail "contracts doctor should pass when the fleet is satisfied"

echo "PASS: contracts eval-02 (fleet pairs satisfied across sibling checkouts)"
