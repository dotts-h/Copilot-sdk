# CONVENTIONS.md — the project constitution

> The single living rulebook. Other skills (TDD, SDET, quality, contracts) read this
> before they act. State **what to do now**; the **why** lives in the linked ADR.
> Machine facts (commands, paths, thresholds) are exact and copy-pasteable.

Module: `github.com/dotts-h/copilot-sdk` · app *my-orchestra*.

## Workflow

- Branch from `main`; never commit directly to `main`.
- Write a failing test first, then the smallest code to pass it. Keep changes small.
- Before pushing, run the gates locally: `make lint && make test`.
- Open a PR; **CI must be green** (lint, race tests + coverage floor, fuzz, build matrix) before merge.
- Merge with `--no-ff`, push `origin/main`, then delete the local branch.
- **Fold supporting docs into the feature branch** — ADRs, REGRESSIONS, TECH_DEBT, and
  CONTRACTS updates ship *in the same PR* as the change that motivated them. Do **not**
  open a separate docs-only PR for them (it serializes an extra CI cycle and risks a merge
  conflict on the shared docs). — see [ADR-0004](adr/0004-fold-supporting-docs-into-the-feature-branch.md)
- **PR mechanics when `gh` is absent:** create/merge PRs via the GitHub REST API using a
  stored PAT (`python3` + `urllib`; `jq` is **not** installed). Poll check-runs for CI
  status. — see [ADR-0004](adr/0004-fold-supporting-docs-into-the-feature-branch.md)

## Architecture rules

> These invariants predate the ADR log (ADR-0001 is the first). They are load-bearing and
> currently **undocumented decisions** — flagged for `recording-decisions` to backfill.

- **No SDK imports outside the seam.** UI/web depend only on `copilot.Client`; new runtime
  behavior goes in `SDKClient`, new UI behavior is tested via `MockClient`. — *ADR needed (backfill)*
- **Domain logic stays pure.** `telemetry`, `ctxforge`, and `config` are dependency-free
  and fully unit-tested. — *ADR needed (backfill)*
- **Determinism.** `Forge.Compile` and pricing must be deterministic; add a test if you
  touch ordering or rates. — *ADR needed (backfill)*
- Markdown for committed agent turns is rendered **server-side**, not in the browser. — see [ADR-0001](adr/0001-render-markdown-server-side-for-committed-agent-turns.md)
- Session pick/start/continue is backed by **SDK session resume**, not a bespoke
  in-memory store. — see [ADR-0002](adr/0002-restore-sdk-session-resume-for-session-pick-start-continue.md)
- Agents are CLI-style: a built-in chat agent plus a per-agent tool allowlist. — see [ADR-0003](adr/0003-claude-cli-style-agents-built-in-chat-agent-and-per-agent-tool-allowlist.md)
- **The desktop shell is CGO + native-runner only.** Keep the Wails import inside
  `cmd/my-orchestra-desktop` (build tag `desktop`); never import it from
  `cmd/my-orchestra` or shared packages, so the pure-Go web binary and the default
  `go build/test ./...` stay CGO-free. Both binaries share `internal/bootstrap`.
  — see [ADR-0006](adr/0006-desktop-shell-via-wails-v3-localhost-window.md)

## Testing

- Layers: unit/seam (table-driven, `MockClient` at the UI seam) · contract (see
  [CONTRACTS.md](CONTRACTS.md)) · browser e2e (Playwright). For *how*, use the
  `practicing-tdd`, `hardening-tests`, and `authoring-tests` skills.
- **Every fixed bug gets a guard test** and an entry in [REGRESSIONS.md](REGRESSIONS.md).
- Every SDK event → normalized-event mapping is tested.
- Pricing has a fuzz smoke target; concurrency is exercised under `-race`.

## Quality gates

Exact commands CI enforces (`.github/workflows/ci.yml`, `Makefile`):

- **Lint:** `make lint` — gofmt + `go vet ./...` + golangci-lint v2 (`.golangci.yml`).
- **Test:** `make test` — `go test ./... -race -count=1 -timeout 180s` with coverage.
- **Coverage floor: 65%** (`go tool cover -func`; CI fails below 65%).
- **Fuzz:** `make fuzz` — smoke on pricing.
- **E2E:** `make e2e` (Playwright; `make e2e-install` first); CI runs `npx playwright test`.
- **Build matrix:** `make build` must pass across the release matrix (pure-Go, `CGO_ENABLED=0`).
- **Desktop:** `make desktop` (CGO, build tag `desktop`) builds the Wails shell; CI
  (`desktop.yml`) builds it on native runners and runs a boot smoke under xvfb.

## Persistence & data

- Config/forge writes are **atomic**: temp-file + rename, and **validate before save**. — *ADR needed (backfill)*
- Schema changes to persisted state must stay backward-readable or ship a migration.

## Environment facts

- **Go toolchain is not on the default PATH:**
  `export PATH=$PATH:/home/ori913/go-install/go/bin`
- **Go 1.25+** required (the Wails v3 desktop dep raised the module floor from 1.24).
- `jq` is **not** installed; use `python3` for JSON.
- Run the app: `make run`. Benchmarks: `make bench`. Tidy modules: `make tidy`.
- Desktop build deps (Linux): `pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev`; then
  `make desktop` / `make run-desktop`. macOS/Windows ship the webview.

## Naming & style

- Branches: `feat/…`, `docs/…`, `fix/…` (kebab, scope-prefixed).
- Skills use gerund slugs (`registering-contracts`); the docs they own live under `docs/`.
- Commit messages: imperative subject, conventional-commit prefix where it fits
  (`feat(web):`, `docs:`).
- Adding a TUI page: `Page` constant + `String()` in `internal/tui/model.go`; `view*` in
  `internal/tui/views.go` wired in `renderBody`; a `View` smoke test covers it.

---
*Bootstrapped 2026-06-04 from CONTRIBUTING.md, Makefile, `.github/workflows/`, and
docs/ARCHITECTURE.md. Rules marked “ADR needed” are load-bearing invariants awaiting a
backfilled decision record.*
