---
id: 0009
title: Diff review lane (Tier 3, item 3.1)
status: closed
severity: medium
group: 0007
github:
links:
  adr: ../adr/0012-diff-review-lane-for-file-write-permissions.md
  prs: []
  issues: [0007]
  regression:
assets: []
---

## Summary

WEB_UI_PLAN UX principle #5 ("diffs get a review lane") was only partially met: a
file-write permission rendered as the same bare one-line control as a shell
command, so approving a write was an act of faith — you couldn't see *what* the
agent would change, even though the runtime hands us the proposed diff. Add a
dedicated review affordance. Source: `docs/NEXT_FEATURES.md` item 3.1.

## Repro
1. Run a turn where the agent proposes a file edit (a `PermissionRequestWrite`).
2. Expected: a collapsible diff with the change, and approve/reject on it.
3. Actual (before): `⚠ allow write file: x? [approve] [reject]` — no diff shown.

## Resolution

- `copilot.PermissionRequest` gained write-only `FileName`, `Intention`, `Diff`,
  populated from `sdk.PermissionRequestWrite` by the pure `permWriteFields`
  helper (empty for other kinds).
- New pure, deterministic `parseUnifiedDiff` (`internal/web/diff.go`) turns the
  unified diff into typed lines (add/del/context/hunk/meta) with old/new gutter
  numbers and add/remove tallies — unit-tested with no browser.
- `renderPermForm` now takes the whole `PermissionRequest`: when the diff parses,
  it renders the **`permReview`** lane — file name, diffstat (`+adds −dels`),
  intention, and a collapsible (`<details open>`) side-numbered inline diff — with
  approve/reject attached, posting to the **same `/perm/{id}` flow**. Any other
  request (and a write with no parseable diff) keeps the compact form.
- Untrusted file content is HTML-escaped (`html/template`, ADR-0001). The
  add/remove distinction is double-encoded (per-line tint **and** a `+`/`-`
  marker) with AA-safe foregrounds, so the lane passes the WCAG scan.
- The offline demo emits a file-write permission so the lane is exercised in
  e2e/a11y. Design recorded in **ADR-0012** (server-side parse; inline vs
  side-by-side; reuse the `/perm` seam vs a new gate).

## Notes

Guarding tests: `internal/web` `TestParseUnifiedDiff*` (parser, table-driven),
`TestRenderPermFormReviewLane` (escaping + diffstat + line classes),
`TestRenderPermFormWriteWithoutDiffStaysCompact`, `TestRenderPermFormEscapes`
(compact fallback), `TestPermissionWithDiffEmitsReviewLane` (reducer);
`internal/copilot` `TestPermWriteFields`, `TestDescribePermission`; browser
`e2e/tests/e2e.spec.ts` "a file-write permission renders a diff review lane and
approves" + the a11y chat scan (now waits for `#perms .perm-review`). Contract
change (additive `PermissionRequest` fields) recorded in CONTRACTS; gotchas
(hunk-header detection, demo-perm→a11y `.no` baseline) in REGRESSIONS. Closes the
diff-lane item of epic 0007; remaining Tier-3 follow-ons are 3.3 / 3.4.
