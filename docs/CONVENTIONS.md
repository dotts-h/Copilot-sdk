# CONVENTIONS.md — the project constitution

> The single living rulebook. Other skills (TDD, SDET, quality, contracts) read this
> before they act. State **what to do now**; the **why** lives in the linked ADR.
> Machine facts (commands, paths, thresholds) are exact and copy-pasteable.

Module: `github.com/dotts-h/copilot-sdk` · app *my-orchestra*.

## Workflow

- Branch from `main`; never commit directly to `main`.
- **Verify the base before branching.** This repo has a **tag named `main`** colliding with the
  `main` *branch*, and the sandbox git server can serve a **stale `origin/main`** on the first
  fetch. Both have silently put work on the wrong base (a `git switch -c` resolved the *tag*; a
  pre-foundation `origin/main` made already-merged files "vanish"). Before writing code:
  `git fetch origin && git switch main && git pull --ff-only`, then **assert the foundation is
  present** — confirm `git rev-parse origin/main` is the expected SHA and, when building on prior
  work, `git cat-file -e origin/main:<a-file-that-must-exist>`. A stale remote must fail loud, not
  silent. — see [RETROS 0002](RETROS/0002-v28-postooluse-governance-close.md)
- **Waiting on CI/remote without a foreground `sleep`** (which the harness blocks): use a
  background until-loop timer (`until [ $((SECONDS-start)) -ge N ]; do sleep 10; done`) to wake,
  then re-check status via the compact MCP calls (`pull_request_read get_status`/`get_check_runs`,
  not `list_workflow_runs` — its per-row repeated `repository` objects are mostly noise). — see
  [RETROS 0002](RETROS/0002-v28-postooluse-governance-close.md)
  - **Cap the wait, and trust the logs over a lagging status.** The `get_check_runs` *status*
    can report `in_progress` for many minutes **after** a job has actually finished (a stale
    GitHub status API), which has burned ~10 min waiting on an e2e job that completed in ~2.
    So bound the wait: a single workflow job (e2e) has a **~6-minute** wall ceiling (observed
    max ~5 + buffer); a whole PR's CI a **~10-minute** ceiling. Past the ceiling, do **not**
    keep re-polling the status — call `get_job_logs` on the job and read the truth: report
    uploaded + clean post-job cleanup with no failing step ⇒ it passed and the status is just
    lagging, proceed; genuinely mid-run or failed ⇒ act on that. Run **one** timer at a time and
    stop arming new ones once a ceiling is hit (stacked timers drain later as stale-notification
    noise). If a real failure or a true stall persists past the ceiling, escalate to the human
    with the diagnosis rather than waiting silently.
- Write a failing test first, then the smallest code to pass it. Keep changes small.
- Before pushing, run the gates locally: `make lint && make test`.
- **Self-review the diff before pushing:** run `/code-review` (always) and, for UI
  changes, `/verify` or `make run`/`make e2e` to exercise the behavior. Audits
  done *before* coding don't catch what the resulting diff introduces — this is
  cheap insurance, not optional. **Size the review to the diff:** default to
  `/code-review low` or `medium` (a Sonnet pass, fewer high-confidence findings);
  reserve high effort for large or correctness-critical diffs. A **bare**
  `/code-review` defaults to a **high-effort Opus fan-out** (7 finder angles, a
  verifier per candidate) — wasted spend on a small, guard-tested presentation
  diff, and easy to leave running into a session-limit with nothing returned.
  Lean on the artifacts: when a guard test already proves an invariant (e.g.
  `css_tokens_test.go` for the token/contrast contract), point the reviewer at
  what the test *can't* cover rather than re-deriving the structure. — see [RETROS 0001](RETROS/0001-quality-and-architecture-hardening.md)
- Open a PR; **CI must be green** (lint, race tests + coverage floor, fuzz, build matrix) before merge.
- Merge with `--no-ff`, push `origin/main`, then delete the local branch.
- **Fold supporting docs into the feature branch** — ADRs, REGRESSIONS, TECH_DEBT, and
  CONTRACTS updates ship *in the same PR* as the change that motivated them. Do **not**
  open a separate docs-only PR for them (it serializes an extra CI cycle and risks a merge
  conflict on the shared docs). — see [ADR-0004](adr/0004-fold-supporting-docs-into-the-feature-branch.md)
- **Recording the PR number is NOT an exception to the above.** You don't know the number
  until the PR exists — so open the PR, then push the doc-record commit (flip the issue to
  `closed`, add the `Resolution (shipped)` section, record the number on the issue/epic/INDEX)
  to the **same branch** *before* merging. A second "record PR #N" follow-up PR is the exact
  separate docs-only PR ADR-0004 forbids — it doubles CI for bookkeeping git already encodes.
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
- **Recipe conformance:** `make doctor` — lock-driven cookbook doctors over
  `.recipes/lock.json`. The catalog ships in the `cookbook@ori` plugin
  (`dotts-h/claude-skills`); in a Claude session it's at `$CLAUDE_PLUGIN_ROOT`,
  otherwise point `RECIPES_DIR` at a checkout of that plugin. Process-infra changes
  land in the `cookbook` plugin first, then flow here via `/update-recipes`
  (ADR-0054). Advisory locally, not a CI gate (CI has no plugin/catalog checkout).
- **Desktop:** `make desktop` (CGO, build tag `desktop`) builds the Wails shell; CI
  (`desktop.yml`) builds it on native runners and runs a boot smoke under xvfb.
- **CI runs once per change.** `ci.yml`/`e2e.yml` trigger on `pull_request` (the open PR)
  and `push: [main]` (merge) — **never** list a feature branch (`claude/**`) under `push`,
  which doubles every run. `pages.yml`/`desktop.yml` are `main`-only and path-filtered.
- **Docs-only changes skip the heavy pipeline.** `ci.yml`/`e2e.yml` carry
  `paths-ignore: ["docs/**", "**/*.md"]`, so a pure docs/ADR/issue edit doesn't spin up the
  6-cell build matrix, race tests, fuzz, or the browser suite. The skip is whole-workflow
  (safe here: a docs-only PR has merged with `desktop.yml`'s checks already skipped). It's
  sound because **no committed `.md` is a build/test input** — keep it that way: a test that
  reads a tracked markdown file would silently lose its gate (synthesize fixtures in a temp
  dir, as the instruction-import tests do). A PR touching code *and* docs runs in full
  (`paths-ignore` skips only when every changed path matches).
- **Workflow guard (self-enforcing):** `scripts/check-workflows.sh` fails if a workflow
  re-introduces a feature-branch `push` trigger (the double-run) or the release
  version-resolution bug (`${GITHUB_REF_NAME:-…}`, which tags a dispatched release after
  the branch). It runs in CI (the lint job) **and** in `make lint`, so these two regressions
  can't land. To also catch them locally in a Claude Code session (before push), opt in by
  adding a `PostToolUse` hook to `.claude/settings.json` that runs
  `scripts/hook-check-workflows.sh` (matcher `Edit|Write|MultiEdit`) — the agent can't edit
  that file (it enables an external plugin marketplace), so wire it by hand.

## Session playbook

The single home for the "carry-forward" session-optimization lessons (RETROS 0001/0002/0005/
0006 each appended one — consolidated here per RETROS 0006 so a new session reads **one** list,
not four overlapping ones). A retro adds a lesson by editing *this* list, and its own
checklist section carries only the **delta** (what changed) plus the one-line session-cost
figure (below). Frontend/style lessons live in *Naming & style*, not here.

**Discovery & reading**
- Map-first (CODEMAP), windowed reads; full file reads only when editing most of a file.
- Delegate discovery to Explore/sub-agents and mechanical work (e.g. test authoring) to a
  scoped sonnet agent on non-overlapping packages; keep the conclusion + line ranges, not
  file dumps.
- Scope audits (named files + concerns) and run them in parallel; don't "sweep everything"
  on a healthy codebase.
- Docs lookups: route (CLAUDE.md) → index (CODEMAP/CONTEXT/INDEX) → grep → windowed read;
  don't reach for retrieval infra the corpus doesn't need (ADR-0050).

**Remote sandbox**
- Verify the base before writing code — `origin/main` SHA + a sentinel file; never trust the
  first fetch.
- Prefer compact MCP endpoints; don't read `list_workflow_runs` to check one status.
- Wait via a background until-loop timer; never a foreground `sleep`.
- An unverifiable e2e change (no browser in-sandbox) must be a faithful copy of a proven
  spec, not a fresh invention.

**Quality loop**
- Package-scoped tests in the loop; `/code-review` (always) + full `make lint && make test`
  before push.
- When an error-path fix breaks green tests, suspect the harness encodes the bug — fix the
  harness, then guard the real failure path.
- Treat a review finding against the *motivating example* as a bug to fix at depth, not a
  limitation to document.

**Close-out**
- Close an epic completely: status + INDEX + children + the epic's **own body**; landing an
  ADR includes its `docs/adr/README.md` index row (treat like CODEMAP regen).
- For "should we adopt X infra?" write the ADR with revisit triggers — retro sections get
  re-asked, ADRs get cited.
- Keep the KICKOFF "READ FIRST" list tight (CODEMAP + the 5 core docs).

**Session-cost line (observability, in-repo, zero-dependency).** End each retro with one
sentence: *roughly what the session cost in tokens and where it went* (e.g. "~150K, dominated
by the three scoped audits"). This is the local, trust-boundary-free equivalent of the
third-party context-optimizer plugins (RETROS 0007): we hold our own dev loop to the same
spend-measurement standard the product holds the agent to — without installing anything that
runs with our process privileges.

## Releases & versioning

Versions are **SemVer** (`vMAJOR.MINOR.PATCH`), bumped per landed change:

- **Feature → minor** (`v0.1.0 → v0.2.0`). A shipped roadmap item / epic child is a feature.
- **Bug fix → patch** (`v0.2.0 → v0.2.1`).
- **Breaking change → major.** Pre-1.0, a breaking change may ride a **minor** (`0.x` is unstable
  by SemVer); call it out in the release notes either way.
- An epic that lands several feature children closes on a single **minor** bump (e.g. epic 0045 /
  roadmap v9 → `v0.2.0`), not one bump per child.

The release is **tag-driven**: pushing a `v*` tag (or a manual `workflow_dispatch` with the tag)
runs `.github/workflows/release.yml`, which cross-compiles the 6-target matrix and publishes a
GitHub Release with `checksums.txt` + auto-generated notes. **GitHub cost is negligible** — release
assets don't count against storage, and Actions minutes are free on public repos (a few minutes per
release on private). In a sandbox where a direct tag `push` is blocked, cut the tag with the
**`cut-release` skill** (it verifies the resolved version end-to-end so a misconfigured workflow
can't publish a mis-tagged release). Never hand-edit the version-resolution step — the workflow
guard (`scripts/check-workflows.sh`) fails on the `${GITHUB_REF_NAME:-…}` regression.

## Docs & comments — one fact, one home

The why lives in **exactly one** canonical place; everything else **points**, never copies.
The meta-layer (comments + docs) already rivals the code in size, so duplication is the
main drift risk (it is how stale rows/READMEs creep in).

- **ADR = the why.** A decision's rationale is single-sourced in its ADR. A code doc-comment
  states the *contract* tersely (invariant, ordering, error mode) and cites the ADR
  (`— ADR-00NN`); it does not re-narrate the rationale.
- **CONTRACTS = the index of seams**, not a second prose copy of each comment. **CODEMAP is
  generated** (`make codemap`) — never hand-edit.
- **CONTEXT.md = the domain glossary** (ubiquitous language: forge, lane, run, share, meter,
  credit, ledger, reconcile, gate…). Define a term **once** there; comments/ADRs use it
  without re-defining. — see [CONTEXT.md](CONTEXT.md)
- **Comments earn their place** by capturing what the code can't (intent, invariants, the
  non-obvious). Don't restate the code; don't copy a paragraph that already lives in an ADR.

## Persistence & data

- Config/forge writes are **atomic**: temp-file + rename, and **validate before save**. — *ADR needed (backfill)*
- Schema changes to persisted state must stay backward-readable or ship a migration.

## Environment facts

- **Go toolchain is not on the default PATH:**
  `export PATH=$PATH:/home/ori913/go-install/go/bin`
- **Go 1.25+** required (the Wails v3 desktop dep raised the module floor from 1.24).
- `jq` is **not** installed; use `python3` for JSON.
- Run the app: `make run`. Benchmarks: `make bench`. Tidy modules: `make tidy`.
- **Navigate by the map, not by reading everything:** read [CODEMAP.md](CODEMAP.md)
  (per-package `type`/`func` index) to find the file/symbol you need, then read
  that window. Regenerate it with `make codemap` after adding/moving top-level
  declarations. — see [RETROS 0001](RETROS/0001-quality-and-architecture-hardening.md)
- Desktop build deps (Linux, Wails v3 GTK4 backend): `pkg-config libgtk-4-dev
  libwebkitgtk-6.0-dev libsoup-3.0-dev`; then `make desktop` / `make run-desktop`.
  macOS/Windows ship the webview.

## Subagent model routing

When a session spawns subagents, spend the right model per task: **retrieval →
cheap; judgment → strong.** Pass the model per call (the `Agent` tool's `model`).
Default subagents inherit the parent (opus) — override **down** for mechanical
work, and **never downgrade a correctness-critical reviewer**.

| Task type | Model |
|-----------|-------|
| retrieval / "where is X" / grep fan-out | haiku |
| read-many-and-summarize, gather context | sonnet |
| test authoring from a clear spec | sonnet |
| mechanical refactor / codemod | sonnet |
| code review (bug hunting) / security review | opus (sonnet at low/medium effort) |
| architecture / planning / ADR decisions | opus |
| research synthesis / adversarial verify | opus → sonnet for fan-out legs |

This repo's `Explore`/`Plan` agents are **built-in** (no `.claude/agents/` files),
so there is no `model:` frontmatter to set — routing is per-call only. If custom
agents are ever added under `.claude/agents/`, set `model:` frontmatter on them:
search/retrieval agents → sonnet (haiku for pure file-finding); planning/
architecture → opus.

## Naming & style

- Branches: `feat/…`, `docs/…`, `fix/…` (kebab, scope-prefixed).
- Skills use gerund slugs (`registering-contracts`); the docs they own live under `docs/`.
- Commit messages: imperative subject, conventional-commit prefix where it fits
  (`feat(web):`, `docs:`).
- Adding a TUI page: `Page` constant + `String()` in `internal/tui/model.go`; `view*` in
  `internal/tui/views.go` wired in `renderBody`; a `View` smoke test covers it.
- **Dim text with the `--dim` token, never `opacity`.** `opacity` dims the foreground *and*
  its contrast with the surface, dropping text below WCAG AA on tinted fills — this trap has
  bitten four times (REGRESSIONS #10/#20/#21 + the sub-agent cluster). Use `color: var(--dim)`
  (AA-tuned, full opacity). Any surface hidden until opened (a `<dialog>`/overlay) gets a
  both-theme axe scan the day it lands — the static-page scan can't reach it. — see [RETROS 0005](RETROS/0005-deep-quality-architecture-and-test-hardening.md)

---
*Bootstrapped 2026-06-04 from CONTRIBUTING.md, Makefile, `.github/workflows/`, and
docs/ARCHITECTURE.md. Rules marked “ADR needed” are load-bearing invariants awaiting a
backfilled decision record.*
