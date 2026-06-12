# 0054. Process infrastructure managed by cookbook recipes — Cookbook-first changes

- Status: accepted
- Date: 2026-06-12
- Deciders: Horia
- Related: [ADR-0051](0051-no-third-party-efficiency-plugins-in-repo-session-playbook.md), `dotts-h/cookbook` (the recipe catalog), `.recipes/lock.json`, `Makefile` (`doctor`), [docs/CONVENTIONS.md](../CONVENTIONS.md)

## Context

This repo evolved the process stack — the CONVENTIONS constitution, quality gates,
the `check-workflows.sh` guard, the hybrid issue store, the dev loop, tag-driven
releases, the contracts registry. The Cookbook repo (`dotts-h/cookbook`) harvested
that stack into **versioned recipes**: manifest-driven packages with a per-repo
lock file, conformance doctors, and a 3-way-merge update path. That left two
unversioned copies of one process: improvements here had to be re-propagated to
Cookbook by hand (and vice versa), and the copies had already begun to diverge
(e.g. the workflow guard exists as two different files). One fact needs one home —
at the fleet grain, that home is Cookbook.

## Decision

This repo is an **adopter** of the cookbook recipes: profile `app-full`, tier L,
all six recipes (core, quality, issues, loop, contracts, release), installed in
**harvest mode** — no existing file was replaced; the repo's copies stay canonical
for this repo. `.recipes/lock.json` records each recipe's version, tier, and bound
answers; `make doctor` runs the lock-driven conformance doctors (it needs a
Cookbook checkout, located via `COOKBOOK_DIR`, default `../Cookbook`).

**Direction of flow:** changes to *process infrastructure* (gate wiring, guard
logic, doc skeleton shapes, loop/issue/release machinery) land in **Cookbook
first** as a recipe version bump, then flow here via the `update-recipes` 3-way
merge — never by hand-editing both repos. Rules that are *specific to this repo*
(architecture invariants, environment facts, naming) stay in this repo's
CONVENTIONS as before; recipes own shape, not repo facts.

Where a recipe expects a script at `scripts/` that this repo already implements
inside a committed skill (`new-issue.sh`, `sync-github.sh`, `start-fresh.sh`,
`next-issue.sh`), the `scripts/` path is a **thin delegator** to the skill's
copy — the recipe's stable entry point, one home for the logic.

## Consequences

- Process drift is caught by machinery (`make doctor`), not memory; divergence
  from a template is *not* a gap — doctors check presence and shape only.
- Improvements made here must be upstreamed: a process fix that stops at this
  repo is a regression of this decision. The richer pieces this repo has that the
  recipes lack (docs-only CI skip, codemap generation, fuzz gate) are upstream
  candidates for recipe bumps.
- Cookbook is v0.1.0: the first `update-recipes` cycle is a shakedown, reviewed
  as a normal PR — the lock makes it possible, not automatic.
- CLAUDE.md became an `@AGENTS.md` import; AGENTS.md is now the host-agnostic
  thin pointer (the core recipe's shape), carrying the map CLAUDE.md used to own.
