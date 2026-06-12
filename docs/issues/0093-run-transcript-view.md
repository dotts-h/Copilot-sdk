---
id: 0093
title: "Run transcript view — chat-order rendering of the event log (O3)"
status: closed
severity: low
group: 0090
depends_on: [0091]
github:
links:
  adr:
  prs: []
  issues: [0090]
  regression:
assets: []
---

## Summary

Add a view toggle on the run-detail page (`?view=transcript`, mirroring the `?window=`
pattern: clamped, garbage → default timeline) that flattens the same events into **chat
reading order** — the "Messages view" every vendor ships beside the tree (LangSmith's
Messages tab is the precedent). User messages and committed assistant messages render
through the **block-AST designed-output pipeline** (epic 0076 — its second consumer), with
tool steps as compact cards between them and lane transitions marked.

## Why now

Cheap (S): O1 already parses/groups the events; this is an alternate ordering + the
existing markdown renderer. The transcript is what humans actually read for long runs —
the timeline is for locating a step, the transcript for understanding the run.

## Touches

- `internal/web` — `runs.go` (view param threading + transcript renderer),
  `templates/fragments.html` (toggle control + transcript block).

## Acceptance

- [x] `?view=transcript` renders the chat-order view; default and garbage values render
      the timeline; the toggle re-fetches into `#main` like the window selector.
- [x] Message bodies go through `renderMarkdown` (block-AST) — callouts/code/tables in a
      recorded run render designed; tool cards stay compact; everything escaped per
      ADR-0001.
- [x] Pricing from O2 (when present) shows per assistant turn; unpriced logs render clean.
- [x] Unit tests cover ordering, the toggle clamp, and escaping; `make lint && make test`
      (floor 65%) green.

## Notes

S-sized; no ADR (presentation over O1's data; the view-param pattern is the established
`?window=` discipline).

**Closed:** chat-order transcript shipped as `clampRunView` + the pure `buildRunTranscript`
(events flattened chronologically with lane-transition markers; `EvToolStart`/`EvToolEnd`
joined via the generalized `popOpenTool`) projected by `transcriptRows`. Committed
user/assistant turns render through `renderMarkdown` (block-AST, epic 0076's second
consumer); tool calls render as compact cards; `EvUsage` credits (O2) attach per assistant
turn (trailing usage → its turn; lead usage → the next). The run-detail page carries a
`timeline ⇄ transcript` toggle; the header's summed-credit cross-check moved to the
view-agnostic `sumEventCredits`. Demo log enriched with designed markdown + priced turns so
the offline inspector showcases the view. No ADR (presentation over O1's data). Unit tests
cover ordering/markers, the clamp, escaping, and per-turn pricing; an e2e asserts the
designed render + toggle. `make lint && make test` (web 89.8%) + `make e2e` (157) green.
