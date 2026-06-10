---
id: 0072
title: "Per-subagent cost + budget leash — live metering, ledger attribution, pre-Send cap (S3)"
status: open
severity: high
group: 0069
depends_on: [0070]
github: 113
links:
  adr: []
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

- [ ] Tagged `EvUsage` accumulates per sub-agent and surfaces on the S2 list row live
      (mock-driven reducer test).
- [ ] `SpendRecord` schema v3 is additive: new tag round-trips; a v2 ledger loads with
      empty tags (upgrade table-test, the ADR-0018 precedent).
- [ ] `SubagentShares` pure reader: per-subagent rollup on Telemetry; root-agent spend
      unaffected (no double-count — a tagged turn is excluded from the root bucket).
- [ ] Leash: a sub-agent crossing max-credits/max-turns parks at the gate before
      `Send`; proceed/raise/cancel each table-tested; concurrent lanes unaffected.
- [ ] Meter concurrency test extended to tagged writers (the 16×100 pattern).
- [ ] Gates green: `make lint && make test` (floor 65%), `make e2e`.

## Out of scope

The pause-record machinery (S4 — the leash reuses the existing budget-gate shape, not
the new pause records), forecast bucketing per sub-agent (later, ADR-0019 cousin).

## Notes

Research: [SUBAGENTS_RESEARCH.md](../SUBAGENTS_RESEARCH.md) §4 — OTel GenAI
standardizes per-agent tokens not cost; Claude Code's `query_source`/`agent_id` schema
is the tag model to mirror; `copilot-cli-cost` is the statusline prior art. Builds on
ADR-0011/0016/0018/0033.
