# 0004. Fold supporting docs into the feature branch

- Status: accepted
- Date: 2026-06-04
- Deciders: Horia
- Related: [CONVENTIONS.md](../CONVENTIONS.md), PR #17 (the separate docs PR this corrects), CI-green gate

## Context

The web-UI backlog shipped as a sequence of phase PRs. Phase 1 put its learnings in a
**separate docs-only PR (#17)**, while Phases 2–4 folded ADR/REGRESSIONS/TECH_DEBT updates
into the phase branch itself. The split approach proved strictly worse: it serialized an
extra full CI cycle (~5 min) for a markdown-only change, and it risked a merge conflict on
shared docs (e.g. REGRESSIONS.md) between the docs PR and the next phase branch. The
supporting docs are also only meaningful alongside the change that motivated them, so
separating them breaks the reviewer's context.

Separately, this repo's environment has **no `gh` CLI and no `jq`**; PRs must be created and
merged through the GitHub REST API. That mechanics knowledge lived only in agent memory,
invisible to a human contributor reading the repo.

## Considered options

- **Separate docs-only PR per phase** — clean isolation of docs from code. Rejected: extra
  CI cycle per phase, cross-PR docs conflicts, and reviewer loses the change↔doc link.
- **Fold supporting docs into the feature branch** — ADRs, REGRESSIONS, TECH_DEBT, and
  CONTRACTS updates ship in the same PR as the change that motivated them.

## Decision

We **fold supporting docs into the feature branch**. The PR that changes behavior also
carries its ADR, its REGRESSIONS guard-test entry, its TECH_DEBT delta, and any CONTRACTS
update. No separate docs-only PR for work that belongs to a shipped change. (A docs *bootstrap*
that has no triggering code change — like seeding CONVENTIONS.md/CONTRACTS.md — may still be
its own branch.)

We also record the **gh-absent PR mechanics** as a repo convention: create/merge PRs via the
GitHub REST API with a stored PAT (`python3` + `urllib`; `jq` is not installed), and poll
check-runs for CI status before merge — consistent with the CI-green gate.

## Consequences

- Positive: one CI cycle per change; no cross-PR docs conflicts; reviewers see the change and
  its rationale together; the PR mechanics are now discoverable in-repo, not just in memory.
- Negative / cost we accept: feature PRs carry a few extra doc files, so the diff is larger and
  mixes prose with code.
- Follow-ups: CONVENTIONS.md Workflow section links here; future skills (recording-decisions,
  logging-learnings, managing-tech-debt, registering-contracts) write into the active branch.
