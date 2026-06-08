# my-orchestra — browser test suite (Playwright)

End-to-end, API, accessibility, UX, and performance tests for the htmx + SSE web
UI. They run against the **offline demo server** (`my-orchestra -demo`), which
drives a scripted `MockClient` so every surface — streaming reply, tool
timeline, inline permission/ask/plan/elicit forms, the sub-agent strip, and the
cost meter — renders deterministically with no Copilot runtime or network.

This complements the in-process Go tests (`internal/...`), which cover the pure
core, the reducer, and the HTTP handlers against the mock. The browser suite
covers what those cannot: real htmx swaps, the real SSE transport, focus and
keyboard behaviour, responsive layout, and WCAG conformance.

## Layout

| File | Layer | What it asserts |
|------|-------|-----------------|
| `tests/e2e.spec.ts`  | **E2E**    | Navigation, a streamed turn (reasoning/tool/answer), inline ask/plan/elicit forms, abort + type-ahead queueing, forge toggle/select, model switch, slash commands |
| `tests/api.spec.ts`  | **API**    | HTTP contract over a real socket: page/asset status + content-types, hardened session cookie, escaped reflection, `/send` 204/echo, bridge-route tolerance, SSE greeting |
| `tests/a11y.spec.ts` | **A11y**   | `axe-core` WCAG 2.1 A/AA scans of every page + the chat page after a turn |
| `tests/ux.spec.ts`   | **UX**     | Autofocus/refocus, Enter-to-send, keyboard nav, no console/page errors, no horizontal overflow (desktop/tablet/mobile), confirm-guarded deletes, reduced-motion |
| `tests/perf.spec.ts` | **Perf**   | Navigation-timing budget, htmx swap latency, SSE first-byte latency, send-ack latency, DOM-growth sanity |
| `tests/helpers.ts`   | —          | Selector map, page table, and `gotoApp` / `navTo` / `send` drivers |

## Running

Requires Node 18+, and `go` on `PATH` (the Playwright `webServer` builds the
binary). First time only, install deps + the browser:

```bash
cd e2e
npm install
npm run install:browsers      # playwright install --with-deps chromium

npm test                      # the whole suite
npm run test:e2e              # one layer
npm run test:a11y
npm run report                # open the last HTML report
```

The config builds `../bin/my-orchestra` and launches `-demo` on `127.0.0.1:8799`
before the first test, and tears it down after. Set `MO_PORT` to relocate it.

> **Single shared session.** The demo routes events through one in-memory
> session (the mock's events carry no session id, so they only route when exactly
> one session exists). Tests therefore run with `workers: 1` and assert on
> *committed* transcript turns / resolution notes rather than on absolute form
> counts, since fixed-id demo forms accumulate across turns.

## Findings surfaced by this suite

Writing these tests turned up real defects the server-side Go tests could not
see. All three were fixed.

1. **Fixed — composer wiped input on every keystroke.** The composer form's
   `hx-on::after-request` (meant to reset after the send POST) also caught the
   *bubbled* `htmx:afterRequest` from the autocomplete `GET /commands` that fires
   on every keyup, so it ran `this.reset()` mid-typing. `page.fill()` hid this
   (it sets the value without emitting keyup); real char-by-char typing exposed
   it. Fix: guard the handler to the form's own request
   (`if (event.target !== this) return;`). Locked by
   `ux.spec.ts › composer preserves input while typing character by character`.

2. **Fixed — topbar overflowed at tablet width.** The brand + 8 nav links + cost
   meter exceeded ~768px with nothing wrapping, pushing the cost footer ~76px
   off-screen. Fix: `flex-wrap: wrap` on `.topbar`/`.nav`. Locked by
   `ux.spec.ts › no horizontal scroll …`.

3. **Fixed — destructive-control contrast (ADR-0025).** The abort / reject /
   decline controls rendered on the brand red (`--bad #f85149`) at ~3.5–4.2:1,
   just under WCAG AA's 4.5:1, and were carried as a documented allowlist
   (`KNOWN_CONTRAST_SELECTORS`). The UI/UX refresh (epic 0045) retuned the palette
   into a semantic light/dark token system: every semantic color now flips bright
   (dark theme) / deep (light theme) so it clears AA as text on the page
   background, and solid fills take one companion `--on-bright` text token — so the
   destructive controls clear AA in **both** themes. The allowlist and its baseline
   guard test are removed; `a11y.spec.ts` now scans every page in **both** themes
   with **no** exceptions.

Also note: navigating away from the chat page while a turn streams logs benign
`htmx-ext-sse` errors (it swaps into chat-only targets like `#status`/`#ctx`
that only exist inside `#main` on the chat page). It is non-fatal — the chat
re-renders on return — and is filtered as known noise in
`ux.spec.ts › navigating across all pages …`. Worth a future fix (scope the SSE
swap targets to the chat partial).

## CI

`.github/workflows/e2e.yml` runs the suite on push/PR: it sets up Go + Node,
caches npm, installs Chromium with OS deps, and uploads the HTML report and
traces on failure.
