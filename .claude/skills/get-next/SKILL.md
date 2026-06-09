---
name: get-next
description: Starts the next unit of work cleanly — verifies the base and branches fresh from a trustworthy main, then picks the next roadmap item (the next open child of an open epic, or the next epic to break down) from docs/issues/ and NEXT_FEATURES.md, and sets up the build. Use at the start of a session when asked to "get the next item", "start fresh", "pick up the next issue/feature", "what's next", or to begin the next epic child.
allowed-tools: Read, Bash, Grep, Glob, Edit, Write
---

# Get next (start fresh, pick the next item)

The cost of getting this wrong is silent and expensive: work cut from a **stale `origin/main`**
(or from the repo's **`main` *tag*** that shadows the branch) lands on the wrong base, and a
half-finished branch reused for a new item smuggles unrelated changes into a PR. This skill makes
the session-start ritual **deterministic**: verify the base, branch fresh, pick the next item from
the single source of truth, then hand off to the build. It does **not** build the feature (that's
`practicing-tdd` + the normal loop) and it does **not** file or restructure issues (that's
`tracking-issues`) — it *selects* and *sets up*.

## Workflow (copy this checklist into your reply and tick it off)

- [ ] **1. Pick the next item.** Run `scripts/next-issue.sh`. It reads `docs/issues/INDEX.md`,
      reconciles epic status against child status, and prints a ranked recommendation:
  - **[1] BUILD `<id>`** — an open child of an open epic → this is your item.
  - **[2] BREAK DOWN epic `<id>`** — an open epic with no children yet → file child #1 with
        `tracking-issues` (`new-issue.sh "<first slice>" --group <id>`), then build it.
  - **[3] STALE flag** / **[4] nothing open** → see "When the pick is ambiguous" below.
  Cross-read `docs/NEXT_FEATURES.md` **"Recommended sequencing"** — it ranks items the INDEX can't
  (e.g. a P0 "consistency spine" before its dependents). The script is a recommender; sequencing wins.
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
      confirm a green base, then hand off to `practicing-tdd`: write the failing test first.

## When the pick is ambiguous

The recommender flags, it doesn't decide. **Ask the user** (don't guess) when:

- **Multiple open epics** (e.g. two roadmap pillars in flight) — which pillar this session advances
  is a product call. Surface the candidates + their "why now" and let them choose.
- **[3] STALE** — an epic is `open` but every child is `closed`. Either it's done (close it via
  `tracking-issues`) or it has un-filed follow-ups. Don't silently close or invent work.
- **[4] nothing open** — the roadmap is exhausted. Don't fabricate an item: a **NEXT_FEATURES
  research pass** (re-read the code + `TECH_DEBT.md` + the differentiators, propose roadmap vN+1)
  is the next move — say so and offer to run it.

## Boundaries (what this skill does NOT do)

- **Doesn't build.** Feature work is `practicing-tdd` + the normal lint/test/PR loop.
- **Doesn't file/restructure issues.** Creating, grouping, or closing issues is `tracking-issues`;
  this skill only *reads* the store to pick and *delegates* a needed file to it.
- **Doesn't write ADRs.** A decision the item needs is `recording-decisions` (written first).
- **Doesn't open or merge the PR.** That's the end of the build loop (CONVENTIONS workflow).

## This repo (facts the scripts rely on)

- Source of truth: `docs/issues/INDEX.md` (epics + children) and `docs/NEXT_FEATURES.md` (roadmap +
  sequencing). The INDEX has had **two epic representations** (epics table vs. an `Epic:`-titled row
  in the issues table) — `next-issue.sh` handles both and back-references children by `group`.
- The repo has a **tag named `main`** colliding with the branch, and the sandbox can serve a
  **stale `origin/main`** on first fetch — both have silently mis-based work before (RETROS 0002).
  `start-fresh.sh` always resolves `refs/remotes/origin/main` and asserts SHA/foundation for that reason.
- Branch from `main`, never commit to `main`; one item per branch; fold supporting docs (ADR,
  REGRESSIONS, TECH_DEBT, CONTRACTS, issue close-out) into the **same** feature branch (ADR-0004).
