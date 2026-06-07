---
id: 0033
title: "Generic telemetry.AppendOnlyStore[T] — collapse the SpendStore/RunStore machinery (roadmap v5, item H1)"
status: closed
severity: medium
group: 0030
github:
links:
  adr:
  prs: [58]
  issues: [0030]
  regression:
assets: []
---

## Summary

`telemetry.SpendStore` (`internal/telemetry/history.go`) and `telemetry.RunStore`
(`internal/telemetry/runs.go`) carried two **near-identical copies** of the same
on-disk-store machinery: load-or-empty on a missing file, atomic temp-file +
rename save, "tolerate a newer schema `version`" forward-compatibility, "reject a
corrupt file" guard, an ephemeral (`dir == ""`) mode that never writes, and
mutex-guarded `Append`/`Records`/`Count`. The only differences were the file
name, the envelope's array key (`records` vs `runs`), the schema version, and a
per-record stamp on `Append` (spend stamps `At`, runs stamps `FinishedAt`). **H1
collapses that duplication** into one generic `AppendOnlyStore[T any]`
(`internal/telemetry/store.go`); `SpendStore`/`RunStore` become thin typed
wrappers that embed it and preserve their **exact** public API and on-disk shape.
A debt-paydown flagged in the B3 review and deferred then as scope creep. Source:
`docs/NEXT_FEATURES.md` item H1; epic [0030](0030-epic-orchestration-visibility-polish.md).

## Repro
1. Read `history.go` and `runs.go` side by side.
   - **Expected:** the persistence discipline (atomic write, missing=empty,
     invalid=error, newer-version tolerance, ephemeral no-write) lives in **one**
     place, so a third persisted history would not triple it.
   - **Actual (before H1):** the two files duplicate the whole machinery; only the
     file name, array key, version, and the append-stamp differ.

## Proposed resolution

- **`internal/telemetry` (new `store.go`):** add `AppendOnlyStore[T any]` carrying
  the shared machinery — `loadAppendOnlyStore` (missing=empty / corrupt=error /
  newer-version-tolerant / ephemeral), `Append` (per-record stamp hook + atomic
  `save`), `Records`, `Count`. Parameterized over the on-disk file name, the
  envelope array key (`records`/`runs`), the schema version, an error-message
  noun, and an optional per-record `stamp func(T) T`.
- **`history.go` / `runs.go`:** re-express `SpendStore`/`RunStore` as
  `struct{ *AppendOnlyStore[…] }` embeddings; `Append`/`Records`/`Count` are
  promoted, so the **public API is unchanged**. The spend `At` / run `FinishedAt`
  defaulting moves into a `stampSpend` / `stampRun` hook. The v1→v2 spend tag
  read needs **no migration code** — the attribution tags are additive, so a v1
  record simply reads back with empty tags (forward-compat, not a converter).
- **On-disk contract unchanged.** The envelope marshaler reproduces the exact
  `{"version":N,"<key>":[…]}` shape; `json.MarshalIndent` re-indents it the same,
  so a file written by the generic store is **byte-identical** to the pre-H1
  output, and a pre-H1 `spend.json`/`runs.json` loads identically. No new file
  format, no schema change, no `telemetry → web/copilot/config` import (the
  package stays pure and dependency-free).
- **Tests:** the **existing** spend + run round-trip / atomic / migration /
  ephemeral tests are the spec and stay green **unchanged**. Added: a direct
  `AppendOnlyStore[T]` unit test on a throwaway `widget` record
  (round-trip, ephemeral never writes, missing→empty, corrupt→reject,
  newer-version tolerated, atomic write leaves no `.tmp`, missing-key→empty); and
  on-disk-tag pins asserting `spend.json`/`runs.json` still carry the literal
  `version` + `records`/`runs` keys (the stable contract didn't drift).

## Resolution (shipped)

Built as specified — a pure-`telemetry` refactor guarded by the existing tests, no
behavior change, no schema change. `internal/telemetry/store.go` adds
`AppendOnlyStore[T any]` (the shared on-disk machinery) plus a hand-built
`envelope[T]` marshaler that emits the exact `{"version":N,"<key>":[…]}` shape so
`json.MarshalIndent` reproduces the pre-H1 bytes **byte-for-byte** (verified: a
custom-marshaler value and the old struct literal produce identical indented
output, nil slice → `null` alike). `SpendStore`/`RunStore` are now
`struct{ *AppendOnlyStore[SpendRecord|RunRecord] }` embeddings; `Append`/`Records`/
`Count` are promoted unchanged, and the `At`/`FinishedAt` defaulting moved into
`stampSpend`/`stampRun` hooks. The v1→v2 spend read needs no converter — the
attribution tags are additive, so a v1 record reads back with empty tags. The
duplicated bodies in `history.go`/`runs.go` (and their `os`/`json`/`filepath`/
`sync`/`fmt` imports) are gone.

Tests: all pre-H1 spend + run guards stay green unchanged
(`TestSpendStoreAppendPersistsAndReloads`, `TestSpendStoreEphemeralNeverWrites`,
`TestLoadSpendStoreMissingIsEmpty`, `TestLoadSpendStoreRejectsCorruptFile`,
`TestLoadSpendStoreToleratesNewerSchema`, `TestSpendStoreReadsV1RecordWithoutTags`,
`TestSpendRecordRoundTripsAttributionTags`; the `Run*` equivalents +
`TestRunStoreStampsFinishedAtWhenZero`). Added (`internal/telemetry/store_test.go`):
`TestAppendOnlyStoreRoundTrip`, `TestAppendOnlyStoreEphemeralNeverWrites`,
`TestAppendOnlyStoreMissingIsEmpty`, `TestAppendOnlyStoreRejectsCorruptFile`,
`TestAppendOnlyStoreToleratesNewerSchema`, `TestAppendOnlyStoreMissingKeyIsEmpty`
(the bare generic machinery on a throwaway `widget`), and the on-disk-contract
pins `TestSpendStoreOnDiskTagsAreStable` / `TestRunStoreOnDiskTagsAreStable`. Gates
green (`make lint && make test`, telemetry coverage 96.8%); pure-telemetry, so e2e
was untouched.

Docs: CONTRACTS §4 (both store entries note they now share
`telemetry.AppendOnlyStore[T]` while the on-disk JSON tags are the unchanged stable
contract). TECH_DEBT — the duplicated-machinery row recorded paid (guarding test
named). No new ADR — an internal extraction that **preserves** the ADR-0009 ledger
+ ADR-0022 run envelopes, referenced from CONTRACTS. No REGRESSIONS entry — no bug
was found-and-fixed; byte-identity, the v1 migration read, the atomic temp+rename,
and the ephemeral no-write were guarded preemptively (self-review with
`/code-review` high effort confirmed all four held and no `telemetry → web/copilot`
import slipped in). Shipped on branch `claude/append-only-store`. **Third and final
child of epic 0030 (roadmap v5) — its merge closes the epic.**

## Notes
- **No ADR:** a refactor-only paydown that keeps the persisted ADR-0009 (spend
  ledger) and ADR-0022 (run history) envelopes byte-identical; only a *changed*
  persisted contract would warrant a new ADR, and the contract is explicitly
  unchanged. Captured as a reference in CONTRACTS §4.
- **Differentiator:** keeps the persistence discipline single-sourced — a future
  third persisted history rewraps the generic store instead of copying it a third
  time. Third (final) child of epic 0030 (roadmap v5 — orchestration visibility &
  polish); the epic closes on this merge (V3, V10, H1 all shipped).
- **Numbering:** issue **0033** (next free after 0032), third build of epic
  **0030**. No ADR consumed (highest ADR stays 0022).
