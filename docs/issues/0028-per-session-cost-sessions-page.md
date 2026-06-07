---
id: 0028
title: Per-session cost on the Sessions page (roadmap v4, item G2/V5)
status: closed
severity: medium
group: 0024
github:
links:
  adr:
  prs: [53]
  issues: [0024, 0027]
  regression:
assets: []
---

## Summary

The Sessions picker is the one cost surface that is still **cost-blind**. Every
`telemetry.SpendRecord` already carries a `SessionID` (schema v2, ADR-0018), but
`internal/web/sessions.go` (`sessionRows`/`sessionsPartial`) shows only a title + relative
age per session — no signal of how much a past conversation cost or how many turns it ran.
G2 makes the picker cost-aware: a pure `telemetry.SessionShares(records)` reader (a cousin
of `AgentShares`/`WorkflowShares`) rolls spend up per session id, and the web layer joins
it onto each `copilot.SessionMeta` row by id so the picker shows *"N turns · X cr"* beside
each session. No schema change — `SessionID` was already tagged. Source:
`docs/NEXT_FEATURES.md` item G2/V5.

## Repro
1. Open the Sessions page with a persisted spend ledger.
   - **Expected:** each listed session shows its total credits + turn count (*"N turns ·
     X cr"*); a session with no recorded spend shows *"no cost yet"* (still listed); a
     spend bucket for a since-deleted session is not shown.
   - **Actual (before G2):** the Sessions page shows only title + relative age — no
     per-session cost at all, even though every spend record is tagged with its session id.

## Proposed resolution

- **`internal/telemetry` (pure):** `SessionShares(records) []SessionShare` mirroring
  `AgentShares`/`WorkflowShares` via the shared `shareBy` helper; `SessionShare{SessionID,
  Credits, Turns}`; sorted by credits descending then session id ascending (deterministic).
  Settle + assert the empty-`SessionID` rule — **exclude** it (`includeEmpty=false`, like
  `WorkflowShares`): a session row needs a real id to join against, and the picker only
  lists real sessions. The turn count rides a per-group `Count` added to `shareBy` (one
  pass, mirroring `DailyTotals`). Unit-tested.
- **`internal/web` (Sessions view):** `sessionRows` joins
  `SessionShares(s.spend.Records())` onto each `copilot.SessionMeta` row by session id
  (read off the spend store's own leaf mutex — neither `s.mu` nor `forgeMu`, so no
  lock-order inversion); a listed session with no spend keeps its row (*"no cost yet"*); a
  spend bucket matching no listed session is not shown; with no spend store wired the rows
  render their prior shape (no cost cell). The `sessionsPage` template gains a cost cell.
  Credits via `telemetry.FormatCredits`; all values through `html/template` (ADR-0001).
- **No new store, no schema change** — `SessionID` is already tagged (ADR-0018).
- **Tests:** unit (`internal/telemetry` — multi-record sessions roll up to per-session
  totals (turns + credits), deterministic sort, the empty-`SessionID` bucket is excluded,
  empty ledger → empty). web — the page renders a cost cell per session joined by id, a
  session with no spend shows the zero/no-cost state, a spend bucket with no matching
  session is not shown, and it renders cleanly with no spend store wired (no panic, prior
  shape). e2e — assert the per-session cost cell **structure** on a session row, never
  exact figures (the shared demo ledger is append-only across the suite).

## Resolution (shipped)

Built as specified, no schema change (the `SessionID` tag already existed, ADR-0018).
`internal/telemetry` (`history.go`): `SessionShares(records) []SessionShare{SessionID,
Credits, Turns}` rolls spend up per copilot session id via the shared `shareBy` helper,
sorted by credits descending then session id ascending; it **excludes** the
empty-`SessionID` bucket (`includeEmpty=false`, like `WorkflowShares`). `shareBy` gained a
per-group `Count` accumulator (one pass, mirroring `DailyTotals`) so the turn count needs
no second scan and no duplicated empty-key guard. `internal/web` (`sessions.go`,
`templates/fragments.html`, `static/app.css`): `sessionRows` joins
`SessionShares(s.spend.Records())` onto each `copilot.SessionMeta` row by id off the spend
store's own leaf mutex (no `s.mu`/`forgeMu`, so no `forgeMu → s.mu` inversion); the
`sessionsPage` template gained a `.session-cost` cell showing *"N turns · X cr"* (a
no-spend session shows *"no cost yet"*, never dropped; a since-deleted bucket is not shown;
no spend store → prior shape). Credits via `telemetry.FormatCredits`; all through
`html/template` (ADR-0001). Tests: unit (`internal/telemetry`
`TestSessionShares`, `TestSessionSharesExcludesEmptySessionID`,
`TestSessionSharesDeterministicTieBreak`, `TestSessionSharesEmpty`), web
(`TestSessionsPageShowsPerSessionCost`, `TestSessionsPageNoSpendStoreNoPanic`), e2e (the
sessions spec asserts the cost-cell structure, never figures). Docs: CONTRACTS §3 (the
Sessions-page per-session-cost surface) + §4 (the `SessionShares` reader + empty-key rule).
No REGRESSIONS entry — no bug was found-and-fixed; the empty-key rule, the no-spend-session
join, and the nil-spend path were guarded preemptively (self-review with `/code-review`
high effort confirmed them). Shipped on branch `claude/per-session-cost` (**PR #53**,
merged).

## Notes
- **No ADR:** a pure-reader composition over the existing ledger, pre-blessed by ADR-0018
  (attribution) ⋈ the `*Shares` pattern — like V4/0025 and F3/0026. Captured in CONTRACTS
  §3 (the Sessions-page per-session-cost surface) + §4 (the `SessionShares` reader + the
  empty-key rule). No REGRESSIONS entry — no bug was found-and-fixed (the empty-key rule
  and the no-spend-session join were guarded preemptively by unit tests).
- **Differentiator:** completes the cost surface — the Sessions picker becomes a
  cost-aware dashboard, like the Workflows page (V4).
- **Numbering:** issue **0028** (next free after 0027), fourth build of epic **0024**
  (roadmap v4). No ADR consumed.
</content>
</invoke>
