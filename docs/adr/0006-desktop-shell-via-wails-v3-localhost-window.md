# 0006. Desktop shell via Wails v3 wrapping the local HTTP server

- Status: accepted
- Date: 2026-06-04
- Deciders: Horia
- Related: `cmd/my-orchestra-desktop`, `internal/bootstrap`, `internal/web`, `.github/workflows/desktop.yml`

## Context

my-orchestra shipped only as a CLI that serves a server-rendered htmx + SSE web UI
on a fixed loopback port; users opened it in a browser. We want a
double-clickable **desktop app** that opens the same UI in a native window.

The app is already a complete web app behind one clean seam:
`web.New(Options).Handler()` returns an `http.Handler`, and the UI talks to Go
purely over HTTP/SSE — there is **no Go↔JS IPC**. So the desktop shell is a thin
edge over the existing seam, not a new UI.

## Considered options

- **Minimal webview wrapper** (`go-webview2` + `webview_go`) — tiny, stable, pure
  "open a window at a URL". But it gives no packaging story (installers, app
  bundles, menus, tray) — we'd build all of that ourselves.
- **Wails v2 (stable)** — mature, but its API is built around embedded assets +
  Go/JS bindings we don't need; more ceremony for a localhost window.
- **Wails v3 (alpha)** — same packaging benefits, modern API, a `WebviewWindow`
  can load an external URL directly, and an `http.Handler` can be plugged in as
  the AssetServer if ever needed.

## Decision

We chose **Wails v3**, used in a deliberately minimal way:

- A new, build-tagged `cmd/my-orchestra-desktop` (tag `desktop`) calls the shared
  `internal/bootstrap.Build` (same assembly the web binary uses), then
  `bootstrap.ServeLocal` to run that `http.Handler` on an **ephemeral loopback
  port**, and points a Wails `WebviewWindow` at `http://127.0.0.1:<port>/`.
- Loading an **external loopback URL** (not Wails' `wails://` asset protocol)
  means SSE streams natively in the OS webview (WebView2 / WKWebView /
  WebKitGTK), exactly as in a browser — sidestepping known SSE-buffering issues
  with the custom protocol. The Wails AssetServer is given a 404 handler and is
  never hit.
- We do **not** use Wails' Go bindings/IPC — only its window + lifecycle
  (`OnShutdown`). Teardown is idempotent (`sync.Once`) so the defer and
  `OnShutdown` can't double-close the client. A `-serve` flag runs headless (no
  window) for CI smoke and Playwright.

## Consequences

- Positive: the desktop UI is **byte-identical** to the browser UI over the same
  seam, so the entire existing test pyramid (unit/seam, HTTP contract, Playwright
  e2e/api/a11y/ux/perf) covers it unchanged. New surface is tiny:
  `bootstrap.ServeLocal` (unit-tested) + thin window wiring (CI boot smoke).
- Negative / cost we accept:
  - **CGO + native runners.** The webview needs CGO and the platform webview
    toolchain, so the desktop binary cannot ride the pure-Go `CGO_ENABLED=0`
    6-target cross-compile in `release.yml`. It builds on a native matrix
    (`.github/workflows/desktop.yml`: ubuntu/macos/windows; Linux needs
    the GTK4 stack: `libgtk-4-dev` + `libwebkitgtk-6.0-dev` + `libsoup-3.0-dev`).
    The Wails import is isolated behind
    the `desktop` build tag so the pure-Go web binary and the default
    `go build/test ./...` never compile CGO.
  - **Go 1.25.** Wails v3 alpha requires Go ≥ 1.25, bumping the module's `go`
    directive (and the workflows' `go-version`) from 1.24 → 1.25 module-wide.
  - **Alpha dependency.** Pinned to `v3.0.0-alpha.98`; tracked in TECH_DEBT to
    revisit on a stable release.
- Follow-ups (TECH_DEBT): real installers (.dmg/.msi/.deb via `wails3 package`);
  revisit the alpha pin; per-OS manual verification of webview-specific behavior
  (SSE, cookies on the loopback origin, focus, native-close → graceful shutdown).
