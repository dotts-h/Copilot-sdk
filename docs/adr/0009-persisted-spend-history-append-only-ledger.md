# 0009. Persisted spend history: an append-only, atomically-written ledger

- Status: accepted
- Date: 2026-06-05
- Deciders: Horia
- Related: `internal/telemetry` (`SpendStore`, `SpendRecord`, `DailyTotals`,
  `ModelShares`, `WriteCSV`), `internal/web` (`session.go` EvUsage,
  `telemetryPartial`, `handleSpendExport`), `internal/bootstrap` (`Build`,
  `seedSpend`), `docs/NEXT_FEATURES.md` item 1.3,
  [ADR-0007](0007-pre-flight-turn-cost-estimate-prices-context-as-fresh-input.md),
  [ADR-0008](0008-budget-guardrails-soft-warn-and-hard-cap-gate.md)

## Context

The `telemetry.Meter` is in-memory only, so all spend accounting dies on
restart. The README leads with "a coding session never surprises you on the
bill," but after a restart there is no bill at all — the gauge resets to zero.
`NEXT_FEATURES.md` item 1.3 asks to persist per-session/per-day spend and add a
trend view (spend over time, per-model share, CSV export) to the Telemetry page.
This is the difference between a live gauge (1.2/1.1) and an accountable ledger.

Three questions had to be answered: *where the store lives*, *the on-disk format
and how it is written*, and *how the persisted history relates to the live
meter on the Telemetry page*.

## Considered options

- **Where the store lives.** A new file in `internal/telemetry` (chosen): the
  records are telemetry types (`Cost`/`Credits` live there) and the aggregations
  are pure functions next to `Price`/`Budget`. The `SpendStore` is the one IO
  edge — the rest of the package stays pure — exactly as `config` is "dependency-
  free" yet does its own atomic disk IO. (Rejected: a `config` sibling — spend
  records aren't user settings; an `internal/spend` package — needless split from
  the cost types it priced.)
- **On-disk format + write discipline.** A single versioned JSON document
  (`{"version":1,"records":[…]}`) rewritten **atomically** (temp-file + rename +
  validate-on-load), mirroring `config.Save` (chosen). (Rejected: **JSON Lines**
  with `O_APPEND` — genuinely append-only on disk and O(1) per turn, but it is
  *not* the established temp-file+rename atomicity the codebase standardises on,
  and a torn final line needs bespoke recovery. For this localhost single-user
  tool the per-turn volume is tiny, so the O(n) full rewrite is a non-issue and
  buys one consistent persistence pattern across config/forge/spend.) "Append-
  only" here is a property of the **records** (immutable, history only grows),
  not of the syscall.
- **History vs. the live meter on the page.** Keep the existing summary rows and
  per-model table sourced from the **live meter** (this process) and add a new
  **trend section** sourced from the **persisted ledger** (all time), clearly
  separated under "Spend history" (chosen). (Rejected: reconciling the budget /
  month-to-date rows against the ledger now — a larger behavior change with
  double-counting risk; deferred to a follow-up and noted in TECH_DEBT.)

## Decision

`telemetry.SpendStore` loads from `dir/spend.json` (missing = empty, present-
but-invalid = error), appends a `SpendRecord` per metered turn, and persists the
whole ledger atomically. An **empty dir yields an ephemeral, in-memory-only
store** — the offline demo and unit tests use it so they never write to a real
config directory. Aggregation is three pure functions over a record slice:
`DailyTotals` (UTC-day buckets, ascending), `ModelShares` (per-model fraction of
total, spend-descending), and `WriteCSV` (a fixed-column export with credits
derived from USD).

The web layer appends a record in the `EvUsage` reducer (best-effort: a disk
error is logged, never surfaced — the live meter and stream are unaffected). The
Telemetry page renders the most-recent-14-day trend and per-model share as
scaled bars, with a `GET /telemetry/export.csv` download. `bootstrap.Build`
loads the ledger from the config dir for the real app and seeds a deterministic
ephemeral one for demo.

The on-disk schema is **stable** (CONTRACTS §4): the `records` array is the
contract, the `version` tag gates migrations, and a newer minor version's extra
fields are ignored on read so the file stays forward-readable.

## Consequences

- Positive: spend now **survives restart** and is queryable as a trend and a CSV
  — the accountable-ledger half of the cost story. The store reuses the atomic-
  write discipline already proven for config/forge; the aggregations are pure and
  fuzz-adjacent (same totality guarantees as pricing). Demo/tests never touch disk.
- Trade-off we accept: the append persists **synchronously inside the single Hub
  pump goroutine** under `s.mu`. For a localhost single-user tool the per-turn
  write is sub-millisecond and the volume is one record per turn, so this is
  fine; batching/async is a noted follow-up if volume ever grows (TECH_DEBT).
- Trade-off: the full-file rewrite is O(n) per turn. Bounded by the same low
  volume; the consistency of one persistence pattern is worth more than O(1).
- Follow-ups: reconcile the budget / month-to-date rows against the ledger (so
  the gauge and the ledger agree across restarts), and pair with item 3.2
  (per-session totals) — the records already carry `SessionID`. A tighter cap
  (ADR-0008) can now fold a persisted cache-hit ratio and rolling output average.
