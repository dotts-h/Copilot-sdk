# MERGE_PLAN — folding cookbook into dotts-h/claude-skills

> This repo is a **staging ground**: a standalone dev marketplace
> (`cookbook-dev`) so the plugin can be installed, exercised, and
> hardened in isolation. The end state is a **second plugin inside the
> `dotts-h/claude-skills` marketplace**, alongside the existing skills plugin.

## Target shape (in claude-skills)

```text
claude-skills/
├── .claude-plugin/marketplace.json     # gains ONE new plugin entry
└── plugins/
    ├── orchestra-skills/               # existing methodology skills
    └── cookbook/              # ← this repo's plugins/cookbook, lifted as-is
```

Marketplace entry to add (only the marketplace name changes for users:
installs become `cookbook@orchestra` instead of
`cookbook@cookbook-dev`):

```json
{
  "name": "cookbook",
  "source": "./plugins/cookbook",
  "description": "Recipe system: install versioned engineering-process packages into any repo, update via lock + 3-way merge, check conformance with doctors."
}
```

## What moves, what renames, what stays

| item | action |
|------|--------|
| `plugins/cookbook/**` (plugin.json, skills, recipes, bindings, profiles, scripts) | **moves verbatim** — it is self-contained; all internal paths are plugin-root-relative (`${CLAUDE_PLUGIN_ROOT}`, `$(dirname "$0")` walks) |
| `.claude-plugin/marketplace.json` (this repo) | **does not move** — claude-skills' marketplace.json gains the entry above |
| `Makefile`, `scripts/lint.sh` | **merge** into claude-skills' equivalents (or copy under a `recipes-` prefix if it has its own gates) — `evals` and `lint` targets must keep running in the merged repo's CI |
| `docs/SPEC.md`, `docs/DESIGN.md` | **move** to claude-skills `docs/recipes/` (or stay plugin-adjacent); links in README/skills updated |
| `docs/MERGE_PLAN.md` | retired (its job is done) |
| `README.md` | shrinks into a section of claude-skills' README + the catalog table |
| repo `dotts-h/cookbook` | archived with a pointer once the merge lands |

No skill renames are needed: `adopt-recipes`, `update-recipes`,
`recipe-doctor` collide with nothing in the skills plugin, and skill names are
already plugin-scoped.

## Pre-merge test gates (in this repo, before lifting)

1. `make lint && make evals` green at the lift commit.
2. Fresh-host smoke: add this repo as a marketplace, install the plugin, run
   `/adopt-recipes` against (a) an empty repo with the `greenfield-lean`
   profile and (b) a brownfield fixture with an existing CLAUDE.md +
   CONTRIBUTING — confirm harvest-not-clobber and a correct lock.
3. `/update-recipes` smoke: bump one recipe's version + template here, re-run
   against a repo locked at the old version, confirm a PR-shaped 3-way merge
   (no overwrite).

## Post-merge test gates (in claude-skills)

1. The marketplace parses: both plugins listed, both installable side by side.
2. Re-run the fresh-host smoke via `cookbook@orchestra`.
3. Boundary check: prompts that should route to the *skills* plugin
   (tracking-issues, cut-release) still do — the recipe skills' descriptions
   target installation/update/conformance, not day-to-day operation, and that
   separation must survive the merge.
4. claude-skills CI runs this plugin's `evals` + `lint` (port the two targets).

## Versioning across the merge

The plugin's own version (plugin.json, currently `0.1.0`) and each recipe's
`version` travel unchanged — locks in already-adopted repos keep working
because updates resolve against recipe versions, not marketplace identity. The
first release *after* the merge bumps the plugin minor to mark the new home.
