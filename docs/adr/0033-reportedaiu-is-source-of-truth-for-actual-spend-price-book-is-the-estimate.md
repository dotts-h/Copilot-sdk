# 0033. ReportedAIU is the source of truth for actual spend; the price book is the estimate/fallback

- Status: accepted
- Date: 2026-06-09
- Deciders: Horia
- Related: `internal/telemetry` (`Meter` — new `HasReported`/`ActualCredits`;
  `SpendRecord` — new `EstimateCredits`/`ActualCredits`/`HasReported`; new
  `spend_source.go`: `ActualCredits`/`HasReported`/`ReportedCredits`/`ActualSpend`/
  `MonthToDateActual`), `internal/web` (`session.go` `recordUsage` —
  reported AIU now folded into the session meter too; `budget.go`
  `MonthToDateActual`/`monthToDateActual`; `render.go` `renderActualCostFooter`
  + the statusline cost cell; `commands.go` `cmdCost`; `demo.go`), the
  `statline`/`costFooter` fragments, [ADR-0016](0016-ledger-is-source-of-truth-for-account-wide-budget-accounting.md),
  [ADR-0011](0011-per-session-telemetry-meter-for-the-statusline.md),
  [ADR-0009](0009-persisted-spend-history-append-only-ledger.md),
  REGRESSIONS #3 (display-only `ExtraTokens`),
  epic [0050](../issues/0050-epic-billing-fidelity.md),
  issue [0057](../issues/0057-authoritative-cost-first-metering.md)

> **Shipped (P0 / issue 0057).** Written first as the framing decision of the
> billing-fidelity epic (ADR-0004 lead-with-a-decision), then built. Guarded by
> `internal/telemetry` `TestActualCredits` (the table-tested selection),
> `TestMeterActualCreditsPrefersReported`, `TestSpendRecordActualVsEstimate`,
> `TestMonthToDateActual`/`TestMonthToDateActualAllReported`; and `internal/web`
> `TestStatlineLabelsReportedVsEstimatedSpend`, `TestCostFooterPrefersReportedSpend`.

## Context

A 2026-06-08 audit (epic 0050) found the meter has **drifted from GitHub Copilot's
usage-based billing**: the static price book under-counts because two billed token
types (cache-write ≈ 1.25× input, and reasoning at the output rate) sit in
display-only `ExtraTokens` (REGRESSIONS #3). The deeper problem is *framing*: the
price book is treated as the **source of truth for actual spend**, when GitHub's
runtime already reports each turn's authoritative cost as `ReportedAIU` (captured as
`UsageData.NanoAIU`, folded into the account meter and persisted on each
`SpendRecord.AIU`). Today that authoritative figure is recorded but never *preferred*
— every "what did this cost" read (statusline, cost footer, `/cost`) shows the
price-book estimate, so a model whose real rate the book has wrong silently
mis-reports the bill.

An AI unit (AIU) and an AI Credit are the **same unit**: GitHub's meter denominates
both in the credit whose USD value is `USDPerCredit` (1 cr = $0.01). So a reported
AIU *is* the actual credit spend, at parity — no conversion factor to choose.

## Considered options

- **Keep the price book as the source of truth for actual spend (status quo).**
  Rejected: it is an *estimate*, structurally blind to cache-write/reasoning and to
  any rate GitHub changed. It drifts from the bill — the exact failure the epic was
  filed on.
- **Replace the price book with reported cost entirely.** Rejected: the estimate
  still earns its keep where no reported figure exists — the **pre-flight composer**
  estimate (cost of the *next* turn, before it runs, so there is nothing reported
  yet), the **burn-rate forecast**, and the **offline fallback** (the mock and any
  unreported turn report no AIU). Removing it would blank those surfaces.
- **Reconcile a blend (estimate ⊕ reported).** Rejected as the *primary* read — it
  re-introduces the double-source ambiguity ADR-0016 closed. Instead assign a strict
  **hierarchy**: reported-where-present, estimate-elsewhere, each labelled. (The
  estimate-vs-reported *drift* row is its own later lane, issue 0060.)
- **A conversion factor between AIU and credits.** Rejected: they are the same unit;
  parity (1 AIU = 1 credit) is stated once in `ReportedCredits` so callers read
  intent, not a bare identity.
- **AIU-presence boundary: `!= 0` vs `> 0`.** Choose **strictly positive**
  (`HasReported`): a zero AIU means "the runtime never reported" (offline/unreported),
  and a negative value is never real spend — both fall back to the estimate.

## Decision

Re-frame the meter around a **three-tier source hierarchy** (per turn): the SDK's
`ReportedAIU` is the truth for *actual* spend when present; the static price book is
the **estimate** (pre-flight + forecast) and the **offline fallback**.

The selection is one pure seam, `telemetry.ActualCredits(estimateCredits,
reportedAIU)` — reported when `HasReported(reportedAIU)`, else the estimate — so
estimate and reported can never silently swap roles. It is folded:

- onto the **`Meter`** (`HasReported`, `ActualCredits`), and the reported AIU is now
  recorded into **both** the account meter and the per-session meter (so the
  statusline prefers reported on its own scope, ADR-0011);
- onto **`SpendRecord`** (`EstimateCredits` = the price-book figure under its explicit
  name; `ActualCredits` = reported-or-estimate; `HasReported`). `Credits()` is
  retained but documented as the *estimate* (the aggregation readers bucket
  estimate-priced USD);
- as a pure account-wide aggregate `MonthToDateActual(records, now) → ActualSpend`
  (actual / estimate / reported sub-totals + `AnyReported`/`AllReported` coverage
  flags), the authoritative-cost-first sibling of `MonthToDate` (ADR-0016) over the
  same UTC-month window.

**Surfacing (this issue's seam — statusline/footer/cost only).** The statusline cost
cell and the topbar cost footer (`renderActualCostFooter`) now show the **actual**
figure tagged with its source — `reported` (authoritative), `est` (price-book), or
`mixed` (some turns reported, some estimated) — and the statusline shows the estimate
beside the reported figure when the two diverge, so the drift is visible where spend
is shown. The per-model breakdown **table** is out of scope (issue 0058).

The on-disk schema is **unchanged**: `SpendRecord.AIU` already exists (additive,
omit-empty), so the new readers are pure functions over the existing records — **no
migration**; an older v1/v2 record with no AIU reads back as estimate-backed, exactly
the offline-fallback path.

## Consequences

- Positive: actual spend is reported **authoritative-cost-first** — the headline
  "never surprises you on the bill" now reflects GitHub's own number where it exists,
  not just our estimate. The estimate is explicitly demoted (pre-flight/forecast/
  offline) and is never mistaken for the bill (it's labelled). One selection seam,
  table-tested, so estimate and reported can't swap.
- Positive (enables): P1 (0059) prices cache-write/reasoning so the *estimate* tracks
  reported more tightly; P3 (0060) adds the estimate-vs-reported **drift** row on the
  Telemetry page, joining the same two figures this ADR names.
- Trade-off we accept: aggregation readers (`DailyTotals`, the `*Shares`, the
  trend/forecast) still bucket the **estimate-priced USD** — they are pre-existing
  estimate views and migrating them to actual is the explicit follow-up scope of the
  drift lane (0060), not this P0. The *account-wide cost surfaces* (footer, `/cost`)
  are authoritative-first now; the trend/share charts remain estimate-denominated
  until 0060.
- Trade-off: the price book staying deterministic and overridable is preserved (it is
  still the estimate engine); a turn that reports no AIU is indistinguishable from a
  fully-estimated turn, which is correct — both are estimates.
