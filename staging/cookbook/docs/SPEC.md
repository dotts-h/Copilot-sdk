# SPEC — the recipe system's exact schemas and contracts

> One fact, one home: this file is the canonical definition of every machine
> format in the system. The installer, doctors, evals, and skills implement
> *this*; if behavior and SPEC disagree, one of them has a bug.

## 1. Recipe layout

```text
recipes/<name>/
├── recipe.yaml      # the manifest (below)
├── templates/       # installable files; {{param}} placeholders
├── scripts/         # doctor.sh (required) + recipe-owned operational scripts
└── evals/           # ≥3 runnable evals (fixture repo in mktemp → install/doctor → assert)
```

## 2. `recipe.yaml` (manifest)

Strict YAML subset: 2-space indents, inline `[a, b]` lists, block lists of
scalars and of maps. Parseable with PyYAML *and* with the installer's built-in
fallback parser (no YAML library required on the host).

```yaml
name: quality                  # unique recipe id (kebab)
version: 0.1.0                 # SemVer; updates compare against the lock
description: One paragraph, third person.
requires:
  recipes: [core]              # install-order dependencies (must be in the lock first)
  params:                      # the answers this recipe needs bound
    - name: coverage_floor     # identifier used as {{coverage_floor}} in templates
      type: int                # informational: string | int | bool(true|false) | enum(a|b)
      default: 65              # used when no --answer overrides it
      prompt: Coverage floor in percent
provides:
  files:
    - template: templates/ci-make.yml.tmpl   # path inside the recipe
      dest: .github/workflows/ci.yml         # path inside the target repo
      tiers: [S, M, L]                       # which tiers install this file
      when: build_system=make                # optional: only if answer equals value
  scripts: [scripts/doctor.sh]               # recipe-owned (not installed) scripts
  skills: []                                 # reserved: skills a recipe could surface
tiers:                          # human notes: what each tier means for this recipe
  S: ...
  M: ...
  L: ...
min_model:                      # certification placeholder: the smallest model
  install: sonnet               # tier each workflow step is certified for.
  doctor: haiku                 # values: haiku | sonnet | opus. Until a real
  harvest: opus                 # eval-based certification exists, these are
                                # the author's judgment — treat as a floor.
```

Rules:
- Every `{{placeholder}}` in a provided template MUST be declared in
  `requires.params` (the installer fails on unbound placeholders).
- `dest` files ending in `.sh` are installed executable.
- Templates never carry literal dates or secrets (repo lint enforces).

## 3. Placeholder substitution

`{{name}}` (optionally `{{ name }}`) where `name` matches `[A-Za-z_]\w*`,
replaced by the bound answer. Anything else between double braces (e.g. GitHub
Actions' `${{ github.ref }}` — contains dots) is left untouched. No logic, no
loops, no escaping — substitution must stay trivially auditable; conditional
content is modeled with separate templates + `when:`.

## 4. `.recipes/lock.json` (in the target repo)

```json
{
  "recipes": [
    {
      "name": "quality",
      "version": "0.1.0",
      "tier": "M",
      "answers": { "coverage_floor": "65", "build_system": "make" },
      "installedAt": "<UTC timestamp, RFC 3339, written at install time>"
    }
  ]
}
```

- One entry per recipe; re-install replaces the entry. Entries sorted by name.
- `answers` holds the **full merged set** (defaults + overrides), all strings —
  it is the record `update-recipes` re-renders old/new templates with.
- The lock is the update contract: *no lock → no update path* (adopt first).
- The repo's installed files are **canonical for that repo**. Updates are
  semantic 3-way merges (locked-version template ⋄ repo file ⋄ new template),
  never blind overwrites; doctors flag missing shape, not divergence.

## 5. Profile (`profiles/<name>.yaml`)

```yaml
name: mini-api
description: ...
tier: S                        # one tier for the whole adoption
recipes: [core, quality, contracts]   # installed in this (dependency) order
bindings: [api]                # bindings/<name>.md copied to docs/bindings/
answers:                       # per-recipe prefilled answers (override defaults;
  quality:                     # user confirmation may override these in turn)
    build_system: make
  contracts:
    fleet_member: "true"
```

## 6. `constellation.yaml` (fleet map)

```yaml
fleet: acme                    # fleet name
repos:
  - name: api                  # member repo name (matches its project_name)
    path: ../api               # path RELATIVE TO THIS FILE's directory
  - name: web
    path: .
```

`fleet-doctor.sh [constellation.yaml]` reads each member's
`docs/CONTRACTS.md`, extracts contract ids from bullets of the form
``- `contract-id` — description`` under `## Provides` and `## Consumes`, and
enforces: every consumed id has a provider in the fleet; no id has two
providers; every member path resolves. Provided-but-unconsumed ids are info,
not gaps.

## 7. Doctor contract

```text
doctor.sh <target-repo> [--tier S|M|L]
```

- Tier resolution: `--tier` flag > the recipe's lock entry > default `M`.
- Exit `0`: conformant. Exit `1`: gaps — one `GAP: <what> — <why it matters>`
  line per finding, plus a `== <recipe> doctor: N gap(s)` summary. Exit `2`:
  usage/environment error.
- Doctors check **presence and shape** (files exist, sections present, scripts
  executable, guards pass), never byte-equality with templates.
- Doctors are read-only and dependency-light: bash + python3 stdlib + git.

The aggregator (`skills/recipe-doctor/scripts/run-doctors.sh <target> [--all]`)
runs installed recipes' doctors per the lock (or all recipes in survey mode)
and exits 1 if any reported gaps.

## 8. Evals

Each recipe ships ≥3 evals: standalone bash scripts that build a fixture repo
in `mktemp -d`, run the real installer/doctor/scripts, and assert outcomes
(files, exit codes, report text). `make evals` runs all of them and fails on
any failure. Evals are the certification harness — a `min_model` claim is only
as good as the evals that hold at that tier.

## 9. Tiers

| tier | meaning | core artifacts |
|------|---------|----------------|
| S | mini repo (~4 lean files) | pointers + CONVENTIONS-lite (with inline `## Decisions`, graduating to ADRs at ~5) + CI guard + CONTRACTS |
| M | standard repo | + full CONVENTIONS, CONTEXT, ADR log, REGRESSIONS, issues store, dev loop |
| L | large/long-lived repo | + generated CODEMAP wiring, RETROS/, TECH_DEBT register |

Tier is recorded per recipe in the lock; a tier change (graduation) is an
*adoption* decision (re-run adopt-recipes), not an update.
