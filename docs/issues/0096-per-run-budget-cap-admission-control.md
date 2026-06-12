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
decision — but a multi-lane run is **not** a single held prompt, so it does **not** reuse the
root-chat `budgetGate`. A breach resolves the ADR-0024 way: `startLane` **does not admit** the
over-cap lane, the remaining un-admitted lanes are **failed-with-reason** ("over run budget
cap"), the partial run is recorded once, and a system note surfaces the cap-hit. Deterministic,
no new gate plumbing, identical whether the run is interactive or (v17) unattended. Lifting the
ceiling is a **config edit + rerun** (ADR-0023), not a transient in-run override.

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
- **Enforcement:** accumulate the run's credits (on `workflowRun`, fed by the same
  `handleRunEvent` EvUsage turn that updates `l.credits`); in `startLane`, before
  `CreateSession`/`Send`, if the run's cumulative credits have reached the cap, **do not admit**
  the lane — settle it over-cap and stop admitting further lanes (the ADR-0024 terminal path
  records the partial run once).
- **Surface:** a system note on the cap-hit ("⚠ run stopped — over budget cap"); the over-cap
  lanes failed-with-reason in the run view. No new page.

## Out of scope

- Forward per-call cost **estimate** gate (realized cumulative-vs-cap between lanes is
  sufficient at run grain — ADR-0053); per-workflow cap **override** (additive follow-up on
  the workflow forge entity); mid-flight **kill** of an already-streaming call (abort stays
  ADR-0024's explicit user action). Anomaly detection (0097) and the digest (0098) are
  siblings.

## Acceptance

- [ ] A run with `RunCapCredits > 0` whose cumulative credits reach the cap does **not** admit
      the next lane — it settles the lane over-cap and stops admitting, recording the partial run.
- [ ] A system note surfaces the cap-hit; the over-cap lane reads failed-with-reason.
- [ ] `RunCapCredits == 0` ⇒ no stop, streaming/record path byte-identical (a guarding test).
- [ ] The run cap composes with the account / persona caps without lock inversion.
- [ ] `make lint && make test` green (coverage ≥ floor); `make e2e` for the pause UI; ADR-0053
      + this close-out ride the branch (ADR-0004).
