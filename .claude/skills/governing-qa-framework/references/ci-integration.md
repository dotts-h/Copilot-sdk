# CI integration

The framework only protects you if it runs on every change and its failures are debuggable. Check the
browser suite is wired into CI with artifacts.

## What good looks like
- A dedicated workflow (`.github/workflows/e2e.yml`) that: checks out, sets up Node, `npm ci` in `e2e/`,
  installs the Chromium browser (`npx playwright install --with-deps chromium`), builds the app binary,
  and runs `npx playwright test` (or `make e2e`).
- **Artifacts on failure:** upload `playwright-report/` and `test-results/` (traces) so a red CI run is
  diagnosable without reproducing locally.
- The Go contract/bench tests run in the main CI via `go test ./...`; the browser suite is its own job so a
  browser flake doesn't mask a unit failure (and vice-versa).

## Makefile parity
Keep `make e2e` and `make e2e-install` so local and CI run the identical commands — divergence between
"works locally" and CI is its own failure mode. This repo already has both targets.

## Flake policy
- `retries: 2` in CI absorbs infra blips, but a test that only passes on retry is a flake to fix, not
  accept — surface retried tests in the report and hand them to `hardening-tests`.
- Don't let the e2e job be allowed-to-fail ("continue-on-error") — a green-by-ignoring suite protects nothing.

## Verify
`framework-audit.sh` checks that an e2e workflow exists and uploads artifacts; eyeball that the browser
install step pins the same Playwright version as `e2e/package.json`.
