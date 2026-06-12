#!/usr/bin/env bash
# lint.sh — repo lint gate for orchestra-recipes.
#
#   1. bash -n every shell script (including .sh.tmpl templates).
#   2. No hardcoded secrets (token-shaped strings) anywhere tracked.
#   3. No hardcoded calendar dates (docs must not be time-sensitive; scripts
#      must compute dates at runtime).
#
# Exit non-zero with a readable report on any violation.
set -uo pipefail
cd "$(dirname "$0")/.."

fail=0

echo "== bash -n on every script"
while IFS= read -r f; do
  if ! bash -n "$f" 2>/tmp/lint-err.$$; then
    echo "ERROR: $f does not parse:"
    sed 's/^/    /' /tmp/lint-err.$$
    fail=1
  fi
done < <(git ls-files -co --exclude-standard -- '*.sh' '*.sh.tmpl' 2>/dev/null)
rm -f /tmp/lint-err.$$
[ "$fail" -eq 0 ] && echo "   all scripts parse"

# --no-index: scan the working tree (tracked + untracked, .gitignore respected),
# so the gate works before the first commit too.
echo "== secret scan"
if git grep --no-index --exclude-standard -nIE '(ghp_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|AKIA[0-9A-Z]{16}|xox[bpars]-[A-Za-z0-9-]{10,}|-----BEGIN (RSA|EC|OPENSSH) PRIVATE KEY)' -- . ; then
  echo "ERROR: token-shaped string found (see above) — secrets never live in the repo"
  fail=1
else
  echo "   no token-shaped strings"
fi

echo "== hardcoded-date scan"
if git grep --no-index --exclude-standard -nIE '20[0-9]{2}-[0-9]{2}-[0-9]{2}' -- . ':!*.json'; then
  echo "ERROR: literal calendar date found (see above) — recipes must not carry time-sensitive text; compute dates at runtime"
  fail=1
else
  echo "   no literal dates"
fi

if [ "$fail" -ne 0 ]; then
  echo "lint FAILED" >&2
  exit 1
fi
echo "lint passed"
