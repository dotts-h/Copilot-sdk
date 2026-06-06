---
id: 0012
title: Prompt/snippet library (Tier 3, item 3.4)
status: closed
severity: low
group: 0007
github:
links:
  adr: ../adr/0015-prompt-snippet-library-forge-backed-composer-insertion.md
  prs: []
  issues: [0007]
  regression:
assets: []
---

## Summary

Add a library of saved, reusable prompts ("snippets") a user can insert from the
chat composer — a lighter cousin of skills. Where a skill is *system-message
context* (compiled into the session when enabled), a snippet is a *one-shot user
prompt* that lands in the composer ready to send. Source: `docs/NEXT_FEATURES.md`
item 3.4 (the last validated roadmap item; closes Tier-3 epic 0007).

## Repro
1. Want to reuse a prompt you type often ("review this PR", "explain this code").
2. Expected: save it once, then insert it from the composer in a keystroke.
3. Actual (before): no such surface — every prompt is retyped or pasted by hand.

## Resolution

- **Model (pure, `internal/ctxforge`):** `Snippet{id, name, body}` as a
  first-class forge entity (`snippet.go`), persisted under the additive
  `snippets` key (`omitempty`, backward-readable). `Validate` enforces a slug id,
  a name, and a body; standard `AddSnippet`/`UpdateSnippet`/`RemoveSnippet`
  builders (rollback-on-invalid) + whole-forge uniqueness. No `Enabled`/toggle —
  a snippet is never compiled into a session (it is *not* `Forge.Compile`d).
- **Library page (`internal/web`):** a **Snippets** nav page (CRUD: add/edit/
  delete, no toggle) mirroring the forge-CRUD pattern (`snippets.go`,
  `forms.go` field helpers, `editForge` rollback path).
- **Composer insertion:** snippets join the existing `/` autocomplete
  (`GET /commands`). A snippet menu entry is marked (`cmd-snippet`) and carries
  its body in `data-body`; `fillSnippet` (index.html) inserts the **body** into
  the composer for editing. A bare `/trigger` submitted directly **expands and
  sends** the body (`snippetExpansion` in `handleSend`). Built-in commands /
  nav slugs always win over a same-named snippet (`isReservedCommand`), at both
  menu time and submit time. All snippet text is HTML-escaped (ADR-0001). Design
  recorded in **ADR-0015** (forge vs config; autocomplete insertion vs a new
  gate; escape-safe `data-body`).
- **Demo seed (`internal/bootstrap`):** two representative snippets (`review-pr`,
  `explain`) so the page and autocomplete are self-contained offline.

## Notes

Guarding tests: `internal/ctxforge` `TestSnippetValidate`, `TestForgeSnippetCRUD`,
`TestForgeValidateRejectsDuplicateSnippet`, `TestForgeSnippetsPersistAndOmitEmpty`;
`internal/web` `TestCommandsMenuIncludesSnippets`, `TestCommandsMenuEscapesSnippetBody`,
`TestReservedCommandBeatsSnippet`, `TestSnippetsPageListsSnippets`,
`TestSnippetCreateAndDelete`, `TestSlashSnippetExpandsAndSends`,
`TestUnknownSlashIsNotSent`; browser `e2e/tests/e2e.spec.ts` ("prompt/snippet
library": add via form + insert from autocomplete). Contract change (additive
`ctxforge.Snippet` schema + `/snippets…` routes) recorded in CONTRACTS; escaping
and reserved-name precedence gotchas in REGRESSIONS; the single-line-composer
newline-flatten limitation as TECH_DEBT #15. Closes the last child of epic 0007 —
**epic 0007 is now closed.**
