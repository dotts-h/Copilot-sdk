# Retro 0002 — V28 PostToolUse command execution & closing epic 0052

- Date: 2026-06-09
- Scope: the single session that built **V28 / G5** (external command-ref hook
  execution, ADR-0032, PR #92), closed **epic 0052** (safe-autopilot governance),
  and cut **v0.3.0**.

## Context

One long session carried a full feature end to end: domain (`ctxforge`) → seam
(`copilot` executor) → UI/timeline (`convo`/`web`) → docs (ADR-0032, CONTEXT,
CONTRACTS §1–4, CODEMAP, issues 0056/0055/0052) → e2e → `/code-review` → PR → CI →
merge → release + verify. The work shipped clean (coverage 88.3%, CI green first
try, release correctly tagged). The lessons are about **where the time and context
actually went** — and they were not the feature.

## What worked

- **TDD per layer.** Failing test first for domain, seam, and web; each green before
  the next. No retrofitting; the seam's load-bearing invariant ("untrusted output can
  never flip a decision") got an explicit assertion.
- **`/code-review` (high) earned its keep — again.** Two independent finder agents
  converged on the same real defect: a `toolKind: write` PostToolUse hook — the ADR's
  *own* gofmt-after-write example — was silently **inert**, because the completion
  event yields no `req.Kind()` and `postToolMeta` only derived `mcp`/`""`. I had
  written it off as a "documented limitation." It wasn't a limitation; it was the
  motivating use case being broken. Fixed at depth (`toolKindFromName`) + a full-chain
  test. **Lesson: when review flags the *motivating example* as not working, that is a
  bug, not a footnote.**
- **Release guard held.** Tag push was 403'd (sandbox); the `workflow_dispatch`
  fallback resolved `v0.3.0` from the input (not the branch), verified post-publish.

## What cost too much

- **The `main`-tag / stale-remote incident (the big one).** The first `fetch` served a
  **stale `origin/main`** (pre-hooks `38f8827`), and the repo carries a **tag named
  `main`** (`b2191cd`) colliding with the branch. `git switch -c` off "main" landed the
  branch on the wrong base, and `internal/ctxforge/hook.go` "didn't exist" — sending me
  diagnosing a phantom. A re-fetch restored `origin/main` to `2d65e8a`; a hard reset
  fixed the branch. The cut-release skill *already* documents a stray `main` tag
  corrupting `git fetch origin main` — **this hazard has now bitten twice.** → **A1.**
- **Context burned on verbose MCP payloads, not code.** `list_workflow_runs` and
  `get_release_by_tag` each returned the full nested `repository` object *per run* and a
  full bot-uploader object *per asset* — tens of thousands of tokens of ~90% boilerplate
  to read a `tag_name` and a few asset names. The feature reads were proportionate; the
  MCP noise was the avoidable sink. → **A2.**
- **Polling friction.** Several turns lost to blocked foreground `sleep`s and
  placeholder `touch/rm` sentinels before settling on the background until-loop timer.
  The pattern should be reached for first, not last. → **A3.**

## Skill / tool usage

- `code-review --effort high` — high value (caught the inert-`toolKind` bug); the
  finder-agents-in-background flow worked well.
- `cut-release` — the pre-flight version audit + post-publish verify is exactly the
  guard that makes a 403-fallback release safe.
- GitHub MCP — correct, but the read-heavy endpoints are context-hostile; prefer the
  compact ones (`get_status`, `get_check_runs`).

## Action items

- **A1 — base-verification guardrail (DONE):** CONVENTIONS now mandates, before
  branching, asserting `git rev-parse origin/main` is the expected SHA and
  `git cat-file -e origin/main:<sentinel>` for the foundation — so a stale remote /
  ambiguous `main` tag fails loud. *Open follow-up:* delete or quarantine the stray
  `main` **tag** (two incidents now) — file it when tag-write access is available.
- **A2 — prefer compact CI/release reads (DONE):** CONVENTIONS now points status checks
  at `pull_request_read get_status`/`get_check_runs` over `list_workflow_runs`.
- **A3 — standard wait-without-sleep pattern (DONE):** CONVENTIONS records the
  background until-loop timer as the way to wait on CI/remote in this harness.

## Session-optimization checklist (carry forward)

1. **Verify the base before writing code** — `origin/main` SHA + a sentinel file;
   never trust the first fetch on this sandbox.
2. Map-first, windowed reads; full reads only when editing most of a file.
3. Prefer compact MCP endpoints; don't read `list_workflow_runs` to check one status.
4. Wait via a background until-loop timer; never a foreground `sleep`.
5. Treat a review finding against the *motivating example* as a bug to fix at depth,
   not a limitation to document.
6. `/code-review` (always) before push; package-scoped tests in the loop, full
   `make test` before push.
