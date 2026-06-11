# 0046. Callout blocks — GitHub-alert syntax, in-house, mapped to semantic tokens

- Status: accepted
- Date: 2026-06-11
- Deciders: Horia
- Related: issue 0078 (epic 0076), [ADR-0045](0045-block-ast-seam-for-markdown-rendering.md), [ADR-0001](0001-render-markdown-server-side-for-committed-agent-turns.md), [ADR-0025](0025-semantic-color-token-tier.md), `internal/web/markdown.go`

## Context

R2 (the first rich block on the ADR-0045 seam) renders GitHub-alert blockquotes —
`> [!NOTE|TIP|IMPORTANT|WARNING|CAUTION]` — as designed callout components instead of
plain blockquotes. The model already emits this syntax natively, so it is the highest
visual ROI per hour. Three decisions had to be settled: the **syntax** to recognize, the
**color/token mapping** (five GitHub kinds vs. our semantic token set), and how the block
upholds the escape-first invariant while emitting richer markup than any existing block.

## Considered options

- **Adopt a callout library / Goldmark extension** — rejected for the same reason as the
  whole epic (ADR-0045, REGRESSIONS #3): dependencies and a foreign rendering path the
  escape-first whitelist would have to be re-established around.
- **A bespoke `:::callout` grammar** — rejected: the model does not emit it, and the
  container-directive grammar is R5's (0081) job with its own allowlist ADR.
- **GitHub-alert blockquote syntax, recognized in-house on the block-AST seam** — a
  blockquote whose first line is `[!KIND]` of a known kind becomes a `calloutBlock`;
  anything else stays a plain `quoteBlock`.

## Decision

We recognize the **GitHub-alert syntax** in-house. In `parseLines`, a blockquote whose
first inner line matches `[!KIND]` (case-insensitive) with `KIND` in the fixed
`calloutKinds` set is promoted to a `calloutBlock{kind, title, children}`; the remaining
lines are the recursively-parsed body, and an optional trailing string on the marker line
overrides the default per-kind label. An **unknown kind degrades to a normal blockquote**,
marker text intact — the safe, lossless fallback.

Each kind maps to **one guarded semantic token** (ADR-0025), surfaced as a soft fill plus
the saturated tone for the rail, glyph, and label:

| kind | role | token |
|------|------|-------|
| note | info | `--accent2` |
| tip | success | `--good` |
| important | brand emphasis | `--accent` |
| warning | warn | `--warn` |
| caution | danger | `--bad` |

`important` takes the brand `--accent` (we have no distinct purple); the other four are the
info/success/warn/danger roles named in the charter. Every kind also carries a **glyph and
a text label**, so the kind is legible without color — color is never the sole signal
(WCAG). All five tokens are already covered by the `css_tokens_test` AA contrast guard in
both themes.

The block renders through **one `frag("callout", …)` template**, joining the
data→template→token-styled-HTML pattern ADR-0045 opened. The escape-first invariant holds
unchanged: `kind` comes from the validated set (so the `callout-<kind>` class is never
attacker-controlled), the title rides the `inline()` pass, and the body is the
already-escaped output of the child block renderers. The fuzz/whitelist corpus is widened
to admit the callout's `div`/`span` — the only structural tags the new template adds.

## Consequences

- Positive: the first proof of the seam end-to-end; the model's native alert syntax renders
  as a designed component with zero new dependencies, one CSS block, and one template; the
  token mapping reuses the AA-guarded semantic tier, so both themes pass the axe.
- Negative / cost we accept: a fixed five-kind vocabulary (extending it means editing
  `calloutKinds` + the CSS map); `important` borrows the brand accent rather than a bespoke
  hue.
- Follow-ups: R3 designed code blocks (0079) and R4 tables (0080) add their own blocks on
  the same seam; R5 container directives (0081) bring the `:::` grammar + allowlist ADR.
