# ⎈ my-orchestra

A **cost-aware coding TUI** that fuses the GitHub **Copilot CLI** and **Claude CLI**
experiences. It drives the official [GitHub Copilot Go SDK](https://github.com/github/copilot-sdk),
composes context with the **my-ctx forge**, and meters your **AI-Credit** spend in
real time so a coding session never surprises you on the bill.

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

- **Fusion TUI** — Copilot CLI's tool-centric chrome + Claude CLI's conversational feel,
  built on [Bubble Tea](https://github.com/charmbracelet/bubbletea).
- **Live credit telemetry** — per-model token & credit breakdown vs. a monthly budget,
  plus GitHub-authoritative AIU cost.
- **my-ctx forge** — toggle **skills**, **instructions**, and **agents** that compile
  into the session's system message; manage **MCP servers**.
- **Config & Settings pages** — model, reasoning effort, theme, streaming, auto-approve,
  budget, per-model price overrides, and key bindings.
- **Offline mock mode** — explore every page with no CLI or token installed.

## Pages

| Page | What it does |
|------|--------------|
| Chat | Streaming agent conversation, tool indicators, live credit footer |
| Telemetry | Credits & token breakdown per model vs. budget; authoritative AIU |
| Skills | Toggle reusable prompt skills into context |
| Instructions | Toggle system-message rules (priority-ordered) |
| Agents | Pick the active agent persona (model + effort + pinned skills) |
| Settings | View effective settings |
| Config | Key bindings & paths |
| Help | Keybindings and page guide |

## Install & run

```bash
# build
go build -o bin/my-orchestra ./cmd/my-orchestra

# seed a starter forge + config
./bin/my-orchestra --seed

# run (drives a real agent when the copilot CLI + token are available)
GITHUB_TOKEN=*** ./bin/my-orchestra
```

Requirements:
- **Go 1.24+** to build.
- For a live agent: the [`copilot` CLI](https://github.com/features/copilot/cli) on your
  `PATH` and a GitHub token. Without them, my-orchestra launches in **offline mock mode**.

Configuration lives in `~/.my-orchestra/` (override with `--config-dir` or
`MY_ORCHESTRA_HOME`):

```
~/.my-orchestra/
  config.json        # app settings + key bindings
  forge/forge.json   # skills, instructions, agents, MCP servers
```

## Keys

| Key | Action |
|-----|--------|
| `tab` / `shift+tab` | cycle pages |
| `enter` | send prompt (Chat) / toggle (lists) |
| `ctrl+j` | newline in the composer |
| `esc` | abort the current turn |
| `↑` / `↓` (`k`/`j`) | move selection on list pages |
| `a` / `e` / `d` | add / edit / delete on Skills·Instructions·Agents |
| `y` / `n` | approve / reject a tool-permission prompt |
| `ctrl+c` | quit |

When **auto-approve tools** is off (the default), the agent's shell/file/tool
actions surface an inline **`⚠ allow … ? [y]es / [n]o`** prompt in the Chat page;
requests queue if several arrive.

### Slash commands

Typed in the chat composer (never sent to the agent):

| Command | Effect |
|---------|--------|
| `/help` `/clear` `/cost` `/skills` `/agents` `/settings` | navigate / clear transcript |
| `/model <name>` | switch model and restart the session |
| `/agent <id>` | switch agent persona and restart the session |
| `/attach <path>` | attach a file/image to the next prompt |
| `/resume [id]` | resume the last (or a specific) session |

Launch with `--resume` to reopen the most recent session on startup.

## Development

```bash
make test       # race + coverage
make lint       # gofmt + vet (+ golangci-lint if installed)
make fuzz       # short fuzz of the pricing engine
make build      # local binary
make run        # build + run
```

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the design, the
`copilot.Client` seam, and how the forge compiles a session.

## Layout

```
cmd/my-orchestra     entrypoint & wiring
internal/tui         Bubble Tea model, views, key handling
internal/copilot     Client interface · SDKClient (Go SDK) · MockClient
internal/ctxforge    skills · instructions · agents · MCP → SessionSpec
internal/telemetry   price book · Meter · budget · AIU
internal/config      settings · key bindings (JSON)
docs/                GitHub Pages site + architecture
```

## License

See [LICENSE](LICENSE).
