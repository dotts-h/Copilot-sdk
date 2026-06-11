# 0042. Per-sub-agent cost attribution (ledger v4 + SubagentShares, the S1-meter exclusion) and the budget leash enforced at the pre-dispatch gate

- Status: accepted
- Date: 2026-06-11
- Deciders: Horia
- Related: epic [0069](../issues/0069-epic-first-class-subagents.md) (first-class
  sub-agents), issue [0072](../issues/0072-per-subagent-cost-budget-leash.md) (S3, this
  ADR's child), [ADR-0040](0040-subagent-identity-instance-agentid-vs-spawn-toolcallid.md)
  (the instance↔spawn identity model), [ADR-0041](0041-subagent-registry-roster-semantics-status-vocabulary-unverified-done.md)
  (the registry + `AddCredits` seam this prices), [ADR-0018](0018-additive-attribution-tags-on-spend-records.md)
  (additive ledger tags, the discipline the v4 bump follows),
  [ADR-0008](0008-budget-guardrails-soft-warn-and-hard-cap-gate.md) (the hard-cap
  pause-resolve shape the leash reuses), [ADR-0033](0033-authoritative-cost-first-aiu-over-estimate.md),
  [SUBAGENTS_RESEARCH.md](../SUBAGENTS_RESEARCH.md) §4, [CONTRACTS §3/§4](../CONTRACTS.md)

## Context

S1 made every event attributable and S2 built the live roster with a parked
`AddCredits` seam (rendering `0.00 cr`). S3 makes a sub-agent's spend **live**,
**accountable**, and **leashed**. Three decisions had to be fixed:

1. Where a sub-agent's metered turn lands. The S1 invariant (ADR-0040, pinned by a
   test) is that tagged usage must **never** be metered as the root agent's spend —
   yet the spend is real money against the account and must show somewhere.
2. How to attribute it in the persisted ledger and roll it up, without the per-agent
   breakdown (`AgentShares`) double-counting it.
3. How to enforce a per-agent budget leash given sub-agents run **inside** the SDK:
   the orchestrator observes their tagged usage but does **not** drive each
   sub-agent's per-turn `Send`, so the literal "pre-`Send` gate per sub-agent turn"
   has no clean hook. S4 (issue 0073) owns pause/continue/cancel; the leash must not
   build that machinery.

## Decision

**1. A sub-agent turn is priced and ledgered, but kept out of the root/session token
meters.** `recordUsage` branches on the tag: a sub-agent-tagged turn is priced with
`telemetry.Price` (no meter mutation) and appended to the ledger, but is **not**
folded into `s.meter`/`s.sessionMeter` — those are the root's "this session" gauges
and the S1 pin test stays green verbatim. The priced credits flow to the registry
row via the existing `AddCredits` (so the live list shows running credits), and the
ledger record means the spend still counts toward the **account-wide** month-to-date
budget (it is real spend) — the S1 invariant is about *attribution to the root*, not
about hiding account spend.

**2. Additive ledger v4 + `SubagentShares`, with `AgentShares` excluding sub-agent
turns.** `SpendRecord` gains `SubagentID`/`SubagentName` (`sub`/`subname`, `omitempty`)
— a v3→v4 bump that is additive exactly like the v2 tags: a v3 ledger reads back with
empty sub-agent tags, older readers ignore the keys (ADR-0018). The name is carried on
the record so the Telemetry breakdown labels a row even after a restart drops the live
registry. `SubagentShares` rolls spend up per instance (excluding non-sub-agent turns,
like `WorkflowShares`); `AgentShares` now **excludes** sub-agent-tagged records so a
sub-agent's spend neither inflates the root (empty-agent) bucket nor any persona's —
no double-count.

**3. The budget leash is a pure `telemetry.Leash` enforced at the orchestrator's
pre-dispatch points, reusing the hard-cap gate.** `Leash{MaxCredits, MaxTurns}` is a
dependency-free domain type (`Active`, `Breached`) configured on a forge agent
persona (`Agent.MaxCredits/MaxTurns`, opt-in, validated non-negative). The web layer
accumulates each persona's credits/turns this session and, **before dispatching** that
persona's next turn (the root chat `Send` and the queue drain), parks the existing
`budgetGate` — tagged `leashAgent` — with proceed · raise · cancel. "raise" lifts only
**this** persona's leash for the session (a transient override, not a forge edit, and
the account cap is untouched). `/clear` resets the accounting. This is faithful to
"checked at the existing pre-`Send` gate / reuse the gate, not S4's pause records".

## Consequences

- **The roster goes live; S1 holds.** Sub-agent credits update over SSE on the same
  `subagents` fragment; the root meter/transcript stay untouched (pin test verbatim).
- **The leash is enforced where the orchestrator drives the `Send`.** That is the root
  persona turn today. A sub-agent running *inside* the SDK is **metered** now but its
  mid-run interruption needs a pause/continue/cancel point the orchestrator doesn't
  yet own — that is S4 (issue 0073), which the same `Leash` will reuse. The boundary
  is deliberate (the alternative — abort+resume now — is S4's machinery pulled
  forward). The per-lane/per-sub-agent accumulation already keyed by agent makes the
  S4 wiring a check at the new dispatch point, not a re-model.
- **One gate, two ceilings.** `budgetGate` + `/budget/{action}` now serve both the
  account hard cap and the per-agent leash; `leashAgent` is what tells `handleBudget`
  which ceiling "raise" lifts. The leash form reuses the hard-cap form's look.
- **CSV/export width grew by two columns** (`subagent`,`subagentName`), appended at the
  end so prior column positions are unchanged (the ADR-0018 backward-compatible rule).
- **The word "leash" now has two senses** — the process sense (lower-autonomy flow) and
  this budget sense (`telemetry.Leash`); CONTEXT disambiguates them.
