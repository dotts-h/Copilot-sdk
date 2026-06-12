#!/usr/bin/env bash
# eval-02 (release): the doctor catches the learned regression — a version step
# that shadows the dispatch input with ${GITHUB_REF_NAME:-...}.
set -euo pipefail
PLUGIN_ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
T="$(mktemp -d)"; trap 'rm -rf "$T"' EXIT
fail() { echo "EVAL FAIL: $*" >&2; exit 1; }

git init -q -b main "$T"
"$PLUGIN_ROOT/scripts/install-recipe.sh" \
  --recipe "$PLUGIN_ROOT/recipes/release" --target "$T" --tier S >/dev/null

# Regress the version step to the buggy shell-default form.
python3 - "$T/.github/workflows/release.yml" <<'PY'
import sys
p = sys.argv[1]
txt = open(p).read()
txt = txt.replace(
    'echo "version=${{ github.event.inputs.tag || github.ref_name }}"',
    'echo "version=${GITHUB_REF_NAME:-${{ github.event.inputs.tag }}}"')
open(p, "w").write(txt)
PY

set +e
out="$("$PLUGIN_ROOT/recipes/release/scripts/doctor.sh" "$T" --tier S 2>&1)"
rc=$?
set -e
[ "$rc" -eq 1 ] || fail "doctor should exit 1 on the version-resolution bug (got $rc)"
echo "$out" | grep -q "GITHUB_REF_NAME:-" || fail "the buggy form was not named in the gap report"
echo "$out" | grep -q "inputs.tag || github.ref_name" || fail "the missing correct form was not flagged"

echo "PASS: release eval-02 (doctor catches the mis-tag regression)"
