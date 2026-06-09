# Running work in parallel (lanes)

Parallelism here is **not** limited by CI (it runs per-PR, so N PRs run at once) — it's limited by
**shared-state contention**. Two lanes that touch the same code seam, or both grab the next
monotonic id, conflict at merge. So the rule is: **parallelize items that are unblocked *and* touch
disjoint seams**, and **reserve the shared counters up front**.

## What can run in parallel (compute it, don't guess)

1. **Unblocked.** Run `scripts/next-issue.sh`. Its **"Parallelizable now"** list is the set of open
   items with no open `depends_on` blocker. A blocked item must wait for its blocker — that edge is
   a hard serialization point (#2's whole purpose).
2. **Disjoint seams.** Among the unblocked set, two items are lane-safe only if their **"Touches"**
   lines (in the issue / `NEXT_FEATURES` entry) name **non-overlapping files**. Two items both
   editing `internal/web/pages.go` will conflict — sequence them. Two items, one in
   `internal/telemetry` and one in `internal/web/static/app.css`, are a clean parallel pair.
3. **Independent decisions.** If both lanes need an ADR, they each consume a number — reserve them.

## The shared counters (reserve before fanning out)

These are append-only or monotonic and WILL collide if two lanes write them simultaneously:

| Shared resource | Collision | Mitigation |
|-----------------|-----------|------------|
| `docs/issues/NNNN` ids | two lanes pick the same next id | `reserve-ids.sh --issues N --stub` up front |
| `docs/adr/NNNN` ids | two ADRs grab the same number | `reserve-ids.sh --adrs M --stub` up front |
| `docs/issues/INDEX.md` | concurrent row edits conflict | each lane edits only its own row; orchestrator batches the index regen at the end |
| `docs/CONTRACTS.md`, `docs/CODEMAP.md` | concurrent edits | `CODEMAP` is generated (`make codemap`) once after merges; CONTRACTS edited only by the lane that owns that seam |

`reserve-ids.sh --issues 2 --adrs 2 --for "laneA,laneB" --stub` writes placeholder files for the
reserved numbers; **commit them on the shared base before fanning out** so every worktree sees the
numbers as taken. Each lane then renames its reserved stub to the real slug.

## The mechanics

1. **Plan the batch (this/orchestrator session).** From the parallelizable set, pick a disjoint-seam
   subset. Reserve ids/ADRs (`--stub`), commit on the base, push.
2. **One lane = one worktree = one branch = one PR.** Spawn each lane in its own git worktree so the
   working trees don't stomp each other:
   - via the `Agent` tool with `isolation: "worktree"` and `run_in_background: true`, one agent per
     lane, each handed its reserved id + assigned seam + the `get-next` → `practicing-tdd` flow;
   - or by hand: `git worktree add ../wt-<lane> <reserved-branch>` per lane.
3. **Each lane runs the normal loop** on its branch: `start-fresh.sh` is unnecessary inside a
   worktree already cut from the verified base — just confirm the base SHA — then TDD → gates → PR.
   Fold the lane's own doc records (its issue close-out, its ADR) into its own branch (ADR-0004).
4. **Merge serially, rebase between.** PRs merge one at a time with `--no-ff`; rebase the next lane on
   the new `main` before merging so the INDEX/CONTRACTS deltas stack cleanly. The doc-record commits
   are small and per-branch, so the only place they meet is the INDEX row each lane owns.

## When NOT to parallelize

- **A dependency chain** (A blocks B blocks C) is inherently serial — run it in one lane, in order.
- **A shared-seam cluster** (several items all reshaping `pages.go`) — one lane, sequential, or the
  rebases cost more than the parallelism saves.
- **Fewer than ~2 genuinely disjoint, unblocked items** — the worktree + reservation overhead isn't
  worth it; just take the single best item via the normal `get-next` flow.

## Boundary

This is orchestration guidance for the `get-next` front door. The lanes themselves still build via
`practicing-tdd` and the normal gates; `reserve-ids.sh` only hands out numbers — it does not file
issues (`tracking-issues`) or write ADRs (`recording-decisions`).
