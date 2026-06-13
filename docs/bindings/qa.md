# Binding: qa

> An adapter doc, not a recipe. Copy it to `docs/bindings/qa.md` in the target
> repo and fill the backfill slots. It binds the open slots Layer 0/1 leave for
> browser/e2e testing — Playwright governance distilled to the rules that decay
> fastest when unwritten.

## Locator strategy

- **Role-based locators first** (`getByRole`, with accessible name), then
  test-ids (`data-testid`) for things without a meaningful role. CSS/XPath
  selectors tied to DOM structure are a review flag — they break on refactors
  that change nothing user-visible.
- Test-ids are a contract between the app and the suite: added deliberately,
  named for the *thing* not the test, and never removed while a test uses them.

## Assertions & waiting

- **Web-first assertions only** (`expect(locator).toBeVisible()` etc.) — they
  retry until the condition holds or times out.
- **No hard waits.** A literal sleep/timeout in a test is a bug: it's either too
  short (flake) or too long (waste). Wait *for the condition*, not for time.
- One behavioral claim per test name; a test that asserts five screens is five
  tests wearing a trench coat.

## Suite governance

- Tests are **independent and parallel-safe**: no ordering dependencies, no
  shared mutable accounts/fixtures; parallel workers are the default.
- **Flake protocol:** a flaky test is quarantined (skipped with an issue id from
  the issues recipe) the day it flakes twice — never retried-into-green
  permanently, never deleted silently. The issue tracks the root cause.
- Traces/screenshots are captured **on failure** in CI (retain-on-failure), so a
  red run is diagnosable without a re-run.
- The e2e suite runs in CI on the same single-run trigger rule as everything
  else (quality recipe); it is a required check, not an advisory one.

## What e2e is for (and not for)

- E2E covers **user-visible flows across the seams** — the paths unit and
  contract tests can't see. Logic permutations belong in unit tests; pushing
  them into e2e buys minutes of CI for coverage already owned elsewhere.
- A bug fixed at the e2e level still gets its guard test at the **lowest layer
  that can catch it** (REGRESSIONS rule, quality recipe).

## Backfill checklist (flagged by adopt-recipes)

- [ ] Point this doc at the suite's config (projects, retries, trace settings).
- [ ] Record the suite's CI job name next to the quality recipe's CI workflow.
- [ ] Sweep existing tests for hard waits and structural selectors; file issues.
