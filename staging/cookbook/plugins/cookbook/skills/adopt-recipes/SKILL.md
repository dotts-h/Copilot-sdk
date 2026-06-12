---
name: adopt-recipes
description: Installs versioned engineering-process recipes (conventions, gates, dev loop, issues, releases, contracts) into a target repo — inventories the repo, proposes a profile + tier + answers, confirms with the user, applies recipes in dependency order with HARVEST mode for brownfield repos (never clobbering existing docs), writes the .recipes/lock.json, and reports backfill gaps. Use when asked to adopt recipes, install recipes, apply a profile, set up engineering process/conventions/CI-gates in a repo, or bring a repo under recipe management.
---

# Adopt recipes

Recipes are doc-based software: installable, versioned process packages. This
skill is the installer's brain — the deterministic parts live in scripts; the
judgment (profile choice, harvesting, merging) is yours. **The repo's existing
content always wins over a template**: the installer never overwrites, and you
never delete working process docs — you fold them in and flag gaps.

## Workflow (copy this checklist into your reply and tick it off)

- [ ] **1. Inventory.** Run
      `${CLAUDE_PLUGIN_ROOT}/skills/adopt-recipes/scripts/inventory.sh <target>`.
      It reports language mix, build system, CI, existing process docs, any
      prior lock, and a suggested tier. Read — don't re-derive these by hand.
- [ ] **2. Propose.** Pick the closest profile from
      `${CLAUDE_PLUGIN_ROOT}/profiles/` (app-full, library, mini-api,
      mini-fe-components, mini-functions, greenfield-lean) and present a short
      table: profile, tier (S/M/L), recipes, bindings, and every answer you'll
      bind (project_name, default_branch, build_system, lint/test commands,
      coverage_floor, fleet_member…), with the values the inventory implies.
      Read each recipe's `recipe.yaml` `requires.params` for prompts/defaults.
- [ ] **3. Confirm with the user.** This is a hard stop: tier and profile shape
      the repo durably. Adjust per their answers.
- [ ] **4. HARVEST (brownfield only).** If the inventory found existing process
      sources (CONTRIBUTING.md, Makefile, package.json scripts, CI workflows,
      a hand-written CLAUDE.md), generate the repo's CONVENTIONS **from them**:
      extract the real commands, branch rules, and review habits into the
      template's sections. Existing files are never replaced — where a recipe
      wants a file that exists, merge the recipe's *structure* into the repo's
      *content* by editing the existing file, and keep what the repo had.
      Anything the recipes expect but the repo can't satisfy yet becomes a
      **backfill checklist item** in your final report, not a fake fact.
- [ ] **5. Install in dependency order.** For each recipe in the profile
      (order: core → quality → issues → loop → contracts → release; each
      manifest's `requires.recipes` must already be installed), run:
      `${CLAUDE_PLUGIN_ROOT}/scripts/install-recipe.sh --recipe
      ${CLAUDE_PLUGIN_ROOT}/recipes/<name> --target <target> --tier <T>
      --answer k=v ...`. SKIP lines are expected on brownfield — those files
      are yours to merge by hand (step 4), not the installer's.
- [ ] **6. Copy bindings.** For each binding in the profile, copy
      `${CLAUDE_PLUGIN_ROOT}/bindings/<name>.md` to
      `<target>/docs/bindings/<name>.md` and fill what the inventory already
      answered; its "Backfill checklist" joins your report.
- [ ] **7. Verify the lock.** `<target>/.recipes/lock.json` lists every
      installed recipe with version, tier, and answers (the installer wrote it).
- [ ] **8. Wire the doctor.** Add a conformance hook so drift is caught by
      machinery, not memory: a `doctor` target in the Makefile (or npm script)
      that runs the recipe-doctor aggregate, and mention it in CONVENTIONS'
      Quality gates. At minimum confirm `scripts/check-workflows.sh` is wired
      into lint/CI (the quality doctor checks this).
- [ ] **9. Run the doctors and report.** Run every installed recipe's
      `${CLAUDE_PLUGIN_ROOT}/recipes/<name>/scripts/doctor.sh <target>` (or the
      recipe-doctor skill's aggregator). Final report: what was installed
      (recipe@version, tier), what was harvested vs. templated, every SKIP you
      merged by hand, and the **backfill checklist** of flagged gaps.

## HARVEST mode rules (brownfield)

- **Never clobber; never fake.** A generated CONVENTIONS states only facts the
  repo evidences (a Makefile target, a CI step, a CONTRIBUTING rule). Wishes go
  to the backfill list.
- Prefer editing the repo's existing doc into the recipe's shape over creating
  a parallel file — two constitutions is worse than none.
- A repo with its own CLAUDE.md keeps its content; restructure it toward the
  thin-pointer pattern only with the user's agreement (step 3 covers this).

## Boundaries (what this skill does NOT do)

- **Doesn't update installed recipes** — version diffs and 3-way merges are
  `update-recipes`.
- **Doesn't run standalone conformance audits** — that's `recipe-doctor`; this
  skill only runs doctors to verify its own install.
- **Doesn't write product code, tests, or CI fixes** beyond wiring the recipe
  files; backfill items are reported, not silently implemented.
- **Doesn't push, tag, or open PRs** — it changes the working tree and reports;
  committing is the caller's flow.
