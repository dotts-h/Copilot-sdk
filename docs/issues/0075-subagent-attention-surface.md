---
id: 0075
title: "Attention surface — needs-you badging, title/favicon dot, Runs integration (S6)"
status: open
severity: medium
group: 0069
depends_on: [0071, 0073]
github: 116
links:
  adr: []
  prs: []
  issues: [0069]
  regression:
---

## Summary

The closing child: make "a sub-agent needs you" impossible to miss once more than a
couple run — the out-of-band signals every shipped product added (Devin's orange
favicon dot, GitHub's "jump in when Copilot needs your input").

- **In-app badge.** The S2 list header shows a count of `input-required` sub-agents
  (amber chip, `--warn`); rows already carry the amber state from S2/S4 — this is the
  rollup. The sidebar Chat nav item gets the same badge so other pages surface it.
- **Out-of-band dot.** When any pause is pending: `<title>` prefix (`(1) …`) and an
  amber favicon dot; restored when the queue drains. Client-side, a few lines in the
  existing head script (the theme-toggle precedent) driven by an SSE-swapped marker —
  no new route, degrades silently.
- **Runs integration.** `input-required` renders distinctly on the lanes panel and the
  Runs page (like `skipped`, ADR-0021/0022); a completed run records pause count +
  total paused duration per lane (additive `RunRecord` fields, v(n)→v(n+1) read-back) —
  the orchestration history shows *where humans were the bottleneck*.

## Acceptance

- [ ] Badge count on the sub-agent list header and Chat nav reflects pending
      `input-required` pauses live (mock-driven; e2e on the demo escalate).
- [ ] Title/favicon dot appears while a pause is pending and clears on resolution
      (Playwright asserts `document.title`; reduced to title-only where favicon swap
      is flaky).
- [ ] Lanes panel + Runs page render `input-required` distinctly; aborting a paused
      run resolves its pauses exactly once (S4's idempotency, asserted here too).
- [ ] `RunRecord` gains pause count/duration additively; old run stores read back
      (upgrade table-test, the ADR-0022 discipline).
- [ ] a11y: badge has an accessible name; not color-only (axe green both themes).
- [ ] Gates green: `make lint && make test` (floor 65%), `make e2e` — closes the epic.

## Out of scope

OS-level notifications (desktop shell), email/webhook escalation on SLA expiry (a
future hook-command consumer — ADR-0032's executor is the natural seam).

## Notes

Research: [SUBAGENTS_RESEARCH.md](../SUBAGENTS_RESEARCH.md) §5 — Devin favicon dot
(orange = waiting on you), mission-control task view, HatchWorks "unclear state" as the
top UX failure. Builds on the S2 list, S4 pause records, ADR-0022 run store, ADR-0024
abort.
