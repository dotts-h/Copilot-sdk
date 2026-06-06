---
id: 0018
title: Textarea composer — Enter sends, Shift-Enter newline (roadmap v2, item C2)
status: closed
severity: low
group: 0013
github:
links:
  adr:
  prs: []
  issues: [0013, 0012]
  regression: REGRESSIONS "the composer is a textarea — multi-line snippets keep newlines"
assets: []
---

## Summary

The chat composer was a single-line `<input name="prompt">`. An `<input
type=text>` value-sanitises away newlines, so a multi-line snippet body inserted
via `fillSnippet` collapsed to one line in the composer (TECH_DEBT #15 / ADR-0015
shipped the snippet library with this known limit). Switch the composer to a
`<textarea name="prompt">`: **Enter sends, Shift-Enter inserts a newline**. This
(a) fixes the snippet flatten-on-insert and (b) enables multi-line prompts
generally. Small, self-contained, compounding UX win the snippet library already
wanted. Source: `docs/NEXT_FEATURES.md` item C2.

## Repro
1. Add (or use the seeded) a snippet whose body spans several lines.
2. In the chat composer type `/` and pick that snippet to insert it.
   - **Expected:** the body lands in the composer with its line breaks intact,
     ready to edit; Enter sends, Shift-Enter adds a newline.
   - **Actual (before):** the body flattened to a single line (the `<input>`
     stripped the newlines); the only newline-preserving path was sending the
     bare `/trigger`, which never touched the input.

## Resolution (shipped)

Built on `claude/next-features-research-8aBvS`. UI-only — no contract/route
change (POST /send and GET /commands are unchanged).

- **Template (`internal/web/templates/fragments.html`, `chatPage`):** the
  `<input type="text" name="prompt">` became a `<textarea name="prompt" rows="1">`
  carrying the same `hx-get="/commands"` autocomplete wiring, an `aria-label`
  (a textarea has no visible label), and `onkeydown`/`oninput` hooks. The form's
  `hx-on::after-request` keeps the REGRESSIONS #8 `event.target === this` guard
  and now refocuses (and re-autosizes) the textarea.
- **JS (`internal/web/templates/index.html`):** a `composer()` helper resolves the
  field once (all four former `#composer input[name=prompt]` call sites —
  `fillCmd`, `fillSnippet`, overlay-close refocus, `focusComposer` — route through
  it); `composerKeydown(e)` restores Enter-to-send (a textarea inserts a newline on
  Enter natively, so a bare Enter is intercepted and `form.requestSubmit()` fires
  the submit htmx hooks; Shift-Enter and any modifier fall through to a newline, and
  an `isComposing` guard avoids submitting mid-IME); `autosize(el)` grows the field
  to fit its content (capped by a CSS `max-height`, then scrolls), accounting for
  the `border-box` borders so it doesn't keep a permanent scrollbar.
- **CSS (`internal/web/static/app.css`):** `#composer textarea` (was `input`) with
  `resize:none`, `line-height`, `max-height:12rem`, `overflow-y:auto`, and the bar
  bottom-aligned so the button stays put as the field grows.
- **Demo (`internal/bootstrap/bootstrap.go`):** seeds a multi-line `checklist`
  snippet so the offline demo / e2e can prove inserting it keeps the line breaks.

Guarded by `internal/web` `TestComposerRendersTextarea` (textarea + autocomplete
wiring + after-request guard, no stray `<input>`),
`TestComposerKeydownAndAutosizeWired` (Enter/Shift-Enter/autosize JS present),
`TestCommandsMenuPreservesMultilineSnippetBody`; e2e `ux.spec.ts` (Shift-Enter
newline; a bound keybinding char typed in the composer stays text, not an action)
and `e2e.spec.ts` (inserting a multi-line snippet keeps its newlines). The
keydown dispatcher already ignored `INPUT/TEXTAREA/SELECT/contenteditable`, so
typing bound shortcut chars in the composer is still text.

## Notes

- **No ADR.** Enter-vs-Shift-Enter is the item's own spec (not a contested
  decision), and autosize-with-cap is the conventional chat-composer choice — a
  REGRESSIONS guard + this issue suffice (NEXT_FEATURES did not flag C2 "Needs an
  ADR", unlike C1/B2).
- **Clears TECH_DEBT #15** (→ Paid). The flatten gotcha in REGRESSIONS inverts:
  with a textarea the inserted body *keeps* its newlines; the dead-end note ("don't
  assert newlines survive the input") is updated accordingly.
- **Local browser is blocked** — the e2e relies on remote CI; structure-only
  assertions on the shared demo session (same family as the shared-session gotcha).
