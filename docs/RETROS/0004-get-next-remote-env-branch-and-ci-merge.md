# Retro 0004 — get-next, end-to-end in the web env (R6 / epic 0076 closed)

- Date: 2026-06-11
- Scope: a single `/get-next` session run entirely in the **remote/web harness** (Claude
  Code on the web): picked **issue 0082** (R6 citation cards), built it test-first on the
  ADR-0045 seam (ADR-0049), opened **PR #152**, code-reviewed it (`--fix`), and merged on
  green CI — closing epic **0076** (R1–R6). The build was clean; the lessons are in the
  **operational flow** around it, which get-next had under-specified for this environment.

## What worked

- **The ritual held.** `next-issue.sh` ranked the pick (0082, unblocked once 0077 closed),
  `start-fresh.sh --require internal/web/markdown.go` asserted the base, and the build loop
  (failing test → impl → ADR/CONTEXT/CODEMAP/issue close-out folded in) compounded exactly
  as designed.
- **`/code-review --fix` earned its place.** High-effort review found **two real bugs** the
  unit pass missed: citation definitions were lifted out of fenced code blocks (a phantom
  source + an emptied code sample), and blanking a definition line reflowed adjacent prose.
  Both were fixed with regression tests before merge. Two other findings (origin-spoofing, a
  claimed "no init cycle") were correctly refuted — the init cycle was **real** and
  compiler-verified, which is *why* the inline marker is hand-built rather than routed
  through `frag()`.

## What cost too much (the under-specified parts)

- **The push-target branch was a genuine fork in the road the skill didn't name.** The
  harness mandated "develop on `claude/get-next-endpoint-e8u4bz`; never push elsewhere" — a
  **hard** constraint that collides with the skill's default lean ("make a scope-prefixed
  `feat/` branch, push it as the PR head"). RETROS 0003 had only addressed *not inferring the
  item* from the session branch; it said nothing about where work *lands* when the harness
  dictates it. Resolving it required working out, in-session, that the right move is to cut
  **the mandated branch itself fresh from `origin/main`** — and that `start-fresh.sh` refuses
  an existing branch, so a harness-pre-created branch must first be confirmed free of unique
  work and deleted. → **A1.**
- **Steps 7–9 assumed a local `git`/`gh` world that isn't this one.** There is no `gh` CLI;
  PRs, checks, and merges go through the GitHub **MCP** tools. CI can't be polled from a
  Monitor (no `gh` in the shell, and MCP calls can't run inside a Monitor command), and
  foreground `sleep` is blocked — so "wait for green, then merge" had to be re-derived as a
  **background-`sleep` timer → re-check `get_check_runs` → loop** pattern, and webhook events
  don't even deliver CI-success. None of this was in the skill. → **A2.**

## Action items

- **A1 — codify the hard-vs-soft push-target split.** ✅ Done this session: the skill's
  "Harness/session branches are plumbing" section now distinguishes the **soft case** (pick
  your own `feat/` branch) from the **hard case** (the harness mandates a push target → cut
  *that* branch fresh from `origin/main`; delete a stale pre-created copy first since
  `start-fresh.sh` refuses an existing branch). The item still comes from the picker; only the
  branch name is the harness's.
- **A2 — codify the remote-env PR/CI/merge mechanics.** ✅ Done this session: steps 7 and 9
  now state that the web env uses the GitHub MCP tools (`create_pull_request`,
  `pull_request_read get_check_runs`, `merge_pull_request`), that CI is waited on via a
  background-timer re-check loop (not a Monitor, not foreground `sleep`), and that the timer
  poll — not webhooks — is the reliable CI-green signal.
- **A3 — keep the review non-optional on stretch slices.** No doc change; this retro is the
  record. The two bugs `/code-review` caught were both in the "lift definitions out of the
  line stream" logic — exactly the kind of structural edge a green unit suite hides. The
  fence-awareness + drop-don't-blank fixes generalized the mechanism rather than patching the
  two symptoms.
