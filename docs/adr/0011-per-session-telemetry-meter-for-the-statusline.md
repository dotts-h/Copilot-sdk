# 0011. Per-session telemetry meter for the statusline

- Status: accepted
- Date: 2026-06-05
- Deciders: Horia
- Related: `internal/telemetry` (`Meter`, `Meter.PriceBook`), `internal/web`
  (`Server.sessionMeter`, `newSession`, `session.go` EvUsage, `renderStatline`),
  `docs/NEXT_FEATURES.md` item 3.2, `docs/TECH_DEBT.md` #2 / #9,
  [ADR-0008](0008-budget-guardrails-soft-warn-and-hard-cap-gate.md),
  [ADR-0009](0009-persisted-spend-history-append-only-ledger.md)

## Context

The `telemetry.Meter` is process-global: `Hub.New` builds one and every
cookie-keyed `Server` shares the pointer. The chat **statusline**
(`renderStatline`) reads it for the running token split, cache-hit %, credits,
and the pre-flight estimate. So a statusline that says "this conversation" was
really showing **every** conversation's combined spend — fine for the common
single-conversation case, wrong the moment a second browser session is open, and
called out as TECH_DEBT #2. Item 3.2 asks to scope the statusline to *this*
session.

The persisted ledger (ADR-0009) already tags every `SpendRecord` with a
`SessionID`, so the records *could* be re-aggregated per session. Three
questions had to be answered: *how to derive the per-session totals*, *what stays
account-wide*, and *where the soft-warn lives*.

## Considered options

- **How to derive per-session totals.**
  - **A scoped sibling `Meter` (chosen).** Each `Server` constructs its own
    `telemetry.Meter` on the *same price book* as the account-wide meter
    (`telemetry.NewMeter(h.meter.PriceBook())`), and the `EvUsage` reducer records
    each turn into **both**. `renderStatline` reads the session meter. Reuses the
    `Meter` type wholesale — `TotalTokens`, `ExtraTokens` (cache-write/reasoning),
    `Totals`, `EstimateTurn` all already exist and stay pure and deterministic;
    O(1) per turn; penny-consistent with the global gauge because the rates are
    shared. Cost: one small accumulator per session (negligible) and a second
    `Record` call in the reducer.
  - **Ledger-derived (filter `SpendStore` by `SessionID`).** Rejected: the ledger
    persists only USD + the priced in/cached/out token counts, **not** the
    display-only cache-write or reasoning ("thinking") tokens the statusline
    shows, so it cannot reconstruct the full split. It is also ephemeral in
    demo/tests, shared across sessions (needs a per-render filter + re-aggregation,
    O(n)), and lags the live stream (the statusline must update on `EvUsage`, not
    after a disk flush). Wrong tool for a live gauge.
  - **Keep global, just relabel the statusline.** Rejected — fails the explicit
    ask; the number would still be account-wide.

- **What stays account-wide.** The **topbar cost footer** (the ambient
  over-budget banner of ADR-0008), the **hard-cap projection** (`overCap`), and
  the **Telemetry page** month-to-date / per-model rows keep reading the global
  `s.meter`. Budget *enforcement* and *accounting* must be cumulative across every
  session — a per-conversation cap or month-to-date would be wrong — and this is
  the natural seam to later reconcile against the ledger (TECH_DEBT #9).

- **Where the soft-warn lives.** The statusline's amber cost item now tints when
  **this session** crosses the budget threshold (it reads the session meter),
  while the **topbar cost footer remains the authoritative cumulative banner**
  (account-wide). We accept that a per-session amber can *under*-warn when spend is
  spread across several open conversations; the topbar gauge still fires on the
  cumulative total, so the ambient warning is not lost. For a single-user
  localhost tool the usual case is one active conversation, where the two
  coincide.

## Decision

Add `Server.sessionMeter`, a per-conversation `telemetry.Meter` built in
`newSession` on the account-wide meter's price book (exposed via the new pure
`Meter.PriceBook()` accessor). The `EvUsage` reducer records each turn into both
the account-wide meter (budget gauge, cap projection, Telemetry page) and the
session meter. `renderStatline` reads the session meter for tokens, cache-hit,
credits, the pre-flight estimate, and the soft-warn tint. Everything else stays
account-wide. No on-disk schema, route, or statusline **field** changes — only
the data source behind the existing statusline fields.

## Consequences

- Positive: the statusline now answers "what has *this* conversation cost",
  closing TECH_DEBT #2. The change reuses the existing pure `Meter` API, stays
  deterministic, adds no IO, and keeps the account-wide budget/enforcement
  surfaces correct. In the single-session demo both meters see identical records,
  so existing e2e/relative assertions are unaffected.
- Trade-off we accept: the turn is priced and recorded **twice** (once per meter)
  — trivial for one record per turn — and the statusline soft-warn is now
  per-session, with the cumulative banner deliberately left on the topbar gauge.
- Follow-up: this is the per-session half of the TECH_DEBT #9 pairing. The
  Telemetry budget / month-to-date rows still read the live global meter (reset on
  restart); reconciling them against the persisted ledger — so "remaining this
  month" survives a restart and can break out per session — is the remaining
  ledger-derived step.
