# 0012. Diff review lane for file-write permissions

- Status: accepted
- Date: 2026-06-05
- Deciders: Horia
- Related: `internal/copilot` (`PermissionRequest`, `permWriteFields`,
  `permissionHandler`), `internal/web` (`diff.go` `parseUnifiedDiff`, `render.go`
  `renderPermForm`, `templates/fragments.html` `permReview`, `static/app.css`,
  `demo.go`), `docs/NEXT_FEATURES.md` item 3.1, `docs/WEB_UI_PLAN.md` UX
  principle #5, [ADR-0001](0001-render-markdown-server-side-for-committed-agent-turns.md),
  [ADR-0008](0008-budget-guardrails-soft-warn-and-hard-cap-gate.md)

## Context

WEB_UI_PLAN UX principle #5 — "diffs get a review lane" — was only partially met.
A file-writing agent's permission prompt rendered as the same bare one-line
control as a shell command (`⚠ allow write file: x? [approve] [reject]`), even
though the runtime hands us the full proposed change. The SDK's
`PermissionRequestWrite` carries a unified `Diff`, the `FileName`, and a
human-readable `Intention` (`rpc/zsession_events.go`), but `describePermission`
collapsed all of that to `"write file: " + FileName`. So approving a write was an
act of faith: you could not see *what* the agent was about to change. Item 3.1
asks for a dedicated review affordance so file-writing agents feel trustworthy.

Three questions had to be answered: **where the diff is parsed**, **inline vs
side-by-side**, and **how approve/reject binds** to the turn.

## Considered options

- **Where the diff is parsed.**
  - **Server-side, pure (chosen).** A new `parseUnifiedDiff` in
    `internal/web/diff.go` turns the unified-diff string into typed lines
    (add/del/context/hunk/meta) with old/new gutter numbers and add/remove
    tallies. It has no IO and no browser dependency, so it is unit-tested on its
    own (table-driven, deterministic), and the rendering escapes every line via
    `html/template` before it reaches the browser. This is the same escape-first,
    "reduce + project, testable without a browser" stance as the markdown
    renderer (ADR-0001).
  - **Client-side (JS diff lib + DOM).** Rejected for the same reasons ADR-0001
    rejected client-side markdown: it makes the rendering browser-only-testable,
    needs a vendored JS dependency, and pushes escaping of untrusted file content
    into the client.

- **Inline vs side-by-side.**
  - **Inline unified (chosen).** The runtime already gives a *unified* diff, so an
    inline rendering is a near-direct projection — deterministic, with no need to
    pair added/removed lines into synthetic side-by-side rows. It reads linearly
    for a screen reader, fits the narrow (56rem) chat column without horizontal
    split, and keeps the parser small. Each line shows both old and new line
    numbers in the gutter, so the "two sides" information is still present.
  - **Side-by-side.** Rejected for now: it needs line-pairing with blank fillers,
    doubles the horizontal space in an already-narrow column, and complicates the
    parser and the a11y reading order — cost without a clear win for this tool.
    Left as possible future work; the parsed `diffView` is general enough to feed
    a side-by-side renderer later.

- **How approve/reject binds.**
  - **Reuse the existing `/perm/{id}` flow (chosen).** The review lane is the same
    `<form>` posting `approve=1|0` to `POST /perm/{id}` as the compact permission
    form; only the body is richer. No new route, no new bridge, no app-level gate.
    This is deliberately **unlike** the budget hard-cap gate (ADR-0008), which is
    an app-level `/budget/{action}` gate because it pauses *before* `Send`; a file
    write is a genuine SDK permission, so it stays on the SDK permission seam.
  - **A new review route / app-level hold.** Rejected — it would duplicate the
    permission bridge and invent a second approval path for no behavioural gain.

## Decision

Extend the normalized `copilot.PermissionRequest` with `FileName`, `Intention`,
and `Diff`, populated from `sdk.PermissionRequestWrite` by the new pure
`permWriteFields` helper (empty for every other request kind). In the web layer,
`renderPermForm` takes the whole `PermissionRequest`: when its `Diff` parses as a
unified diff (`parseUnifiedDiff(...).OK`), it renders the **`permReview`** lane —
the file name, a diffstat (`+adds −dels`), the intention, and a collapsible
(`<details open>`) inline diff with side-numbered, typed lines — with the
approve/reject buttons attached, posting to the same `/perm/{id}` flow. Every
other request (and a write whose diff doesn't parse) renders the unchanged
compact form. The diff is added to the offline demo so the lane is exercised in
the browser e2e/a11y suites.

## Consequences

- Positive: approving a file write is now an informed review, closing the last
  gap in WEB_UI_PLAN principle #5. The diff parser is pure and deterministic
  (unit-tested with no browser); untrusted file content is HTML-escaped
  (ADR-0001); approve/reject reuse the existing seam, so no new route/contract on
  the turn-answer path. The add/remove distinction is triple-encoded — per-line
  tint, a `+`/`-` gutter marker, and a visually-hidden "added"/"removed" label for
  screen readers (the marker is `aria-hidden`) — and foregrounds stay on AA-safe
  tokens, so the lane passes the WCAG scan and reads meaningfully without color or
  sight.
- Trade-off we accept: the lane is **inline only** — side-by-side is deferred. The
  rendered diff is bounded at `maxDiffLines` (400) with an elision note so a huge
  change can't flood the timeline or balloon the SSE fragment; the full change
  still applies on approve.
- New baseline: surfacing a permission in the demo brought the reject button
  (`.no`, white-on-red) into the a11y scan for the first time; it joins the
  documented destructive-control contrast baseline (`.abort`/`.plan-reject`/
  `.elicit-no`) rather than silently passing — the review lane's *own* diff body
  is fully AA.
- Contract change: `copilot.PermissionRequest` grew three write-only fields
  (additive, backward-compatible) — recorded in CONTRACTS. The normalization is
  covered by `internal/copilot` `TestPermWriteFields` / `TestDescribePermission`.
