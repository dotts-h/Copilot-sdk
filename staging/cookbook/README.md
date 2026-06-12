# cookbook

**Installable, versioned, doc-based engineering process packages.** A *recipe*
turns a hard-won process (a constitution, quality gates, an issue store, a dev
loop, a release ritual, a contracts registry) into something you install into a
repo, keep updated, and conformance-check — the way a package manager treats
code.

## Recipe vs. skill vs. plugin

| thing | lives where | lifetime | who benefits |
|-------|-------------|----------|--------------|
| **plugin / skill** | the agent host | per-session context; leaves **no trace** in the repo | the agent that loaded it |
| **recipe** | the target repo (docs, scripts, CI, `.recipes/lock.json`) | **durable state**, versioned + updatable | *any* agent (or human) that opens the repo, on any host |

Skills here are the *operators* (adopt/update/doctor); recipes are the
*payload*. A repo that adopted recipes works with no plugin installed at all —
the docs and scripts are just there, host-agnostic (both `CLAUDE.md` and
`AGENTS.md` thin pointers).

## Quickstart

```text
# 1. Add this repo as a marketplace (Claude Code)
/plugin marketplace add dotts-h/cookbook

# 2. Enable the plugin
/plugin install cookbook@cookbook-dev

# 3. In the repo you want to bring under management
/adopt-recipes
```

`adopt-recipes` inventories the repo, proposes a profile + tier + answers,
confirms with you, installs in dependency order (harvesting existing docs on
brownfield — never clobbering), writes `.recipes/lock.json`, and reports gaps.
Later: `/update-recipes` (semantic 3-way merge to new versions, via PR) and
`/recipe-doctor` (aggregate conformance report).

Without the plugin, everything is runnable by hand:

```bash
make evals                       # run all recipe evals (fixture repos in mktemp)
make lint                        # bash -n all scripts + secret/date scans
make doctor TARGET=/path/to/repo # aggregate doctors against a repo
plugins/cookbook/scripts/install-recipe.sh \
  --recipe plugins/cookbook/recipes/core --target /path/to/repo --tier M
```

## Catalog

| recipe | layer | what it installs |
|--------|-------|------------------|
| `core` | 0 | CLAUDE.md + AGENTS.md thin pointers, CONVENTIONS constitution (doctrine), CONTEXT glossary; ADR log (M+); CODEMAP/RETROS/TECH_DEBT (L) |
| `quality` | 1 | single-run CI (make/npm flavors), the self-enforcing workflow guard, coverage-floor parameter, REGRESSIONS register (M+) |
| `loop` | 1 | the dev loop playbook + `start-fresh.sh` (verified base) + `next-issue.sh` (dependency-aware picker) |
| `issues` | 1 | hybrid issue store: markdown source of truth + GitHub mirror; INDEX + frontmatter format; epics via `group:` |
| `release` | 1 | tag-driven release workflow with verified version resolution; SemVer + verify-after playbook |
| `contracts` | 1 | CONTRACTS registry with Provides/Consumes; fleet `constellation.yaml` + `fleet-doctor.sh` cross-check |

**Bindings** (Layer 2, adapter docs — not recipes): `frontend`, `api`, `qa`,
`services` — each fills the slots Layers 0/1 leave open for that repo shape.

**Profiles** (recipes × tier × bindings × prefilled answers): `app-full`,
`library`, `mini-api`, `mini-fe-components`, `mini-functions`,
`greenfield-lean`.

**Tiers:** **S** (mini repo: ~4 lean files, decisions inline in CONVENTIONS) ·
**M** (+ ADR log, REGRESSIONS, issues store, loop) · **L** (+ CODEMAP wiring,
RETROS, TECH_DEBT).

## Docs

- [docs/SPEC.md](docs/SPEC.md) — exact schemas: `recipe.yaml`, `lock.json`,
  profiles, `constellation.yaml`, the doctor exit-code contract, `min_model`.
- [docs/DESIGN.md](docs/DESIGN.md) — why it's built this way.
- [docs/MERGE_PLAN.md](docs/MERGE_PLAN.md) — path into the `claude-skills`
  marketplace.

## Status

**Staging ground.** This repo is the development marketplace
(`cookbook-dev`) for the `cookbook` plugin; the merge target
is the `dotts-h/claude-skills` plugin marketplace (see the merge plan). APIs
above `0.x` stability are not promised yet — the lock file is what protects
adopted repos across changes.
