#!/usr/bin/env bash
# framework-audit.sh [e2e-dir] — audit the Playwright harness against the standards.
# Read-only. Each ✗ is a fix to make; output is a quick health snapshot.
set -uo pipefail
e2e="${1:-e2e}"
[ -d "$e2e" ] || { echo "no $e2e/ dir" >&2; exit 1; }
cfg=$(ls "$e2e"/playwright.config.* 2>/dev/null | head -1)

ok(){ echo "  ✓ $1"; }; bad(){ echo "  ✗ $1"; }

echo "=== config ($cfg) ==="
if [ -n "$cfg" ]; then
  grep -q "trace:" "$cfg" && ok "trace configured" || bad "no trace (set trace: 'on-first-retry')"
  grep -q "reporter" "$cfg" && ok "reporter set" || bad "no reporter"
  grep -q "retries" "$cfg" && ok "retries set" || bad "no retries (CI blips will fail builds)"
  grep -qE "screenshot|video" "$cfg" && ok "failure artifacts" || bad "no screenshot/video on failure"
  grep -q "webServer" "$cfg" && ok "webServer manages the app" || bad "no webServer (manual server start)"
else bad "no playwright.config.* found"; fi

echo; echo "=== locator / wait discipline ==="
hw=$(grep -rn "waitForTimeout" "$e2e/tests" 2>/dev/null | wc -l)
[ "$hw" -eq 0 ] && ok "no hard waits" || { bad "$hw hard wait(s) — replace with web-first assertions:"; grep -rn "waitForTimeout" "$e2e/tests" | sed 's/^/      /'; }
grep -rqE "getByRole|getByTestId|getByLabel" "$e2e/tests" 2>/dev/null && ok "uses role/test-id locators" || bad "few role/test-id locators (brittle selectors?)"

echo; echo "=== fixtures / seed ==="
[ -f "$e2e/tests/seed.spec.ts" ] && ok "seed test present" || bad "no tests/seed.spec.ts"
ls "$e2e"/tests/helpers.* >/dev/null 2>&1 && ok "shared helpers/fixtures" || bad "no shared helpers (boilerplate likely duplicated)"

echo; echo "=== agents ==="
ls "$e2e"/.github/*agent* "$e2e"/*agent* >/dev/null 2>&1 && ok "agent definitions present" || bad "run: npx playwright init-agents --loop=claude"

echo; echo "=== CI ==="
ls .github/workflows/e2e.y*ml >/dev/null 2>&1 && ok "e2e workflow exists" || bad "no e2e CI workflow"
grep -rq "upload-artifact" .github/workflows/ 2>/dev/null && ok "uploads artifacts" || bad "CI doesn't upload trace/report on failure"
