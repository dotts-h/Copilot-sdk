# my-orchestra → Web (htmx) migration plan

Status: **design / not yet started.** This doc is the kickoff brief for the rewrite
of the frontend from a Bubble Tea TUI to a server-rendered htmx web app. The
non-UI core (`internal/copilot`, `internal/ctxforge`, `internal/telemetry`,
`internal/config`) is **reused unchanged** — only `internal/tui` is replaced.

## Why htmx fits
The `copilot.Client` seam already emits normalized `Event`s on a channel. That
channel maps directly onto **Server-Sent Events (SSE)**: one long-lived GET that
ranges over `client.Events()` and writes HTML fragments. Client→server actions
(send prompt, answer permission, toggle skill) are ordinary `hx-post`/`hx-get`.
SSE is unidirectional, lightweight, auto-reconnecting, and the right tool for
one-way token streaming (WebSockets only needed for bidirectional). HTTP/2
removes the old 6-connection limit.

## UX principles (from research)
1. **Stream everything; optimize time-to-first-token.** Tokens appear live.
2. **Transparency beats black-box.** Show every tool call — name, args, live
   progress, result/diff — as a timeline entry. This is the trust surface.
3. **Reasoning is visually distinct** from the answer (separate dim "thinking"
   block), collapsible.
4. **Approval/permission is a first-class inline control**, not a modal afterthought.
5. **Diffs get a review lane** — file writes/edits render as a diff with the
   approve/reject affordance attached.
6. **Event-driven, not request/response** — the agent streams updates; the UI
   reflects progress continuously (AG-UI style).
7. **Cost stays ambient** — the credit/budget meter is always visible (footer),
   updated live on each usage event.
8. **Background/long-running work** surfaces as an activity indicator, not buried
   in chat (sets up workstream 2).

## Recommended stack (dependency-light, no JS build chain)
- **Server:** Go stdlib `net/http`. One process, single local user assumed.
- **Templates:** stdlib `html/template` (zero codegen). `templ` is optional later
  for compile-time-typed templates; not needed to start.
- **htmx:** `htmx.org@2.0.x` + `htmx-ext-sse@2.2.x`, vendored as static files under
  `internal/web/static/` (don't depend on a CDN for a local tool).
- **SSE:** a tiny custom hub (no library) — a goroutine per connection ranging the
  `Client.Events()` channel; set `Content-Type: text/event-stream`,
  `Cache-Control: no-cache`, `X-Accel-Buffering: no`, and `Flush()` after each event.
- **CSS:** a small handcrafted stylesheet mirroring the existing `Palette`
  (terracotta accent `#d98c5f`, copilot blue `#6ea8fe`, slate chrome). No Tailwind
  unless desired.

## Architecture
```
cmd/my-orchestra        entrypoint: build core, start http.Server instead of TUI
internal/convo (NEW)    UI-agnostic transcript model lifted from tui/chat.go:
                        Turn, ToolView, chatState reducer (appendDelta,
                        appendReasoning, toolStart/Progress/End, finish). Pure +
                        unit-tested; both a future TUI and the web render from it.
internal/web   (NEW)    http handlers, SSE hub, html/template partials, static assets
internal/copilot        UNCHANGED (Client seam, SDKClient, MockClient, permBridge)
internal/ctxforge       UNCHANGED
internal/telemetry      UNCHANGED
internal/config         UNCHANGED
internal/tui            DELETED (hard cut — no dual frontend)
```

**Decision (2026-06-03): replace the TUI outright.** No dual-frontend, no
keeping it as a fallback. The migration happens on a branch; `internal/tui` is
deleted within the same effort and the app ships with the web UI as its only
frontend. The Bubble Tea dependencies (`bubbletea`, `bubbles`, `lipgloss`) are
dropped from `go.mod` at the end of the cut. To avoid a non-functional `main`
mid-migration, do the work on a feature branch and land it once the web UI
covers the Chat path end-to-end (steps 1–4); secondary pages (step 5) can follow
on the same branch before merge.
Per-connection server state = essentially today's `Model` minus rendering: the
active `sessionID`, the `convo` transcript, the permission queue, pending
attachments. Held in a `web.Session` struct keyed by a cookie/session id (single
user → one in-memory instance is fine to start).

## The SSE event → fragment contract (the heart of it)
Normalized `copilot.Event` → SSE message. Append-style for streams, OOB swap for
in-place updates (tool cards, footer).

| Event | SSE name | DOM effect |
|-------|----------|-----------|
| EvMessageDelta | `msg-delta` | `<span>` appended to `#cur-msg` (`hx-swap=beforeend`); wrap each token in a span to preserve whitespace |
| EvReasoningDelta / EvReasoning | `rea-delta` | append to `#cur-reasoning` (separate, dim, collapsible) |
| EvMessage | `msg-done` | finalize current bubble (`outerHTML` swap to committed turn), reset `#cur-msg` |
| EvToolStart | `tool` | OOB append a `<div id="tool-{id}">` card (glyph ●, name, args) to `#timeline` |
| EvToolProgress | `tool` | OOB swap `#tool-{id}` with updated card (progress line) |
| EvToolEnd | `tool` | OOB swap `#tool-{id}` → ✓/✗ + bounded result/diff |
| EvUsage | `cost` | OOB swap `#cost-footer` (credits, budget bar) |
| EvPermission | `perm` | OOB append inline `[approve]/[reject]` form targeting `/perm/{id}` |
| EvIdle | `turn-end` | clear spinner, finalize any open buffers |
| EvError | `error` | OOB append an error banner |

In-place updates use htmx OOB: send `data: <div id="tool-x" hx-swap-oob="true">…</div>`.

### Routes (client→server)
```
GET  /                 full page (current page shell + timeline + composer + footer)
GET  /events           SSE stream (ranges client.Events(); never returns until close)
POST /send             enqueue/send prompt (+attachments); appends user bubble, returns partial
POST /perm/{id}        approve/reject a permission (?approve=1|0) → resolves permBridge
POST /abort            abort the in-flight turn
GET  /page/{name}      hx-get partial for nav (chat|telemetry|skills|instructions|agents|settings)
POST /skills/{id}/toggle, /agents/{id}/select, forge CRUD …
GET  /static/*         htmx + css + js assets
```
Decouple send (POST) from receive (SSE): POST /send only echoes the user bubble;
the assistant's streamed reply arrives on the always-open `/events` channel. This
respects SSE unidirectionality and makes queued input (workstream 2) trivial —
the composer is never disabled.

## Phased next steps
Do all of this on a feature branch (e.g. `feat/web-htmx`); `main` stays on the
TUI until the branch lands as the hard cut.
1. **Scaffold + walking skeleton.** `internal/web` server, base layout template,
   vendored htmx, `/events` SSE hub driven by `MockClient`, a hardcoded streaming
   message end-to-end. Goal: see tokens stream in a browser. *(start a fresh session)*
2. **Lift `internal/convo`.** Move `chatState`/`Turn`/`ToolView` out of `tui`,
   keep its tests; render it from templates. Wire `/send` + the message/reasoning
   SSE contract.
3. **Tool timeline + reasoning + cost footer** as fragments (port workstream-1 logic).
4. **Permissions** inline form + permBridge over `/perm/{id}`. — Chat path now at
   parity; the branch is mergeable from here.
5. **Secondary pages** (telemetry, skills/instructions/agents CRUD, settings) via
   hx-get partials.
6. **Hard cut.** Delete `internal/tui`; repoint `cmd/my-orchestra` at the web
   server; drop `bubbletea`/`bubbles`/`lipgloss` from `go.mod` (`go mod tidy`);
   update README/ARCHITECTURE/CI; remove the `--resume`/TUI-specific wiring that
   no longer applies. Merge the branch.
7. **Then resume product workstreams 2 & 3** (background behavior, control
   surfaces) — now implemented once, in HTML.

## Gotchas
- Disable proxy/server buffering or streaming stalls (`X-Accel-Buffering: no`, flush).
- Wrap streamed tokens in `<span>` or whitespace collapses.
- HTML-escape model output (`html/template` auto-escapes; ensure tokens go through
  it, not raw writes).
- Coalesce/debounce very high-frequency deltas to avoid thousands of tiny swaps.
- permBridge stays as-is; just resolve it from the POST handler instead of a keypress.
```

Sources: htmx SSE extension docs (htmx.org/extensions/sse), Peter Stuifzand
"Streaming AI with htmx", AI-chat UX surveys (patterns.dev, AG-UI, thefrontkit).
