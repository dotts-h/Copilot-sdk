---
id: 0075
title: "Attention surface — needs-you badging, title/favicon dot, Runs integration (S6)"
status: closed
severity: medium
group: 0069
depends_on: [0071, 0073]
github: 116
links:
  adr: []
  prs: []
  issues: [0069]
  regression: "SSE-swapped marker reacts via an inline script in the fragment, not an htmx:afterSwap listener"
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

- [x] Badge count on the sub-agent list header and Chat nav reflects pending
      `input-required` live. The **list-header badge** counts the registry's parked
      sub-agents (the S2 roster rollup — `subagentHeader`, `TestSubagentListHeaderAttentionBadge`);
      the **Chat nav badge + out-of-band dot** count *pending pauses* (the inclusive
      "needs you" signal, so a lane escalation with no registry sub-agent still raises
      it — `attentionFrag`/`attentionMarker`, `TestAttentionMarkerCountsPendingPauses`).
- [x] Title/favicon dot appears while a pause is pending and clears on resolution
      (e2e asserts `document.title` `(1) my-orchestra` ⇄ `my-orchestra` on the demo
      escalate, plus the favicon-dot href swap — `e2e.spec.ts` "parks a lane as
      input-required"). Client-side, an inline script in the SSE-swapped marker (no
      new route).
- [x] Lanes panel + Runs page render `input-required`/pauses distinctly — the lanes
      panel already renders `lane-input-required` amber (S4); the Runs page now shows a
      per-lane `lane-paused` indicator with count + waited time
      (`TestRunsPartialRendersLanePauses`). Aborting a paused run resolves its pauses
      exactly once AND still attributes the lane's pause
      (`TestAbortedPausedLaneStillRecordsPause`).
- [x] `RunRecord`/`RunLane` gain `pauses`/`pausedMs` additively (schema **v2**); a v1
      file reads back with them zero (`TestRunStoreReadsBackPrePauseSchema`,
      `TestRunRecordPauseFields` — the ADR-0022 discipline).
- [x] a11y: the list-header badge and nav badge carry an accessible name
      (`aria-label="N sub-agent(s) need(s) your input"`, `role=status`) and convey
      state by text + the `--warn` token, never color alone; the both-theme axe suite
      (`a11y.spec.ts`) stays green.
- [x] Gates green: `make lint && make test` (floor 65%, web 90.1%), `make e2e` (152
      passing) — closes the epic.

## Out of scope

OS-level notifications (desktop shell), email/webhook escalation on SLA expiry (a
future hook-command consumer — ADR-0032's executor is the natural seam).

## Notes

Research: [SUBAGENTS_RESEARCH.md](../SUBAGENTS_RESEARCH.md) §5 — Devin favicon dot
(orange = waiting on you), mission-control task view, HatchWorks "unclear state" as the
top UX failure. Builds on the S2 list, S4 pause records, ADR-0022 run store, ADR-0024
abort.
