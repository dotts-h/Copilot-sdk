---
id: 0072
title: "Per-subagent cost + budget leash — live metering, ledger attribution, pre-Send cap (S3)"
status: closed
severity: high
group: 0069
depends_on: [0070]
github: 113
links:
  adr: [0042]
  prs: []
  issues: [0069]
  regression:
---

## Summary

The cost-awareness half of the epic: make a sub-agent's spend **live** (list row
credits), **accountable** (ledger), and **leashed** (a cap that blocks before the spend,
not after).

- **Live meter.** `AgentID`-tagged `EvUsage` routes through the shared `recordUsage`
  into a per-subagent accumulation (the `sessionMeter` pattern, ADR-0011), exposed to
  the S2 registry so each row shows running credits. Estimate-first, reported-AIU when
  present — the existing ADR-0033 hierarchy, unchanged.
- **Ledger attribution.** `SpendRecord` gains an additive `SubagentID`/`SubagentName`
  tag (schema **v3**; v2 ledgers read back with empty tags — the exact ADR-0018
  discipline). `AgentShares`-style pure reader (`SubagentShares`) for the Telemetry
  breakdown; `SubagentCompleted.TotalTokens` is the end-of-run cross-check, not the
  metering source.
- **Budget leash.** Per-subagent `max credits` + `max turns`, checked at the existing
  pre-`Send` gate (ADR-0008's pause-resolve shape): a breach parks the sub-agent as a
  gate (proceed | raise | cancel), never a silent kill. Default off; configured on the
  agent persona (forge) or the workflow step. Enforce at **our** gate — the research's
  LiteLLM lesson is that downstream budget enforcement has bypass bugs; ours sits
  before the spend.
- **Why now:** multi-agent ≈ 15× chat tokens (Anthropic), and GitHub's June 2026
  AI-Credits switch makes sub-agent spend token-proportional — the runaway sub-agent is
  the dominant cost-risk mode.

## Acceptance

- [x] Tagged `EvUsage` accumulates per sub-agent and surfaces on the S2 list row live
      (mock-driven reducer test — `TestSubagentUsageMetersCreditsOntoRow`).
- [x] `SpendRecord` schema is additive: new tag round-trips; an older ledger loads with
      empty tags (upgrade table-test, the ADR-0018 precedent). **Reconciled:** the bump
      is **v3→v4** (`sub`/`subname`) — v3 was already taken by ADR-0034's cache-write/
      reasoning counts; a v3 file reads back with empty sub-agent tags.
- [x] `SubagentShares` pure reader: per-subagent rollup on Telemetry; root-agent spend
      unaffected (no double-count — `AgentShares` excludes a sub-agent-tagged turn).
- [x] Leash: an agent crossing max-credits/max-turns parks at the pre-dispatch gate;
      proceed/raise/cancel each table-tested; concurrent surfaces unaffected. **Scope
      (ADR-0042):** enforced at the orchestrator-driven `Send` (root persona turn +
      queue drain); a sub-agent running *inside* the SDK is metered now, its mid-run
      interruption rides S4 (0073) — the same `telemetry.Leash` reused at that point.
- [x] Meter concurrency: **n/a as written** — the S1-preserving design prices a
      sub-agent turn with the pure `telemetry.Price` (no meter mutation) and the
      registry accumulation is serialized under the session mutex, so no new concurrent
      *meter* writer is introduced; the existing 16×100 `TestMeterConcurrentSafe` still
      covers the meter.
- [x] Gates green: `make lint && make test` (floor 65%).

## Out of scope

The pause-record machinery (S4 — the leash reuses the existing budget-gate shape, not
the new pause records), forecast bucketing per sub-agent (later, ADR-0019 cousin).

## Close-out (2026-06-11)

Shipped on this branch (ADR-0042). **Cost half:** sub-agent `EvUsage` is priced
(`recordUsage` branch) and ledgered with `sub`/`subname` (schema v4, additive), feeding
the live registry row's credits and a new "Cost by sub-agent" Telemetry breakdown
(`SubagentShares`); `AgentShares` excludes sub-agent turns so nothing is double-counted;
the S1 invariant (no sub-agent spend in the root/session token meters) is preserved
verbatim. **Leash half:** pure `telemetry.Leash{MaxCredits,MaxTurns}` configured on the
forge `Agent` (UI fields + validation), enforced at the pre-dispatch gate by reusing
`budgetGate`/`/budget/{action}` (proceed · raise-leash · cancel), reset on `/clear`.
Mid-run interruption of SDK-internal sub-agents is deferred to S4 (0073) by design.

## Notes

Research: [SUBAGENTS_RESEARCH.md](../SUBAGENTS_RESEARCH.md) §4 — OTel GenAI
standardizes per-agent tokens not cost; Claude Code's `query_source`/`agent_id` schema
is the tag model to mirror; `copilot-cli-cost` is the statusline prior art. Builds on
ADR-0011/0016/0018/0033.
