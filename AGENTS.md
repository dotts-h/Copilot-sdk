# AGENTS.md — my-orchestra

A cost-aware coding web app that drives the GitHub Copilot Go SDK, composes
context with the my-ctx forge, and meters AI-Credit spend in real time.

This file is a **thin pointer**; the canonical rules live in the docs below — read
them before acting. One fact, one home: never copy a rule here, link it.

- **Conventions (the constitution):** [docs/CONVENTIONS.md](docs/CONVENTIONS.md) —
  workflow, architecture invariants, quality gates, environment facts.
- **Context (the glossary):** [docs/CONTEXT.md](docs/CONTEXT.md) — the ubiquitous
  language (forge, lane, run, share, meter, ledger, reconcile…), defined once. Read it
  before naming a new type or writing a doc; add a term here before you write its code.
- **Codebase map (navigate first):** [docs/CODEMAP.md](docs/CODEMAP.md) —
  per-package `type`/`func` index; read it to find a symbol instead of opening
  files. Regenerate with `make codemap`.
- **Contracts (stable promises):** [docs/CONTRACTS.md](docs/CONTRACTS.md) —
  the `copilot.Client` seam, HTTP routes, event vocabulary.
- **Decisions (why):** [docs/adr/](docs/adr/) — one record per decision.
- **Dev loop (the unit-of-work ritual):** [docs/DEV_LOOP.md](docs/DEV_LOOP.md).
- **Releases (playbook):** [docs/RELEASING.md](docs/RELEASING.md).
- **Learnings & dead-ends:** [docs/REGRESSIONS.md](docs/REGRESSIONS.md).
- **Retrospectives (process learnings):** [docs/RETROS/](docs/RETROS/).
- **Tech-debt register:** [docs/TECH_DEBT.md](docs/TECH_DEBT.md).
- **Architecture overview:** [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).
- **Skills program:** [SKILLS_PLAN.md](SKILLS_PLAN.md).

Process infra is managed by cookbook recipes (`.recipes/lock.json`; conformance
via `make doctor`); process changes land in the Cookbook repo first — see ADR-0054.

Quick gates (exact in CONVENTIONS): `make lint && make test` before push; coverage
floor 65%; CI must be green before merge. Branch from `main`; never commit to it
directly. Go toolchain: `export PATH=$PATH:/home/ori913/go-install/go/bin`.
