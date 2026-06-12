---
id: 0096
title: "Per-run budget cap — pre-Send admission control at run grain (P1)"
status: open
severity: high
group: 0095
depends_on: []
github:
links:
  adr: [0053]
  prs: []
  issues: [0095]
  regression:
assets: []
---

## Summary

Bound a workflow run's **cumulative** spend. A run fans out across lanes (`launchLanes` →
`startLane`) whose usage sums into `RunRecord.Credits`, but nothing caps the total — a
looping/wide or **unattended** run can burn without limit. Add a per-run credit ceiling
(`telemetry.Leash`, **reused**), sourced from `config.Telemetry.RunCapCredits` (`0` =
unleashed, the default), accumulated in `s.runCredits` by the same `recordUsage` turn that
feeds `RunRecord.Credits`, and **checked before admitting the next lane's Send** in
`startLane` — the run-grain dual of the per-persona leash (issue 0072, ADR-0042) and the
account hard cap (ADR-0008). Contract: **ADR-0053**.

This **mirrors the 0072 sub-agent leash at run grain** — the same pure `leash.Breached(...)`
decision and the same `budgetGate` pause (proceed / raise / cancel), so the interactive UX
and the lift-this-session semantics are identical. `raise` lifts the cap for **this run
only** (transient, like `leashLifted`), never editing the workflow or the account cap. An
**unattended** run (no human to answer the gate — the v17 queue/schedule path) resolves a
breach deterministically to **cancel**: remaining lanes failed-with-reason, the partial run
recorded once (the ADR-0024 abort shape). The safe default never spends past the cap.

## Why now

The roadmap-v16 research found per-run admission control the converged, lowest-risk,
highest-value next slice: monthly/account caps structurally can't stop run-grain runaways
(the verified $47K/11-day incident), and the fix sits directly on seams the repo already has
(the ledger, the leash type, the `budgetGate`). It is the **safety precondition for v17**
(durable/unattended runs) — a run nobody is watching must not burn past its ceiling first.

## Scope (P1)

- **Pure logic (test-first):** reuse `telemetry.Leash`; a run-grain accumulation + breach
  decision driven off realized cumulative credits (`leash.Breached(runCredits, runTurns)`).
  Pure, table-tested in `internal/telemetry` / `internal/web` with no I/O.
- **Config:** `config.Telemetry.RunCapCredits float64` (`0` = unleashed); default off, so the
  streaming/record path is **byte-identical** when unset.
- **Enforcement:** accumulate `s.runCredits` in `recordUsage` (the run-tagged turn); in
  `startLane`, before `client.Send`, if the run's cumulative credits have reached the cap,
  **do not admit** the lane — mark the run budget-paused and stop admitting further lanes;
  raise a `budgetGate` (interactive) or resolve to cancel (unattended).
- **Surface:** a system note on pause/cancel; the cap shown alongside the run; the gate reuses
  the existing budget pause partial. No new page.

## Out of scope

- Forward per-call cost **estimate** gate (realized cumulative-vs-cap between lanes is
  sufficient at run grain — ADR-0053); per-workflow cap **override** (additive follow-up on
  the workflow forge entity); mid-flight **kill** of an already-streaming call (abort stays
  ADR-0024's explicit user action). Anomaly detection (0097) and the digest (0098) are
  siblings.

## Acceptance

- [ ] A run with `RunCapCredits > 0` whose cumulative credits reach the cap does **not** admit
      the next lane — it pauses (interactive) or cancels (unattended) instead.
- [ ] `raise` lifts the cap for this run only; `cancel`/unanswered drops the run safely (no
      spend past the cap); `proceed` is the account-cap shape where applicable.
- [ ] `RunCapCredits == 0` ⇒ no gate, streaming/record path byte-identical (a guarding test).
- [ ] The three caps (account / persona / run) compose without lock inversion.
- [ ] `make lint && make test` green (coverage ≥ floor); `make e2e` for the pause UI; ADR-0053
      + this close-out ride the branch (ADR-0004).
