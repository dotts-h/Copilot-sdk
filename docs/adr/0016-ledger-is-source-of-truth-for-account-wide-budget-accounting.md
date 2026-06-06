# 0016. The persisted ledger is the source of truth for account-wide budget accounting

- Status: proposed
- Date: 2026-06-06
- Deciders: Horia
- Related: `internal/telemetry` (`SpendStore`, `SpendRecord`, `DailyTotals`,
  `ModelShares` — a new `MonthToDate` joins them), `internal/web`
  (`pages.go` `telemetryPartial`, `render.go` `renderCostFooter`, `server.go`
  `budget()` / hard-cap `overCap` projection, `session.go` `EvUsage`),
  `docs/NEXT_FEATURES.md` item A1, `docs/TECH_DEBT.md` #9,
  [ADR-0009](0009-persisted-spend-history-append-only-ledger.md),
  [ADR-0008](0008-budget-guardrails-soft-warn-and-hard-cap-gate.md),
  [ADR-0011](0011-per-session-telemetry-meter-for-the-statusline.md),
  issue [0014](../issues/0014-ledger-derived-budget-rows.md)

> **Proposed, not built.** This record is the architectural pick from the
> 2026-06-06 next-features research pass that grounds the build-first item
> (A1 / issue 0014). It is written first (ADR-0004 lead-with-a-decision) so the
> next build session inherits the decision, not a blank page. Promote to
> `accepted` when 0014 ships.

## Context

The cost differentiator is *active* end to end — pre-flight estimate (ADR-0007),
soft-warn + hard-cap guardrails (ADR-0008), an append-only persisted ledger with
trends (ADR-0009), and a per-session statusline meter (ADR-0011). But the
**account-wide accounting rows still read the live, in-process `telemetry.Meter`**:
`telemetryPartial` (`pages.go:183`) computes "Total cost / Monthly budget /
Remaining" from `s.meter.Totals()`, `renderCostFooter` reads the same meter, and
the hard-cap projection (`overCap`) measures "used" as this-process spend. The
`Meter` resets to zero on every restart, so "remaining this month" is
**restart-amnesiac** — directly undercutting the README headline ("a coding
session never surprises you on the bill"). ADR-0009 explicitly deferred this
("reconcile the budget / month-to-date rows against the ledger") and logged it as
TECH_DEBT #9.

Meanwhile the **persisted `SpendStore`** already records one `SpendRecord` per
turn (atomic temp-file+rename), each tagged with `At` and `SessionID`, survives
restart, and already backs the Telemetry **trend** view. The data the account-wide
rows need is already on disk; only the *read* still points at the wrong source.

## Considered options

- **Keep the live meter for the summary rows (status quo).** Rejected: amnesiac
  across restart, contradicts the headline promise, and leaves the trend view
  (ledger) and the budget rows (meter) silently disagreeing after a restart.
- **Replace the `Meter` entirely with the ledger.** Rejected: the in-process
  meter still earns its keep — the **per-session statusline** (ADR-0011), the
  live token split (cache-write / reasoning display counts), and the cache-hit
  rate are this-process / this-conversation signals the ledger doesn't carry. The
  meter isn't going away; only the *account-wide accounting source* shifts.
- **Reconcile a hybrid (meter ⊕ ledger).** Rejected: ADR-0009 already named the
  double-counting risk. Cleaner to assign **one source per surface**: the ledger
  owns account-wide accounting (Total/Monthly/Remaining, the cap baseline); the
  meter owns the live this-process / this-session signal. No reconciliation math,
  no drift.
- **Window definition: rolling 30 days vs UTC calendar month.** Choose the **UTC
  calendar month** — it matches the "monthly allowance" / billing-cycle mental
  model the allowance knob already implies. Make the window a **pure function**
  (`MonthToDate(records, now)`) so a configurable billing-anchor day is a later,
  additive tweak, not a rewrite.

## Decision

Add a pure `telemetry.MonthToDate(records []SpendRecord, now time.Time) Cost`
(credits/USD) beside `DailyTotals` / `ModelShares` — UTC calendar-month bucketing,
same totality guarantees as the rest of the package, no IO. The account-wide
surfaces read **month-to-date from the ledger**, not the live meter:

- `telemetryPartial`'s "Total cost / Monthly budget / Remaining" rows,
- `renderCostFooter`'s account-wide credits,
- the hard-cap projection's "used" baseline (`total + EstimateTurn` measured
  against persisted month-to-date, so a mid-month restart no longer silently
  re-opens a cap the user already approached).

The per-session statusline meter (ADR-0011) and the live token split stay on the
in-process meter, unchanged. Because every `SpendRecord` carries `SessionID`,
per-session (and, once A2 tags it, per-agent / per-workflow) month-to-date
breakdowns fall out of the same query for free.

This **supersedes the "history vs. live meter" split in ADR-0009** for the
account-wide rows: the trend section already used the ledger; now the summary rows
and the cap baseline join it. The on-disk schema is unchanged — `MonthToDate` is a
new pure reader over the existing v1 `records` array (CONTRACTS §4), so there is
**no migration**.

## Consequences

- Positive: "remaining this month" and the hard-cap baseline **survive restart**,
  closing the last amnesiac gap in the headline promise (TECH_DEBT #9 paid). One
  source of truth per surface — no meter/ledger drift. Per-session and (later)
  per-agent rollups are the same query with a filter, seeding item A2.
- Trade-off we accept: the ledger read is now on the **Telemetry render path** and
  the **pre-`Send` cap check** — both O(n) over the current month's records.
  Bounded by the same low single-user, one-record-per-turn volume as ADR-0009's
  append; if volume ever grows, cache the month total and invalidate on append
  (noted, not built).
- Trade-off: a UTC calendar month won't match a user whose plan resets mid-month.
  Acceptable now (the allowance is already a flat monthly number); the pure-
  function window makes a billing-anchor day an additive follow-up.
- Follow-ups: A2 (attribute records to agent/workflow → per-agent month-to-date),
  A3 (a burn-rate forecast over `DailyTotals` + the allowance). Both build on the
  ledger queries this ADR establishes.
