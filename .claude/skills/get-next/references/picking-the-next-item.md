# Picking the next item — precedence & sequencing

`next-issue.sh` is a **recommender** over `docs/issues/INDEX.md`. It encodes the precedence below
and flags drift; the human/agent makes the final call, with `NEXT_FEATURES.md` sequencing as the
tie-breaker. This file is the *why* behind the script so its output is interpretable.

## The store (two files, one truth)

- **`docs/issues/INDEX.md`** — the regenerated index. Two tables:
  - *Epics table*: `id | title | status | children`.
  - *Issues table*: `id | title | status | severity | group | links`.
  Newer epics have sometimes been filed as an `Epic:`-titled row **in the issues table** instead of
  the epics table. Both are valid; the picker recognizes either and resolves an epic's children
  from its explicit `children` list **∪** every issue whose `group` back-references it.
- **`docs/NEXT_FEATURES.md`** — the roadmap research doc. Its **"Recommended sequencing"** section
  orders items by dependency and value in a way a flat index can't (e.g. a P0 "consistency spine"
  must land before the items built on it). **Sequencing overrides raw severity.**

## Precedence (what "next" means)

1. **Open child of an open epic → BUILD it.** Among an open epic's open children, prefer higher
   severity, then lower id (oldest first). This is the common case: continue the epic in flight.
2. **Open epic with no children yet → BREAK IT DOWN.** The epic is a charter, not yet sliced into
   buildable issues. File child #1 with `tracking-issues`
   (`new-issue.sh "<first slice>" --group <epic-id>`), grounding it in the epic's charter and the
   `NEXT_FEATURES` item it maps to, then build that child. Keep slices small (one seam, one PR).
3. **Open epic, all children closed → STALE.** Either the epic is done (close it — `tracking-issues`)
   or it has un-filed follow-ups. This is a **human call**, never an auto-close or invented item.
4. **Nothing open → research, don't fabricate.** The roadmap is exhausted. Run a `NEXT_FEATURES`
   research pass (re-read the code, `TECH_DEBT.md`, and the product's differentiators; propose the
   next roadmap version and promote BUILD-FIRST picks into `docs/issues/`). Filing work you invented
   without that pass is how the roadmap drifts from the code.

## Tie-breakers (when several items are eligible)

- **Sequencing first.** If `NEXT_FEATURES` names an order (P0→P1→P2, "BUILD FIRST" tags), follow it.
- **Then severity** (`critical > high > medium > low`), then **oldest id**.
- **Then dependency**: an item whose seam another open item builds on goes first (read the "Touches"
  line in the `NEXT_FEATURES` entry).
- **One pillar per session** when two epics are both live — which pillar to advance is a product
  call; surface both and ask rather than interleaving unrelated work onto one branch.

## After the pick

Hand back to the `get-next` checklist: read the charter, name the branch, `start-fresh.sh`, confirm
a green base, then `practicing-tdd`. The picker never branches or mutates — selection and setup are
separate steps so a wrong pick is cheap to discard.
