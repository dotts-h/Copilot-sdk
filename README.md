# ⎈ my-orchestra

A **cost-aware coding web app** that fuses the GitHub **Copilot CLI** and **Claude CLI**
experiences. It drives the official [GitHub Copilot Go SDK](https://github.com/github/copilot-sdk),
composes context with the **my-ctx forge**, and meters your **AI-Credit** spend in
real time so a coding session never surprises you on the bill. The frontend is a
**server-rendered htmx UI** that streams the agent over Server-Sent Events.

> Built test-first, hardened with race/fuzz/concurrency tests, shipped with
> CI/CD and a GitHub Pages architecture site.

[**→ Architecture & internals (GitHub Pages)**](https://dotts-h.github.io/Copilot-sdk/) ·
[Deep dive](docs/ARCHITECTURE.md)

---

## Why

GitHub Copilot moved to **usage-based billing** on 2026-06-01: spend is metered on
tokens (input, cached, output, reasoning) and converted to **AI Credits** at a fixed
`1 credit = $0.01`. Agentic workflows can get expensive fast. my-orchestra puts the
meter front-and-center: every turn shows its running credit cost against your budget,
and the Telemetry page breaks it down per model — alongside GitHub's own authoritative
cost reported by the runtime.

## Features

- **Streaming htmx web UI** — Copilot CLI's tool-centric chrome + Claude CLI's
  conversational feel, server-rendered over SSE with zero JS build chain (htmx +
  htmx-ext-sse vendored locally).
- **Live credit telemetry** — per-model token & credit breakdown vs. a monthly budget,
  plus GitHub-authoritative AIU cost.
- **my-ctx forge** — toggle **skills** and **instructions** and pick the active
  **agent** persona; they compile into the session's system message.
- **Inline tool approvals** — when auto-approve is off, the agent's shell/file/tool
  actions surface an inline approve/reject control in the chat stream.
- **Offline mock mode** — explore every page with no CLI or token installed (`-demo`).

## Pages

| Page | What it does |
|------|--------------|
| Chat | Streaming conversation with a Copilot-style tool timeline (args, live progress, results/diffs), reasoning shown as a separate "thinking" block, and a live credit footer |
| Telemetry | Credits & token breakdown per model vs. budget; authoritative AIU |
| Skills | Toggle reusable prompt skills into context |
| Instructions | Toggle system-message rules (priority-ordered) |
| Agents | Pick the active agent persona (model + effort + pinned skills) |
| Settings | View effective settings |

Pages are `hx-get` partials swapped into the shell; the chat SSE stream stays
open across navigation.

## Install & run

```bash
# build
go build -o bin/my-orchestra ./cmd/my-orchestra

# seed a starter forge + config
./bin/my-orchestra --seed

# run (drives a real agent using your logged-in copilot CLI session)
# then open the printed URL (default http://127.0.0.1:8765)
./bin/my-orchestra

# explore the UI offline with a scripted mock — no copilot CLI needed
./bin/my-orchestra -demo
```

Flags: `-addr` sets the listen address (default `127.0.0.1:8765`), `-demo` drives
a scripted mock, `-seed` writes a starter forge then exits, `-config-dir` picks
the config directory.

By default my-orchestra authenticates with the **already-logged-in `copilot` CLI
session** — run `copilot` once to log in and you're set; no token to manage. To
override with an explicit token instead, point `githubTokenEnv` in `config.json`
at an env var holding the token (e.g. `"githubTokenEnv": "GITHUB_TOKEN"`).

Requirements:
- **Go 1.24+** to build.
- For a live agent: the [`copilot` CLI](https://github.com/features/copilot/cli) on your
  `PATH`, logged in (`copilot`). Without it, my-orchestra launches in **offline mock mode**.

Configuration lives in `~/.my-orchestra/` (override with `--config-dir` or
`MY_ORCHESTRA_HOME`):

```
~/.my-orchestra/
  config.json        # app settings + key bindings
  forge/forge.json   # skills, instructions, agents, MCP servers
```

## Using the UI

The nav bar switches pages (chat, telemetry, skills, instructions, agents,
settings); the chat stream stays open in the background as you navigate. Type a
prompt and **Send** to start a turn — assistant tokens, reasoning, and tool calls
stream in live. On the Skills and Instructions pages, click a row to toggle it
into context; on Agents, click to make a persona active.

When **auto-approve tools** is off (the default), the agent's shell/file/tool
actions surface an inline **approve / reject** control in the chat stream;
requests queue if several arrive.

> Add/edit forms for forge entities, a slash-command menu, in-place model/agent
> switching, and an abort button are tracked as follow-ups (see
> [docs/WEB_UI_PLAN.md](docs/WEB_UI_PLAN.md)); toggle/select/delete are wired today.

## Development

```bash
make test       # race + coverage
make bench      # web render/reducer benchmarks + concurrent load
make lint       # gofmt + vet (+ golangci-lint if installed)
make fuzz       # short fuzz of the pricing engine
make build      # local binary
make run        # build + run

# Browser suite (e2e · api · a11y · ux · perf) against the offline demo server
make e2e-install   # one-time: npm ci + Playwright Chromium
make e2e           # build + drive Chromium via Playwright
```

The browser suite lives in [`e2e/`](e2e/README.md) and runs against
`my-orchestra -demo`; it covers what the Go tests can't (real htmx swaps, the SSE
transport, keyboard/focus behaviour, responsive layout, and WCAG conformance).

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the design, the
`copilot.Client` seam, and how the forge compiles a session.

## Layout

```
cmd/my-orchestra     entrypoint & wiring
internal/web         net/http server · html/template partials · SSE hub · vendored htmx
internal/convo       UI-agnostic transcript model (Turn · ToolView · State reducer)
internal/copilot     Client interface · SDKClient (Go SDK) · MockClient
internal/ctxforge    skills · instructions · agents · MCP → SessionSpec
internal/telemetry   price book · Meter · budget · AIU
internal/config      settings · key bindings (JSON)
e2e/                 Playwright browser suite (e2e · api · a11y · ux · perf)
docs/                GitHub Pages site + architecture
```

## License

Business Source License 1.1 (BSL) — source-available. Free for personal and
internal individual use, including production; you may not offer it to third
parties as a commercial product or hosted service. Each version converts to
Apache License 2.0 on its Change Date (2030-06-04). See [LICENSE](LICENSE).
