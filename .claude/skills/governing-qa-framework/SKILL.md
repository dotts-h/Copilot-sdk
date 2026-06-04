---
name: governing-qa-framework
description: Audits and upgrades the Playwright test framework against current best practices — config (projects, reporters, trace/video, retries), fixtures, role/test-id locator strategy, web-first assertions, no hard waits, parallelization, CI wiring, and the planner/generator/healer agent setup. Use this whenever setting up, auditing, or modernizing the test harness itself (not when writing an individual test — that's authoring-tests).
allowed-tools: Read, Write, Edit, Bash, Grep, Glob
---

# Governing the QA framework

Individual tests are only as good as the harness they run in. This skill owns the *framework's* health:
the config, fixtures, locator conventions, parallelization, CI wiring, and the agent toolchain. Get
these right and every test the team writes is faster, less flaky, and easier to debug. It rarely writes
a test — it writes config, fixtures, and CI.

## Audit pass
```bash
scripts/framework-audit.sh        # flags hard waits, missing trace, weak locators, CI gaps
```
Then review against the standards:

1. **Config** — projects, a useful reporter (list + HTML), `trace: 'on-first-retry'`, `retries` in CI,
   `screenshot/video` on failure, sensible timeouts. See
   [references/playwright-config-standards.md](references/playwright-config-standards.md).
2. **Locator strategy** — role and test-id locators, **web-first assertions** (`expect(locator).toBeVisible()`),
   **no `waitForTimeout`**. Hard waits are the top flake source. See
   [references/locator-strategy.md](references/locator-strategy.md).
3. **Fixtures** — shared setup in `tests/helpers.ts`/fixtures, a `seed.spec.ts` that lands the app in the
   start state, no copy-pasted boilerplate across specs.
4. **Parallelization** — match the app's constraints. This repo's demo is one shared in-memory session
   (`workers: 1`, `fullyParallel: false`) on purpose; document *why* so no one "optimizes" it into flakes.
5. **CI** — the browser suite runs in `.github/workflows/e2e.yml` and via `make e2e`; artifacts (trace,
   report) uploaded on failure. See [references/ci-integration.md](references/ci-integration.md).
6. **Agent toolchain** — `npx playwright init-agents --loop=claude` present and regenerated after PW
   upgrades, so planner/generator/healer have current tools. See [references/agents-setup.md](references/agents-setup.md).

## Output
A short framework-health report: each item ✓ / ✗ with the fix. Apply the config/fixture/CI fixes here;
hand any individual test changes to `authoring-tests`.

## Boundaries
This skill owns the **harness and standards**; `authoring-tests` owns **the tests**; `exploring-quality`
drives the browser **ad hoc**. If you're editing a `.spec.ts` assertion, you're probably in the wrong skill.

## Why "no hard waits" is the hill to die on
`waitForTimeout` trades a real signal (the element/state is ready) for a guess (250ms is probably enough).
It's slower on fast machines and flaky on slow ones — the worst of both. Web-first assertions wait for the
actual condition, so they're faster *and* more reliable. Enforcing this one rule removes most e2e flake.
