---
id: 0022
title: "Epic: extensibility & convergence (roadmap v3)"
status: open
severity: high
group:
github:
links:
  adr:
  prs: []
  issues: [0019, 0023]
  regression:
assets: []
---

## Charter

Roadmap **v1** (cost active: guardrails + ledger + per-session meter) and **v2**
(epic 0013: cost *accountable across time* + *attributable per agent/workflow*, and
orchestration's *parallel* + *branching* lanes with *persisted run history*) are both
shipped and closed. The two differentiators — **cost-awareness** (the meter) and
**orchestration** (the name) — are now *deep*. The 2026-06-06 v3 research pass
(`docs/NEXT_FEATURES.md`) re-read the code as-of today and found the next frontier is
no longer *depth within* each differentiator but **reach and convergence**:

- **Extensibility is gated.** MCP is how a user grows the agent's tools, but the MCP
  page is **key-free**: there is no secrets/`Env` editor, so the highest-value servers
  (GitHub, web search) can't be configured from the UI (TECH_DEBT #10). The blocker was
  always *where secrets live* — now decided in ADR-0020 (env-var-reference indirection,
  no secret at rest).
- **The two stores never meet.** B3 persists workflow runs (`RunStore`) and A2 tags
  spend per workflow (`SpendStore`), but no view **joins** them — Runs is a flat log
  with no duration and no roll-up. The convergence of cost ⋈ orchestration is one pure
  reader away.

This epic carries the two **build-first** picks. The remaining v3 candidates (the
generic `AppendOnlyStore[T]` paydown, the Settings price-override editor, per-session /
per-workflow cost surfaces, the bucketed forecast, Tier D distribution) stay candidates
in `NEXT_FEATURES.md` until promoted.

## Tasks

- [x] **C1 — MCP secrets / Env editor** → [0019](0019-mcp-secrets-env-editor.md)
      (ADR-0020, promotes TECH_DEBT #10) — **shipped**. A masked `Env` editor on the MCP form whose
      secret rows persist only a `${VAR}` **reference** (resolved from the environment
      at session start via `web.MCPServerSpecs`, behind a lookup seam) — **no secret at
      rest**, following the `config.GitHubTokenEnv` precedent. Unblocks key-requiring
      curated servers. **Build first** — it opens the extensibility story and its key
      decision (ADR-0020) is already settled. Claims the **reserved** issue 0019 /
      ADR-0020.
- [ ] **V1 — Workflow run-history aggregations + Runs duration** →
      [0023](0023-workflow-run-aggregations.md) (no ADR — pure readers pre-blessed by
      ADR-0022). A `RunAggregates` roll-up (run count, avg cost, avg duration, failure
      rate per workflow) + a `RunRecord.Duration()` helper over the existing `RunStore`
      records (no schema change); a Runs duration column + per-workflow summary, joining
      the spend and run stores. The cost ⋈ orchestration convergence — small,
      compounding.

## Status

**Open.** Build-first sequence: **C1 → V1**. **C1 shipped** (the MCP Env editor /
secret references, ADR-0020 — extensibility gate opened). **V1 next** (pure-reader
convergence; lowest risk, builds on B3). Everything else stays a candidate in
`NEXT_FEATURES.md`.

## Notes

Per CONVENTIONS: write the failing test first; keep domain logic pure
(`telemetry`/`ctxforge`/`config` dependency-free); `make lint && make test` (floor
65%) + `make e2e` for UI; fold ADR/CONTRACTS/REGRESSIONS into the feature branch that
motivates them (ADR-0004) — so C1's CONTRACTS §4 `MCPServer.Env` update lands with its
build, not with this research pass.
