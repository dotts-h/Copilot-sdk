#!/usr/bin/env bash
# doctor.sh (release) — conformance check for the `release` recipe.
#
#   doctor.sh <target-repo> [--tier S|M|L]
#
# Exit codes: 0 conformant · 1 gaps (readable "GAP:" lines) · 2 usage error.
set -uo pipefail

target="${1:-}"; shift || true
tier=""
while [ $# -gt 0 ]; do case "$1" in
  --tier) tier="$2"; shift 2;;
  *) echo "unknown arg: $1" >&2; exit 2;;
esac; done
[ -n "$target" ] && [ -d "$target" ] || { echo "usage: doctor.sh <target-repo> [--tier S|M|L]" >&2; exit 2; }
tier="${tier:-M}"

gaps=0
gap() { echo "GAP: $*"; gaps=$((gaps+1)); }
ok()  { echo "  ok: $*"; }

echo "== release doctor (tier $tier) on $target"

# Find the release workflow: any workflow with a push.tags trigger.
rel=""
for wf in "$target"/.github/workflows/*.yml "$target"/.github/workflows/*.yaml; do
  [ -e "$wf" ] || continue
  if grep -qE '^\s*tags:' "$wf"; then rel="$wf"; break; fi
done

if [ -z "$rel" ]; then
  gap "no workflow with a push.tags trigger — releases aren't tag-driven (install the release recipe)"
else
  ok "tag-driven release workflow: ${rel#"$target"/}"

  grep -q 'workflow_dispatch' "$rel" \
    && ok "manual dispatch path present (sandboxes can release without a tag push)" \
    || gap "${rel#"$target"/}: no workflow_dispatch trigger — a blocked tag push leaves no release path"

  if grep -v '^[[:space:]]*#' "$rel" | grep -q 'github\.event\.inputs\.tag *|| *github\.ref_name'; then
    ok "version resolves dispatch-input-first (inputs.tag || ref_name)"
  else
    gap "${rel#"$target"/}: version step does not resolve 'github.event.inputs.tag || github.ref_name' — a dispatched release can be mis-tagged"
  fi

  if grep -v '^[[:space:]]*#' "$rel" | grep -q 'GITHUB_REF_NAME:-'; then
    gap "${rel#"$target"/}: uses the \${GITHUB_REF_NAME:-…} form — on dispatch this tags the release after the *branch*"
  else
    ok "no GITHUB_REF_NAME:- shadowing"
  fi

  grep -q 'checksums' "$rel" \
    && ok "checksums published with the artifacts" \
    || gap "${rel#"$target"/}: no checksums step — published artifacts should be verifiable"
fi

if [ "$tier" != "S" ]; then
  [ -f "$target/docs/RELEASING.md" ] \
    && ok "docs/RELEASING.md present" \
    || gap "docs/RELEASING.md missing — SemVer rules + verify-after checklist have no home (tier $tier)"
fi

echo "== release doctor: $gaps gap(s)"
[ "$gaps" -eq 0 ] || exit 1
