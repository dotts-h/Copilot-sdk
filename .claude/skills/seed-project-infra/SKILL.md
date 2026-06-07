---
name: seed-project-infra
description: Seed a new (or under-structured) repo with the portable engineering infrastructure this project learned — single-run CI, a self-enforcing workflow guard, a correct release workflow, the doc skeleton (CONVENTIONS + CONTEXT glossary + ADR log + generated CODEMAP), and the "one fact, one home" + leash doctrine. Use when starting a new project, or when asked to set up conventions/CI/docs so good defaults are baked in instead of re-derived.
allowed-tools: Read, Write, Edit, Bash, Grep, Glob
---

# Seed project infrastructure (the portable kit)

Conventions that live only as prose drift and don't travel. This skill makes the hard-won
defaults **executable and portable**: a new repo starts with them baked in, so the same
lessons aren't re-learned (and the same mistakes re-made) per project.

## The doctrine (carry this to every project)

1. **One fact, one home.** A decision's *why* lives in exactly one ADR; code comments state
   the *contract* tersely and point (`— ADR-00NN`); the contracts registry indexes seams; the
   glossary owns domain terms. Duplication across layers is the main drift engine.
2. **A `CONTEXT.md` glossary per repo.** The ubiquitous language, defined once. It shrinks every
   prompt (working memory) and is the first thing an architecture pass reads. Add a term here
   *before* writing its code.
3. **Small, auditable diffs; leash the seam-touching ones.** One tight change per PR. For a
   refactor that crosses a module boundary or a public seam, run plan-first (human approves the
   plan) before editing.
4. **Enforce with hooks, not memory.** A rule you must remember is a rule that fails. Encode each
   hard-won invariant as a deterministic check wired into CI (can't merge a violation) and
   `make lint` (caught before push).
5. **Verify outward-facing actions before firing.** Releases, published artifacts, anything hard
   to reverse — confirm the inputs and check the result; never fire blind.

## The scaffolds to drop in

- **CI that runs once per change.** Workflow triggers must be:
  ```yaml
  on:
    push:
      branches: [main]        # merges only — NEVER a feature-branch glob
    pull_request:
      branches: [main]        # validates the open PR
    workflow_dispatch:
  ```
  Listing a feature branch under `push` doubles every run (push + pull_request both fire on
  different `github.ref`s the concurrency group can't dedup). Keep the matrix/heavy jobs `needs:`
  the cheap ones; path-filter deploy/desktop workflows to `main`.
- **A self-enforcing workflow guard.** Copy `scripts/check-workflows.sh` (fails on a feature-branch
  `push` trigger or the `${GITHUB_REF_NAME:-…}` release bug); wire it into the CI lint job and as a
  `make lint` dependency. Optionally add the `scripts/hook-check-workflows.sh` PostToolUse hook for
  the local dev loop.
- **A correct release workflow.** Version resolves `${{ github.event.inputs.tag || github.ref_name }}`
  (see the `cut-release` skill). Cross-compile + checksums + `generate_release_notes`.
- **The doc skeleton** (thin `CLAUDE.md` pointer → the rest):
  - `docs/CONVENTIONS.md` — the constitution (workflow, gates, architecture invariants).
  - `docs/CONTEXT.md` — the glossary (seed it empty-but-structured).
  - `docs/CONTRACTS.md` — the seam/route/schema registry.
  - `docs/adr/` — one record per decision; `docs/REGRESSIONS.md` — every fixed bug + guard test.
  - `docs/CODEMAP.md` — **generated** (`make codemap`), never hand-edited.
- **Quality gates in the Makefile + CI:** test (with a coverage floor), lint (fmt + vet + the
  workflow guard), and — for domain-pure packages — keep them dependency-free and unit-tested.

## Procedure

1. Inventory what exists (CI workflows, Makefile, `docs/`, `.claude/`). Don't clobber; fill gaps.
2. Drop the scaffolds above, adapting names/module paths. Prefer copying the canonical files from a
   reference repo over re-authoring them.
3. Wire the guard into CI lint + `make lint`; confirm `bash scripts/check-workflows.sh` passes.
4. Seed `CONTEXT.md` with the project's first domain terms and `CONVENTIONS.md` with the doctrine.
5. **Keep it lean.** A new repo inherits the *structure* and a few hundred lines — not a mature
   project's whole doc corpus. Grow the docs as decisions are actually made.

## Companion skills

- `cut-release` — publish a tagged release with version verification.
- (Spec these next) `pick-next-child` — read the roadmap/issue index, pick the next unit of work,
  scaffold its issue + branch (replaces a hand-carried planning prompt). `deepen-module` — the
  deep/shallow + deletion-test architecture lens, scoped and plan-first.
