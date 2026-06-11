---
name: get-next
description: Does the next unit of work end-to-end — verifies the base and branches fresh from a trustworthy main, picks the next roadmap item (the next open child of an open epic, or the next epic to break down) from docs/issues/ and NEXT_FEATURES.md, builds it test-first, then opens the PR, code-reviews it (applying fixes), and merges once CI is green. Use at the start of a session when asked to "get the next item", "start fresh", "pick up the next issue/feature", "what's next", or to begin the next epic child.
allowed-tools: Read, Bash, Grep, Glob, Edit, Write
---

# Get next (start fresh, pick the next item, drive it to merged)

The cost of getting this wrong is silent and expensive: work cut from a **stale `origin/main`**
(or from the repo's **`main` *tag*** that shadows the branch) lands on the wrong base, and a
half-finished branch reused for a new item smuggles unrelated changes into a PR. This skill makes
the session-start ritual **deterministic**: verify the base, branch fresh, pick the next item from
the single source of truth — then **carry the item all the way to a merged PR**. Invoking
`/get-next` means "do the next unit of work", not "tell me what it would be": after setup,
continue straight into the build (`practicing-tdd`), then PR → code review (+fixes) → CI-green →
merge, without stopping to ask between steps. Stop mid-ritual only for the genuine ambiguities
listed below. It does **not** file or restructure issues (that's `tracking-issues`) — it
*selects*, *sets up*, *builds, and lands*.

## Workflow (copy this checklist into your reply and tick it off)

> **Script paths resolve against this skill's base directory** (printed when the skill loads —
> `.claude/skills/get-next/`), NOT the repo root or your cwd: run
> `.claude/skills/get-next/scripts/next-issue.sh`. A `No such file` (127) from the repo root means
> you're in the wrong directory, not that the scripts are missing — do **not** fall back to
> hand-rolling the ritual (it skips `start-fresh.sh`'s codified base assertions). — RETROS 0003.

- [ ] **1. Pick the next item.** Run `scripts/next-issue.sh`. It reads `docs/issues/`, reconciles
      epic-vs-child status, follows `depends_on` edges (it will **not** recommend a blocked item),
      and prints a ranked recommendation:
  - **[1] BUILD `<id>`** — an open, **unblocked** child of an open epic → this is your item.
  - **[2] BREAK DOWN epic `<id>`** — an open epic with no children yet → file child #1 with
        `tracking-issues` (`new-issue.sh "<first slice>" --group <id> [--depends <id>]`), then build it.
  - **Blocked** items are listed with their open blocker (finish that first); **Parallelizable now**
        lists the unblocked set (see "Batching in parallel" below).
  - **[3] STALE flag** / **[4] nothing open** → see "When the pick is ambiguous" below.
  Cross-read `docs/NEXT_FEATURES.md` **"Recommended sequencing"** — `depends_on` encodes the *hard*
  order; sequencing adds the *value* ranking the index can't.
- [ ] **2. Read the item's charter.** Open `docs/issues/<id>-*.md`: the *what*, the seam/files it
      touches, the "why now", and any linked ADR. If it needs a decision, that ADR is written
      **first** (ADR-0004) — note it now.
- [ ] **3. Choose the branch name.** Scope-prefixed kebab per CONVENTIONS: `feat/…`, `fix/…`,
      `docs/…` (e.g. `feat/billing-cache-write-pricing`). One item per branch.
- [ ] **4. Start fresh from main.** Run
      `scripts/start-fresh.sh <branch> [--expect-sha <sha>] [--require <file>,<file>]`.
      It fetches, fast-forwards `main`, prints the resolved SHA, and cuts the branch from
      `refs/remotes/origin/main` (never the bare `main`, which the tag shadows). **Pass `--require`
      a file the item builds on** (a seam it touches) so a wrong base fails loud, not silent —
      e.g. `--require internal/telemetry/meter.go`. If it exits non-zero, **stop**: the base is
      wrong; do not branch over it. — CONVENTIONS "Verify the base before branching", RETROS 0002.
- [ ] **5. Set up the build.** Confirm the Go toolchain is on PATH
      (`export PATH=$PATH:/home/ori913/go-install/go/bin`), run `make lint && make test` once to
      confirm a green base.
- [ ] **6. Build the item** (`practicing-tdd`): failing test first, then the implementation, with
      the item's ADR/CONTRACTS/CONTEXT/CODEMAP/issue-close-out folded into the **same** branch
      (ADR-0004). **Do not stop after step 5 to ask whether to build** — selecting the item WAS
      the instruction to build it. Gates before pushing: `make lint && make test` (floor 65%) +
      `make e2e` when the UI changed.
- [ ] **7. Open the PR.** Push with `git push -u origin <branch>` and open a PR against `main`
      titled from the item (reference the issue id + epic/slice label). The PR body carries the
      what/how and the item's acceptance boxes, ticked.
- [ ] **8. Code-review the PR (+ apply fixes).** Run `/code-review --fix` on the diff. Apply
      confirmed correctness/reuse/simplification findings, note refuted/deferred ones in the
      review summary, re-run the gates, and push the fix commit to the same branch.
- [ ] **9. Merge when CI is green.** Wait for ALL checks on the PR head (lint, test, fuzz, e2e) —
      CI green is a hard precondition (CONVENTIONS). Then merge the PR (merge commit, matching the
      repo's `Merge PR #N: <title>` history) and confirm `origin/main` advanced. If a check fails:
      diagnose, fix on the branch, re-push, re-wait — a red check is never merged around.

## Batching in parallel

When the **Parallelizable now** set has two or more unblocked items, you can run them as concurrent
lanes — but parallelism is bounded by **shared-state contention**, not CI. Two items are lane-safe
only if they're unblocked **and** touch **disjoint seams** (compare their "Touches" lines). Before
fanning out, reserve the shared monotonic counters so lanes don't collide:
`scripts/reserve-ids.sh --issues N --adrs M --for "laneA,laneB" --stub`, commit the stubs on the
base, then spawn one worktree+branch+PR per lane (the `Agent` tool's `isolation: "worktree"` +
`run_in_background`). Full procedure, mitigations, and when *not* to parallelize:
[references/parallel-lanes.md](references/parallel-lanes.md). The dependency graph is what makes this
safe — only items with no edge between them belong in the same batch.

## When the pick is ambiguous

The recommender flags, it doesn't decide. **Ask the user** (don't guess) when:

- **Multiple open epics** (e.g. two roadmap pillars in flight) — which pillar this session advances
  is a product call. Surface the candidates + their "why now" and let them choose. **Harness
  metadata never breaks this tie**: an auto-generated session branch name or session title is
  plumbing, not product intent — inferring the pillar from it set up the wrong item once
  (RETROS 0003). Ask.
- **[3] STALE** — an epic is `open` but every child is `closed`. Either it's done (close it via
  `tracking-issues`) or it has un-filed follow-ups. Don't silently close or invent work.
- **[4] nothing open** — the roadmap is exhausted. Don't fabricate an item: a **NEXT_FEATURES
  research pass** (re-read the code + `TECH_DEBT.md` + the differentiators, propose roadmap vN+1)
  is the next move — say so and offer to run it.

## Harness/session branches are plumbing

A remote harness (Claude Code on the web, a GitHub Action) may designate an auto-generated
session branch (e.g. `claude/<old-epic-slug>-<hash>`) and a session title. **Neither carries
product intent** (RETROS 0003): the item comes from `docs/issues/` via `next-issue.sh`, and the
work happens on the **scope-prefixed feature branch from step 3/4** — never infer the item from
the session branch name, and never develop on it when it contradicts the picker. If the harness
requires its branch to be pushed, push the feature branch as the real PR head and say so; flag
the mismatch once in the reply rather than asking.

## Boundaries (what this skill does NOT do)

- **Doesn't file/restructure issues.** Creating, grouping, or closing issues is `tracking-issues`;
  this skill only *reads* the store to pick and *delegates* a needed file to it (the **close-out**
  of the built item rides the feature branch per ADR-0004).
- **Doesn't write ADRs itself.** A decision the item needs is `recording-decisions` (written
  first, on the same branch).
- **Doesn't cut releases.** Tagging/publishing is `cut-release`, run separately when asked.

## This repo (facts the scripts rely on)

- Source of truth: `docs/issues/INDEX.md` (epics + children) and `docs/NEXT_FEATURES.md` (roadmap +
  sequencing). The INDEX has had **two epic representations** (epics table vs. an `Epic:`-titled row
  in the issues table) — `next-issue.sh` handles both and back-references children by `group`.
- Hard ordering is the issue frontmatter `depends_on: [ids]` (blocked-by edges), filed by
  `tracking-issues` (`new-issue.sh --depends`) and mirrored to GitHub sub-issues/blocked-by by its
  `sync-github.sh`. The picker reads these to skip blocked items and compute the parallelizable set.
- The repo has a **tag named `main`** colliding with the branch, and the sandbox can serve a
  **stale `origin/main`** on first fetch — both have silently mis-based work before (RETROS 0002).
  `start-fresh.sh` always resolves `refs/remotes/origin/main` and asserts SHA/foundation for that reason.
- Branch from `main`, never commit to `main`; one item per branch; fold supporting docs (ADR,
  REGRESSIONS, TECH_DEBT, CONTRACTS, issue close-out) into the **same** feature branch (ADR-0004).
