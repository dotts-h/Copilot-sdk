---
name: recipe-doctor
description: Runs the conformance doctors of every recipe installed in a repo (per .recipes/lock.json) and aggregates the gaps into one readable report with suggested fixes. Use when asked to check recipe conformance, audit a repo's process health, run the doctor(s), diagnose drift from installed recipes, or survey an un-adopted repo against the recipe catalog.
---

# Recipe doctor

Doctors are the "enforce with hooks, not memory" half of the recipe system:
deterministic scripts that check *presence and shape*, never byte-equality —
a repo's local adaptations are legitimate; missing or broken machinery is not.
This skill runs them all and turns the output into an actionable report.

## Workflow (copy this checklist into your reply and tick it off)

- [ ] **1. Aggregate.** Run
      `${CLAUDE_PLUGIN_ROOT}/skills/recipe-doctor/scripts/run-doctors.sh <target>`.
      It reads `.recipes/lock.json` and runs each installed recipe's doctor at
      its locked tier. No lock? Re-run with `--all` for a survey of every
      recipe (expect gaps — that's the point of a survey) and recommend
      `adopt-recipes`.
- [ ] **2. Read every `GAP:` line.** Each is one missing/broken conformance
      fact with the reason it matters baked into the message.
- [ ] **3. Classify each gap:**
      - *mechanical* — missing file/exec bit/stale index; safe to fix by
        re-rendering the template or a one-line change.
      - *judgment* — tier mismatch, fleet contract unsatisfied, fat pointer
        file; needs the user (or another repo) to decide.
- [ ] **4. Report.** One table: recipe · gap · class · suggested fix. Green
      doctors get one line, not a table.
- [ ] **5. Offer (don't assume) the fixes.** Apply mechanical fixes only when
      the user says so, then re-run step 1 to show green.

## Reading the exit codes

Each doctor: `0` conformant · `1` gaps (readable `GAP:` lines) · `2` usage
error. The aggregator exits `1` if any doctor reported gaps — wire it into a
`make doctor` target or CI for scheduled conformance.

## Boundaries (what this skill does NOT do)

- **Doesn't install or update recipes** — recommend `adopt-recipes` /
  `update-recipes` when gaps trace to a missing install or version drift.
- **Doesn't rewrite diverged files to match templates** — divergence is not a
  gap; doctors check shape, and local adaptations stay.
- **Doesn't fix product code or failing tests** — only recipe-conformance
  machinery, and only on explicit approval (step 5).
