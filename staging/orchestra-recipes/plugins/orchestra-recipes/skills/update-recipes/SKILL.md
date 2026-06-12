---
name: update-recipes
description: Updates recipes already installed in a repo to the plugin's current versions — reads .recipes/lock.json, diffs locked versions against the catalog, and applies each change as a semantic 3-way merge (old template vs. the repo's current file vs. new template) on a branch with a PR, never blind-overwriting local adaptations. Use when asked to update/upgrade recipes, sync a repo with new recipe versions, or after the recipe catalog ships a new release.
---

# Update recipes

The lock is what makes updates possible: it records which recipe **version**
rendered each file and with which **answers**. An update is a *merge*, not a
re-install — the repo's copy is canonical-for-that-repo, and local adaptations
are usually deliberate. The model is Copier-style: re-apply the new version by
reasoning about three texts, and let a human approve the result via PR.

## Workflow (copy this checklist into your reply and tick it off)

- [ ] **1. Read the lock.** `<target>/.recipes/lock.json` → for each entry:
      name, locked version, tier, answers. No lock → stop; this repo needs
      `adopt-recipes` first.
- [ ] **2. Diff versions.** Compare each locked version against
      `${CLAUDE_PLUGIN_ROOT}/recipes/<name>/recipe.yaml`'s `version`. Equal →
      skip. Report the update set before touching anything.
- [ ] **3. Branch.** Cut a branch (e.g. `chore/update-recipes`) from the
      default branch; never update on the default branch directly.
- [ ] **4. For each updated recipe, per provided file (manifest
      `provides.files`, filtered by the locked tier and `when` conditions):**
  - **OLD** = the template at the *locked* version, rendered with the locked
    answers. Recover it from the plugin repo's git history
    (`git -C ${CLAUDE_PLUGIN_ROOT} show <locked-tag-or-rev>:recipes/<name>/<template>`);
    if history is unavailable, reconstruct the best approximation and say so
    in the PR.
  - **CURRENT** = the file in the target repo.
  - **NEW** = the current template, rendered with the locked answers.
  - **Merge semantically:** every difference between OLD and CURRENT is a
    *local adaptation* — preserve it. Every difference between OLD and NEW is
    an *upstream improvement* — apply it. Where they collide, prefer the local
    adaptation and surface the collision in the PR body for a human call.
    Never replace CURRENT wholesale; never drop a local rule silently.
  - A file in NEW that doesn't exist locally is added; a file dropped upstream
    is *proposed* for deletion in the PR, not deleted unilaterally.
- [ ] **5. Update the lock** entries to the new versions (keep tier + answers;
      refresh `installedAt`). The lock change rides the same branch.
- [ ] **6. Verify.** Run each updated recipe's
      `${CLAUDE_PLUGIN_ROOT}/recipes/<name>/scripts/doctor.sh <target>` and the
      repo's own gates if available. Doctors must not be *worse* than before
      the update.
- [ ] **7. Open a PR** titled `chore: update recipes (<name> a.b.c -> x.y.z, …)`.
      Body: per-file merge summary — upstream changes applied, local
      adaptations preserved, collisions needing a human decision. **Never
      auto-merge; never push to the default branch.**

## Boundaries (what this skill does NOT do)

- **Doesn't install new recipes or change tier/answers** — that's
  `adopt-recipes` (a tier graduation is an adoption decision, not an update).
- **Doesn't resolve OLD-vs-CURRENT-vs-NEW collisions by overwriting** — a
  collision is escalated in the PR, full stop.
- **Doesn't run broad conformance audits** — `recipe-doctor` owns that; this
  skill runs doctors only to verify its own merge.
- **Doesn't merge the PR** — CI green + human review gate it, per the very
  conventions these recipes install.
