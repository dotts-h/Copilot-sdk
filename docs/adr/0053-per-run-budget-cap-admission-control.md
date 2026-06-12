# 0053. Per-run budget cap — pre-Send admission control at run grain

- Status: accepted
- Date: 2026-06-12
- Deciders: Horia
- Related: issue 0096 (epic 0095), [ADR-0008](0008-budget-guardrails-soft-warn-hard-cap.md), [ADR-0024](0024-abort-in-flight-run.md), [ADR-0042](0042-per-subagent-cost-budget-leash.md), `internal/telemetry/credits.go`, `internal/web/server.go`, `internal/web/run_adapter.go`, `internal/web/session.go`

## Context

The meter **sees** run spend but cannot **stop** it. The repo enforces pre-Send
*admission control* at two grains already:

- **Account-wide hard cap** — `config.Telemetry.HardCapCredits` → `s.hardCap`, gated in
  `handleSend` / the queue drain via a `budgetGate` that pauses the turn (proceed / raise /
  cancel) — ADR-0008.
- **Per-persona budget leash** — `telemetry.Leash{MaxCredits, MaxTurns}` accumulated per
  agent in `s.agentCredits` / `s.agentTurns`, checked by `leashFor` → `pendingLeashGate`,
  same `budgetGate` pause shape — ADR-0042 (issue 0072).

The **missing grain is the run**. A workflow run fans out across lanes (and their
sub-agents) through `launchLanes` → `startLane`; each lane's usage folds into the run via
`recordUsage`, summing to `RunRecord.Credits`. But nothing bounds the *cumulative* credits a
single run may spend: a looping or wide run, and especially an **unattended / queued** run
(the v17 durable-autopilot direction), can burn without limit. The roadmap-v16 deep-research
pass found this is the converged failure mode — monthly/account caps structurally cannot stop
a run-grain runaway (the verified $47K/11-day incident had logging and monitoring but "no
hard limit, no per-conversation budget"), and the settled fix is **admission control**: a
gate that runs *before* the next model call, reads the run's running cost, and refuses to
proceed once the cap would be crossed (Portkey HTTP 412, LiteLLM `max_budget`, OpenRouter
key limits — all pre-call, key/run-scoped). This ADR records the contract for the run-grain
gate. It is the **enforcement** dual of ADR-0048's run *event log* (which only records).

## Decision

A run carries an optional **credit ceiling** (`telemetry.Leash`, reused — not a new type),
sourced from config (`config.Telemetry.RunCapCredits`; `0` = unleashed, the default). The
server accumulates a run's spend (`s.runCredits`, fed by the same `recordUsage` turn that
updates `RunRecord.Credits`) and checks it **before admitting the next lane's Send** in
`startLane`, the run's per-lane dispatch point.

### Admission, not interruption

The gate is **pre-Send admission control**, never a mid-flight kill of an in-flight call: a
lane already streaming runs to its natural end (its tokens are already committed), and the
abort path for a running call stays ADR-0024's explicit user action. The run cap decides
only whether the **next** lane is *admitted*: when the run's cumulative credits have reached
the cap, `startLane` does not open/Send that lane — it marks the run **budget-paused** and
stops admitting further lanes. This keeps the cap a deterministic, side-effect-free decision
(`leash.Breached(runCredits, runTurns)`) layered over the existing dispatch, with **no change
to the streaming or record path** when no cap is set.

### Interactive pause reuses `budgetGate`; unattended runs hard-stop

A run cap breach raises the **same `budgetGate`** the account cap and persona leash use
(proceed / raise / cancel), so the interactive UX and the lift-this-session semantics are
identical — `raise` lifts the run cap for *this run* only (a transient override, like the
persona leash's `leashLifted`), never editing the workflow or the account cap. For an
**unattended** run (no human to answer the gate — the v17 queue/schedule path) the same
breach resolves deterministically to **cancel** (the safe default that already governs an
unanswered gate): the run stops with the remaining lanes failed-with-reason and the partial
run recorded once, exactly the ADR-0024 abort shape. The default is always the cheap one — an
ambiguous or unanswered cap never spends past it.

### Estimate basis

The check is **cumulative-spend-vs-cap on the realized ledger** (`s.runCredits`, the same
authoritative figure that sums to `RunRecord.Credits`), evaluated *between* lanes — not a
pre-estimate of the next call's cost. Per-call superlinear growth (history resent each step)
means a realized cumulative check between admissions is both simpler and sufficient at run
grain; a forward per-call estimate is deferred (it belongs with the per-step pricing of
ADR-0048/issue 0092, and would only tighten the bound by one lane's worth). Pricing the cap
off the realized ledger means a price-book change never silently re-arms or relaxes a cap
mid-run.

### Reconciliation with the existing grains

The three caps **compose without inversion**: a turn is admitted only if it clears the
account hard cap *and* its persona leash *and* its run cap. They read independent counters
(`monthToDate` / `s.agentCredits` / `s.runCredits`) under `s.mu`, with no new lock and no
forge access on the hot path, so the run cap cannot invert the `forgeMu → s.mu` order. A
non-workflow (root-chat) turn has no run and is unaffected.

## Consequences

- A runaway run is **bounded**, not just observable — the v6 "store one renderer away" finding
  inverts to "the meter one gate away from acting." This is the safety precondition for v17
  (durable / unattended / queued runs): a run that nobody is watching can no longer burn past
  its ceiling.
- **Zero new type, zero new infra:** the cap reuses `telemetry.Leash` and the `budgetGate`
  pause; the only new state is a per-run credit counter parallel to the persona one. The
  streaming/record path is byte-identical when `RunCapCredits` is `0` (the default).
- The cap is **per-run-instance**, sourced from one global config knob in this slice; a
  per-workflow override (a field on the workflow forge entity) is a clean additive follow-up,
  noted but out of scope for P1.
- Sets up P2 (anomaly signal — a reader over the same `s.runCredits` / step credits) and P3
  (spend digest — rolls up cap-hits + anomalies), the rest of epic 0095.
