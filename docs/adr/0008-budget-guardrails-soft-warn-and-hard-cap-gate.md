# 0008. Budget guardrails: a soft warn and a hard-cap turn gate

- Status: accepted
- Date: 2026-06-05
- Deciders: Horia
- Related: `internal/telemetry` (`Budget`), `internal/web` (`handleSend`,
  `handleBudget`, `renderStatline`, `renderCostFooter`), `config.TelemetryConfig`,
  `docs/NEXT_FEATURES.md` item 1.1, [ADR-0007](0007-pre-flight-turn-cost-estimate-prices-context-as-fresh-input.md)

## Context

The meter observes spend and (since ADR-0007) projects the next turn, but nothing
**intervenes**. The README promises "a coding session never surprises you on the
bill," yet `telemetry.Budget` was read-only (`Remaining`/`FractionUsed`).
`NEXT_FEATURES.md` item 1.1 asks for two guardrails: a **soft warn** at a
threshold of the allowance, and an optional **hard cap** that pauses a turn whose
spend would breach it. The budget type, the live meter, the pre-flight estimate,
and the inline-approval UX primitive (the permission form) all already exist; this
wires them together.

Three questions had to be answered: *what to price for the cap*, *how to gate a
turn*, and *how the gate decision should surface*.

## Considered options

- **What to price for the cap.** Reuse `EstimateTurn` (ADR-0007): the projected
  spend is `running total + EstimateTurn(model, liveContext)`. It is the one
  knowable, deterministic figure before a turn runs, and reusing it keeps the cap
  consistent with the "next turn ~N cr" the user already sees. (Rejected:
  modelling cache hits / output length — dresses a guess as precision, and ADR-0007
  already rejected it for the estimate.)
- **How to gate.** Pause **before `Send`** in `handleSend`: if the projection
  breaches the cap, record a `budgetGate` (the held prompt + attachments) and
  render an inline form instead of dispatching. The user bubble is still added, so
  the type-ahead UX is preserved. (Rejected: gating reactively on `EvUsage` after
  the turn — too late; the spend is already incurred.)
- **Route for the decision.** A **dedicated `POST /budget/{action}`** route
  (`proceed` | `raise` | `cancel`), *reusing the permission form's look* but not
  its `permBridge`/`/perm/{id}` plumbing. The gate is an **app-level** pause, not
  an SDK tool permission; routing it through `Respond(id, approve)` would answer a
  permission the runtime never asked for. (Rejected: literally reusing `/perm/{id}`
  — conflates two different decisions and lies to the SDK.)

## Decision

`telemetry.Budget` gains `WarnFraction` and `HardCapCredits` and two pure, total
predicates: `Warned(used)` (soft threshold crossed; disabled when allowance or
fraction is zero) and `CapExceeded(projected)` (strict `>`, disabled when the cap
is zero). `config.TelemetryConfig.HardCapCredits` persists the cap (atomic write,
validated `>= 0`); `WarnFraction` already existed.

The web layer:
- **Soft warn** is ambient: the topbar cost footer and the statusline cost item
  turn amber (`Budget.Warned`) — the topbar *is* the banner, no new surface.
- **Hard cap** gates in `handleSend`: `overCap()` projects `total + EstimateTurn`
  and, if it breaches the cap, holds the turn in a `budgetGate` and renders the
  inline form. `handleBudget` resolves it — **proceed** dispatches and keeps the
  cap; **raise** lifts (disables) the cap, persists it, and dispatches; **cancel**
  drops the turn. A settings save refreshes the live session's cached budget
  (`refreshBudget`) so the gate takes effect immediately, not only next session.

No new normalized event or persisted schema beyond the one config field; the gate
is a synchronous POST round-trip, with a `budget` SSE listener for parity.

## Consequences

- Positive: cost is now **active** — the differentiator the README promises.
  Everything reuses existing primitives (the meter, `EstimateTurn`, the perm-form
  look, the atomic-config discipline); domain logic stays pure and unit-tested.
- Trade-off we accept: the cap is checked against the **deliberately approximate**
  projection from ADR-0007 (a conservative input ceiling that omits the cache
  discount and output), so it is a *guardrail*, not an accountant — framed as
  "would exceed" and always overridable (proceed / raise). The authoritative AIU
  on the Telemetry page remains ground truth.
- Trade-off: `refreshBudget` updates only the **editing** session's cached knobs;
  other concurrent cookie-keyed sessions pick the change up on their next session
  (logged in TECH_DEBT). Acceptable for the single-user localhost tool.
- Follow-ups: a tighter cap could fold the observed cache-hit ratio and a rolling
  output average once spend history is persisted (item 1.3).
