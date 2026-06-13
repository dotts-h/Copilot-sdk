# Retro 0008 — recipe update + the `make doctor` fix, then a quality & architecture pass on an exhausted roadmap

- Date: 2026-06-13
- Scope: a process/maintenance session, then a code-quality + architecture milestone.
  Ran `/update-recipes` (core 0.1.1, quality 0.3.0), **diagnosed and fixed why `make doctor`
  had been failing across multiple prior sessions**, then — finding the promoted roadmap
  fully shipped — ran the requested code-quality pass and an architecture assessment, each
  PR → self-review → merged. Four PRs: #172 (recipes + doctor fix), #173 (quality pass),
  #174 (ADR-0056 architecture assessment), plus this docs wrap-up.

## What prompted it

`/update-recipes`, then a pointed question: *"do recipes finally work correctly in this
repo? We spent all day debugging this across sessions."* That framing was the real signal —
the recipe **content** flow worked, but something in the **plumbing** had been costing days.
The user then handed the session full autonomy to drive the rest while away.

## How recipes worked in this repo (the deep section)

**Verdict: the recipe *model* is sound and delivered; the recipe *resolution plumbing* was
broken by a wrong assumption in ADR-0054, and that single bug is what burned the prior
sessions. Now fixed.** Detail, because this is the part that kept biting:

- **What worked — the lock-driven merge model.** `.recipes/lock.json` records each recipe's
  version + bound answers, and `/update-recipes` is a 3-way merge, not a re-install. On this
  repo (a *harvest-mode* adopter — its own docs/scripts are canonical), that distinction is
  everything: the catalog's templates for the doc surfaces (CONVENTIONS, CONTEXT, ADR/RETROS
  READMEs, TECH_DEBT, REGRESSIONS) are **empty skeletons**, while the repo's copies are rich
  and load-bearing. A blind re-install would have flattened ~2,000 lines of canonical docs.
  The merge correctly **preserved** all of them and **applied** only the one genuine upstream
  improvement — the hardened `check-workflows.sh` (default-branch detection, a PyYAML-absent
  text fallback, Rule 2 over every workflow). One collision (`codemap.sh` generalized to
  language-agnostic vs. the repo's deliberate Go-specific version) was preserved-and-flagged,
  not auto-resolved. The model did exactly what ADR-0054 promised.
- **What was broken — `make doctor` couldn't find the catalog.** The headline failure, and
  the source of the multi-session debugging: `make doctor` exited 127 (`run-doctors.sh: No
  such file or directory`) in **every** plain shell / web session, even with `cookbook@ori`
  installed and enabled. Root cause: the Makefile resolved the catalog as
  `RECIPES_DIR ?= $(CLAUDE_PLUGIN_ROOT)` with a `../claude-skills/...` sibling-checkout
  fallback. **Both are empty in practice.** `$CLAUDE_PLUGIN_ROOT` is exported only *inside a
  plugin's own command/hook/skill execution* — never to the session shell or a user-run
  `make` (verified unset even in a login shell). ADR-0054's amendment literally says *"in a
  Claude session it's at `$CLAUDE_PLUGIN_ROOT`"* — that assumption is simply false for
  `make doctor`. The doctors only ever ran when someone hand-fed `RECIPES_DIR`, which is
  exactly why it "worked when debugging interactively" but never on its own.
- **The fix.** Resolve the catalog from the **marketplace install dir** —
  `$(firstword $(wildcard $(HOME)/.claude/plugins/marketplaces/*/plugins/cookbook) …)` — the
  stable path recorded in `known_marketplaces.json` (`installLocation`), present whenever the
  marketplace is installed, marketplace-name-agnostic, and needing no env var. `RECIPES_DIR=`
  still overrides for CI. Verified: `make doctor` now returns 0 with **all** recipe doctors
  green and nothing set. Logged as REGRESSIONS #23; CONVENTIONS "Quality gates" corrected.
- **The honest caveat — the design bug lives upstream.** The faulty `$CLAUDE_PLUGIN_ROOT`
  assumption is in the `cookbook` plugin's contract / `recipe-doctor` skill, so **every other
  adopter repo will re-hit this** until the catalog ships canonical "find-me-without-an-env-var"
  resolution. This repo's Makefile fallback is the local fix; the upstream fix is tracked as
  TECH_DEBT #19 (cross-repo, `dotts-h/claude-skills`, out of this session's push scope).
- **Why it cost days.** The failure was *silent and intermittent-looking*: interactive
  debugging often sets a var or runs from a checkout, so the doctor would pass in the moment
  and fail unattended — the classic "works when I watch it" trap. The lesson is the doctrine's
  own: **a gate that depends on ambient session state isn't a gate.** The fix makes resolution
  deterministic from a path that always exists.

## The roadmap was already done — so I did *not* fabricate work

The autonomy mandate was "run `/get-next` until the roadmap is complete." Running the picker:
**[4] no open epics with actionable work** — every epic and child through roadmap v16 / epic
0095 is closed. Per `get-next`'s own anti-fabrication rule (an exhausted roadmap calls for a
NEXT_FEATURES research pass, *not* an invented item), I **did not** unilaterally build the
unscoped roadmap-v17 "durable autopilot" candidate: it's a large, schema-bearing, persistence
+ scheduler epic that NEXT_FEATURES explicitly leaves to "a fresh pass," and building+merging
it to `main` unsupervised would violate the leash doctrine (verify architecturally-significant,
hard-to-reverse work before firing). Instead I completed every *safely-completable* thing:
the recipe fix, the requested quality + architecture passes, the bookkeeping (epic-0095 INDEX
drift), and this retro — and left v17 scoped-and-teed-up for the user's return.

## The quality & architecture pass

- **Quality (PR #173).** Two parallel read-only package audits found the codebase genuinely
  clean — **no correctness bugs, races, or determinism violations**. Applied only the safe,
  behavior-preserving findings: Workflow CRUD folded onto the shared `forgeCRUD[T]` generic
  (~45 lines of duplicated lock/lookup/error-re-render gone); a shared `pctClamped` helper and
  a single `subagentLabel` home; a dead `s.live` write removed; an SSE single-line fast-path;
  `Meter` keeping a count instead of an unbounded `[]Usage`; `PriceBook.Set` closing a $0
  cache-write footgun; `Subagents.Entries()` deep-copying like its siblings. Three new guard
  tests. Two independent reviewers confirmed byte-for-byte equivalence before merge.
- **Architecture (PR #174 / ADR-0056).** Assessed boundaries, dependency direction, coupling,
  drift. Acyclic graph, correct direction, the `copilot → ctxforge` edge ADR-0029-blessed,
  `web.Server` a coherent single-session aggregate (splitting it would fragment the one mutex
  — a correctness risk). **Verdict: sound, no structural change** — recorded as a "decided not
  to act" ADR (precedent: 0050/0035) with two watch-item tripwires, rather than forcing a
  speculative refactor. The honest engineering answer to "improve the architecture" was
  "assess → it's sound → record why," plus the one real cleanup (the CRUD generic) that
  shipped in #173.

## Skill / tool usage

- `cookbook:update-recipes` for the merge; parallel `general-purpose` sub-agents for the two
  quality audits and the architecture assessment + two independent diff reviewers (proportionate
  to a small, guard-tested diff — deliberately **not** a 7-angle high-effort `/code-review`
  fan-out, per CONVENTIONS' "size the review to the diff").
- The `main`-tag hazard **bit again**: a mid-session `git switch main` resolved the *tag*
  (93e2eb2), reverting the working tree; `git checkout -B main refs/remotes/origin/main`
  recovered it. CONVENTIONS' prescribed recovery command was itself the unsafe one — corrected
  this session (see Action items).
- CI waits via background until-loop timers + `pull_request_read get_check_runs` (never a
  foreground sleep, never `list_workflow_runs`).

## Action items

- **A1 — `make doctor` resolves without an env var.** ✅ Makefile marketplace-dir fallback;
  REGRESSIONS #23; CONVENTIONS corrected.
- **A2 — upstream the resolution pattern to the `cookbook` plugin** so other adopters get it.
  Tracked as TECH_DEBT #19 (cross-repo; not pushable from this session).
- **A3 — CONVENTIONS no longer prescribes the unsafe `git switch main`.** ✅ It now points at
  `start-fresh.sh` / `git checkout -B main refs/remotes/origin/main`, naming the tag-resolves
  trap explicitly.
- **A4 — roadmap v17 (durable autopilot) is the teed-up next epic.** Not built (unscoped,
  large, unsupervised). A fresh NEXT_FEATURES pass should scope it before `get-next` picks it.

## Session-optimization (delta only — canonical list in CONVENTIONS "Session playbook")

- New this session: **`git switch main` resolves the tag** — use `start-fresh.sh` or
  `checkout -B main refs/remotes/origin/main` (folded into the base-verification rule above).
- Reinforced: parallel scoped sub-agents for breadth audits + independent diff verification
  keep a review proportionate without a full high-effort fan-out.

**Session cost:** medium — heavier than a pure docs session (0007). Cost went to: one
`/update-recipes` merge with full-diff reasoning across 11 files; ~5 sub-agents (2 quality
audits, 1 architecture assessment, 2 diff reviewers) at scoped budgets; four full CI cycles
waited on via background timers; and the toolchain warm + race-test runs. No full-file dumps
in the main thread (audits delegated, conclusions kept). The four-PR throughput (recipe fix →
quality → architecture → wrap-up) on one green `main` is the unit the cost bought.
