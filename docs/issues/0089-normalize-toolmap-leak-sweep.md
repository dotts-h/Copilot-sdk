---
id: 0089
title: "Sweep orphaned toolNames/toolMeta entries in normalize.go on session teardown"
status: open
severity: low
group: 0086
depends_on: []
github:
links:
  adr: []
  prs: []
  issues: [0086]
  regression:
assets: []
---

## Summary

`SDKClient` threads tool names/metadata across the start→complete event pair via two maps
keyed by tool-call id: `c.toolNames` and `c.toolMeta` (`internal/copilot/normalize.go`).
They are populated on `ToolExecutionStartData` and **deleted on `ToolExecutionCompleteData`**.
If the SDK errors after a tool start with no matching complete, the entry is **orphaned** —
it leaks until `Close()`. Bounded to process lifetime, but it accumulates across many
tool-erroring sessions in a long-lived instance. (The sibling `reasoned` map is already swept
on idle; these two are not.)

## Why now

Low-severity hygiene; pick up alongside the other code-health items (epic 0086) or when
long-running multi-session use becomes real. Cheap and local.

## Touches

- `internal/copilot/normalize.go` / `sdkclient.go` — clear orphaned `toolNames`/`toolMeta`
  entries for a session being torn down (in `DeleteSession` and/or `Close`). Two viable
  shapes: key the maps per-session (so teardown drops the whole session's bucket), or sweep
  by a session-prefixed id on `DeleteSession`. Keep it under the existing `c.mu`; do not
  change the start/complete threading behavior.

## Acceptance

- [ ] An orphaned tool entry (start with no complete) is reclaimed on session teardown —
      assert via a test that drives a start-without-complete then `DeleteSession`/`Close` and
      checks the maps are empty for that session.
- [ ] Normal start→complete threading is unchanged (existing normalize tests pass).
- [ ] `make lint && make test` (floor 65%) green, run under `-race` (the maps are `c.mu`-guarded).

## Notes

Different package from 0087/0088 (`internal/copilot` vs `internal/web`) → **fully
parallel-safe**. This one *does* add a guard test (it fixes a real leak, unlike the pure
splits). SemVer **patch**.
</parameter>
