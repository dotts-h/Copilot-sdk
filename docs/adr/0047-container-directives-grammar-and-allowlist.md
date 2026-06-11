# 0047. Container directives — in-house `:::name{attrs}` grammar, resolved against a registry allowlist

- Status: accepted
- Date: 2026-06-11
- Deciders: Horia
- Related: issue 0081 (epic 0076), [ADR-0045](0045-block-ast-seam-for-markdown-rendering.md), [ADR-0046](0046-callout-blocks-token-mapping.md), [ADR-0001](0001-render-markdown-server-side-for-committed-agent-turns.md), [ADR-0025](0025-semantic-color-token-tier.md), `internal/web/markdown.go`

## Context

R5 (the last structural slice of epic 0076) gives the model a **governed vocabulary of
designed blocks it can author** — `:::card`, `:::details{summary=…}`, and whatever we
register next — instead of a new bespoke regex per component (the path R2 callouts, R3
code blocks, R4 tables each took). The research pass named "a designed component the model
can emit" as the safest generative-UI mechanism for a server-rendered, no-build-chain
stack: richer than markdown, but every byte still flows through the escape-first whitelist
(ADR-0001).

Three things had to be settled before any code: the **syntax** to recognize, how an
authored block is **resolved to a renderer** (and what happens when it can't be), and how
**attributes** cross the escape-first boundary. The cardinal risk is the same one ADR-0045
named: a block that can name arbitrary markup. A directive grammar is exactly a mechanism
for the model to name a component — so the resolution rule is the security boundary, not a
convenience.

## Considered options

- **Adopt `remark-directive` (or any Node/Goldmark plugin)** — rejected for the reason the
  whole epic rejects libraries (ADR-0045, REGRESSIONS #3): a Node toolchain / foreign
  rendering path, and the escape-first invariant re-established around someone else's AST.
  We take the *syntax* `remark-directive` popularized; we reject its machinery.
- **A blocklist / sanitizer** — parse any `:::name`, render it, strip "dangerous" names or
  attributes after the fact. Rejected: blocklists are open by default — a name or attribute
  we forget to forbid renders. The epic's invariant is the opposite (allowlist > blocklist,
  safe by construction).
- **Per-component bespoke syntax** (a `:::card` regex, a `:::details` regex, …) — rejected:
  that is the very duplication R5 exists to end; each new designed block would again be new
  parser surface to fuzz, not a registry entry.
- **In-house `:::` container grammar resolved against a Go registry allowlist** — one
  parser for the fence shape, a closed map from name → renderer; an unregistered name is
  **not a directive at all**.

## Decision

We add an **in-house container-directive block** on the ADR-0045 seam, resolved against a
**closed Go registry**. The registry — not the parser — is the trust boundary: a name the
registry does not hold never produces directive markup.

### Grammar

- A container **opens** with a line that, trimmed, is `:::` followed by a bare component
  name (`[a-z][a-z0-9-]*`), optionally followed by a brace-delimited attribute list:
  `:::card`, `:::details{summary=Build steps}`, `:::details{summary="Build steps"}`.
- It **closes** with a line that, trimmed, is a bare `:::`.
- The body is everything between, **parsed recursively** as blocks (`parseLines`) — so a
  directive can contain paragraphs, lists, callouts, even nested directives, exactly as a
  blockquote/callout body can. Nesting is tracked by fence depth: a named `:::name` line
  inside the body increments depth, a bare `:::` decrements, and only the close that
  returns depth to zero ends the outer container. Recursion is **bounded** (a fixed
  max-nesting depth, beyond which the inner `:::` degrades to text) so pathological input
  can't blow the parse stack — the same defensive posture as the rest of the subset.
- An **unterminated** container (EOF before the matching close) degrades: the `:::name`
  line is treated as ordinary text and the body parses as normal blocks. Nothing is
  swallowed.

### Resolution — the allowlist invariant

`parseLines` recognizes a directive **only when the name is registered**, precisely as a
callout is recognized only when `[!KIND]` is in `calloutKinds` (ADR-0046). The registry is
a closed `map[string]directiveSpec`; each spec carries the `frag` template name and a
projector that turns the parsed, validated attributes into that template's data (applying
defaults, dropping unrecognized keys).

> **An unregistered name is not a directive.** The `:::foo` line degrades to ordinary
> markdown (escaped paragraph text); the body renders as normal blocks. The renderer never
> emits markup for a component it does not own — there is no "unknown directive" code path
> that reaches a template.

This makes the safe behavior the *default*: shipping a new designed block is adding a
registry entry + a `frag`; forgetting to register something fails closed (it renders as
text), never open.

### Attributes — escape-first and bounded

Attributes are a brace-delimited, space-separated list of `key=value` /
`key="quoted value"` pairs, parsed into a `map[string]string`. The list is **length-
bounded** (a cap on the brace span) so a giant attribute string can't be weaponized into
quadratic work or memory. Each registered component declares the keys it consumes;
**unrecognized keys are dropped**, and **every value is escape-first** — it rides the
`inline()` / `html.EscapeString` path before it enters a template as trusted HTML, identical
to the callout title and table cells. A class/structural attribute is therefore never
attacker-controlled: the component name (hence its CSS class) comes from the validated
registry key, and free-form values are escaped text only.

### Initial vocabulary

- **`:::card`** → `frag("dirCard", {Body})`: a token-styled raised card surface (the
  existing card recipe — `--panel`/`--bg` ladder, `--radius` ladder), body = the
  recursively-rendered child blocks. No attributes in v1.
- **`:::details{summary=…}`** → `frag("dirDetails", {Summary, Body})`: a native, JS-free
  collapsible `<details><summary>…</summary>…</details>`. `summary` rides the inline escape
  pass; absent → a default label ("Details"). `details`/`summary` join the tag whitelist;
  the open/close state is the browser's, so there is no new JS and no new stylesheet beyond
  the token-styled frame.

Both reuse only **guarded semantic tokens** (no new color pair), so the `css_tokens_test`
AA contrast guard and the both-theme a11y e2e scan stay green — the same constraint every
slice of this epic holds.

## Consequences

- Positive: the epic's open-ended goal — designed blocks the model can author — lands as
  *one* parser + a registry, so R6 and every future component is additive (a name + a
  `frag`), not new regex to fuzz. The allowlist makes "safe" the default and "forgot to
  register" fail closed. Zero dependencies, no JS, one CSS block per component, both themes
  AA.
- Negative / cost we accept: the model can only use the components we register (by design —
  the vocabulary is curated, not open); attribute syntax is a deliberately small subset
  (`key=value` / quoted), not full CommonMark-directive attribute grammar; nesting depth is
  capped. These are the price of "safe by construction" and are acceptable.
- Follow-ups: R6 citation cards (0082) can register as a directive (`:::cite`/inline `[n]`)
  or stay inline — its ADR/issue decides. Any new designed block enters through this
  registry; the fuzz/XSS corpus is widened once (for the directive fence + the two initial
  components) and then guards every later entry.
