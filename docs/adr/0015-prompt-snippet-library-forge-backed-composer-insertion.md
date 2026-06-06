# 0015. Prompt/snippet library: a forge-backed library inserted from the composer

- Status: accepted
- Date: 2026-06-06
- Deciders: Horia
- Related: `internal/ctxforge` (`snippet.go`: `Snippet`/`Snippet`/`AddSnippet`/
  `UpdateSnippet`/`RemoveSnippet`, `forge.go` `Snippets` field + validate),
  `internal/web` (`snippets.go`, `autocomplete.go` `matchSnippets`/`renderMenu`/
  `isReservedCommand`/`snippetExpansion`, `pages.go`, `hub.go`, `server.go`
  `handleSend`, `templates/fragments.html` `cmdMenu`/`snippetsPage`,
  `templates/index.html` `fillSnippet`), `internal/bootstrap` (seed),
  `docs/NEXT_FEATURES.md` item 3.4,
  [ADR-0001](0001-render-markdown-server-side-for-committed-agent-turns.md),
  [ADR-0003](0003-claude-cli-style-agents-built-in-chat-agent-and-per-agent-tool-allowlist.md)

## Context

Item 3.4 asks for a **prompt/snippet library**: saved, reusable prompts a user
can insert from the composer — "a lighter cousin of skills." Skills already
exist, so the design has to make the distinction sharp and reuse the existing
machinery (the forge-CRUD pattern and the slash-command autocomplete) rather than
inventing a parallel system.

The key conceptual line: a **Skill** is *system-message context* — its prompt is
compiled into the session's system message (`Forge.Compile`) when enabled, and it
can pin an agent. A **Snippet** is a *one-shot user prompt* — it is never compiled
into the system message, never toggled into a session, and carries no
model/tools/effort. It is purely a library entry that lands in the composer as
the next thing you send.

Three questions: **where snippets persist**, **how they surface and insert from
the composer**, and **how the insertion stays escape-safe**.

## Considered options

- **Where snippets persist.**
  - **In the forge as a first-class entity (chosen).** A snippet is a reusable
    building block, exactly the forge's remit (skills, instructions, agents, MCP
    servers, workflows). Adding `ctxforge.Snippet` + a `Forge.Snippets` slice
    (`json:"snippets,omitempty"`, backward-readable) reuses the entire CRUD path —
    validated builders, rollback-on-invalid save, the forms.go field helpers, a
    nav page, atomic persistence. Validation stays pure (`internal/ctxforge`).
  - **In `config.Config`.** Rejected: config is user *settings* (model, effort,
    budget, keybindings), not a registry of content; snippets are content with the
    same id/CRUD/uniqueness needs the forge already solves.

- **How snippets surface and insert.**
  - **Through the existing slash autocomplete, inserted client-side (chosen).**
    The composer's `/` menu already fetches `GET /commands` on each keystroke;
    snippets join that menu (their `id` is the `/trigger`). A snippet menu item
    is marked (`cmd-snippet`) and carries its body in a `data-body` attribute;
    clicking it runs `fillSnippet`, which drops the **body** (not "/trigger ")
    into the composer for editing — distinct from `fillCmd`, which fills a command
    name. No server round-trip, no form submit, so the composer's after-request
    `reset()` can't wipe the inserted text.
  - **A bare `/trigger` submitted directly expands-and-sends (chosen, secondary).**
    Because the trigger appears in the menu, users will also type it and press
    Enter. `handleSend` resolves a bare `/trigger` (no args) to the snippet body
    via `snippetExpansion` and sends it like a normal prompt, instead of dead-
    ending on "unknown command". **Reserved command/page slugs always win**
    (`isReservedCommand`), so a user snippet can never shadow `/clear`, `/model`,
    a nav slug, etc. — checked both at menu time (the colliding snippet is not
    offered) and at submit time.
  - **A new "insert" gate / its own route.** Rejected: the autocomplete is the
    stated surface and already does prefix-matching, escaping, and rendering; a
    bespoke route would duplicate it.

- **Escaping (ADR-0001).** The snippet body reaches the browser only as an
  `html/template`-escaped attribute value (`data-body`); `fillSnippet` reads it
  back as a plain string and assigns `input.value`, so it is never parsed as HTML.
  The library page escapes name/body like every other forge row. The expand-and-
  send path feeds the body straight into the normal prompt pipeline, which already
  escapes the rendered user turn.

## Decision

Add `ctxforge.Snippet` (`{id, name, body}`) as a forge entity persisted under the
additive `snippets` key, with pure `Validate` (slug id, name, body) and the
standard Add/Update/Remove builders + whole-forge uniqueness. Surface a
**Snippets** nav page (CRUD, no toggle — a snippet is never compiled into a
session) and fold snippets into the composer's `/` autocomplete: a marked menu
entry whose `data-body` `fillSnippet` inserts into the composer, plus an
expand-and-send fallback in `handleSend` for a bare `/trigger`. Reserved
command/page slugs always take precedence over a snippet of the same id.

## Consequences

- Positive: a complete library with one new entity and one new page, reusing the
  forge-CRUD and autocomplete machinery; domain logic stays pure and
  unit-tested; the skill-vs-snippet line is explicit (system-message context vs
  one-shot prompt). Insertion is escape-safe (ADR-0001) and needs no new route —
  it rides `GET /commands` and `POST /send`.
- Trade-off we accept: snippets carry no model/effort/tools and aren't compiled
  into the system message — that is the whole point of the skill/snippet split.
  Collisions resolve in favour of built-in commands, so a snippet can't be named
  after one (it just won't be offered/expanded as a snippet).
- Known limitation: the composer is a single-line `<input>`, whose value-
  sanitisation strips newlines, so a **multi-line** snippet body inserted into the
  composer is flattened to one line (the expand-and-send path preserves newlines,
  since the body never passes through the input). Tracked as TECH_DEBT #15
  (pay down by switching the composer to a `<textarea>`).
- Contract change: the forge grew the additive `snippets` key (omitempty, older
  files read clean) and the `/snippets…` route group — recorded in CONTRACTS.
  Covered by `internal/ctxforge` `TestSnippetValidate`/`TestForgeSnippetCRUD`/
  `TestForgeValidateRejectsDuplicateSnippet`/`TestForgeSnippetsPersistAndOmitEmpty`
  and `internal/web` `TestCommandsMenuIncludesSnippets`/
  `TestCommandsMenuEscapesSnippetBody`/`TestReservedCommandBeatsSnippet`/
  `TestSnippetsPageListsSnippets`/`TestSnippetCreateAndDelete`/
  `TestSlashSnippetExpandsAndSends`/`TestUnknownSlashIsNotSent`; the browser
  insert path by `e2e/tests/e2e.spec.ts` ("prompt/snippet library").
