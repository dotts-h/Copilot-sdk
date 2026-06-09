# Skills & workflow research — what to adopt, what to skip (2026-06-09)

> Research deliverable, not a commitment. Companion to [SKILLS_PLAN.md](../SKILLS_PLAN.md)
> (the original 15-skill brief) and [NEXT_FEATURES.md](NEXT_FEATURES.md) (the product roadmap).
> Three questions, grounded in **this** repo's current state:
> **(A)** the external skills research at
> `dotts-h.github.io/claude-skills/research.html` — what's worth applying here;
> **(B)** GitHub's product/project processes — how they enhance our local-first workflow;
> **(C)** a `get-next` skill — start fresh from main and pick the next item (**shipped this pass**).
>
> The bar for adopting anything is the repo doctrine: *one fact, one home; enforce with hooks, not
> memory; small auditable diffs; conventions that are executable, not prose* ([CONVENTIONS](CONVENTIONS.md),
> [seed-project-infra](../.claude/skills/seed-project-infra/SKILL.md)).

## Where the workflow is now (so we don't re-add what we have)

This repo already runs a tight, mostly-systematized loop:

- **Roadmap → issues → build → record.** `NEXT_FEATURES.md` (research) promotes BUILD-FIRST items
  into `docs/issues/` epics+children (local markdown is the source of truth, mirrored to GitHub via
  `tracking-issues`), each built TDD on a fresh branch, closed with its PR# and an ADR where a
  decision was made (ADR-0004 folds docs into the same PR).
- **Skills program is live:** 13 global methodology skills + 4 in-repo (`cut-release`,
  `seed-project-infra`, `tracking-issues`, `governing-qa-framework`) — now 5 with `get-next`.
- **Quality gates are enforced, not remembered:** `make lint && make test`, 65% coverage floor,
  fuzz smoke on pricing, `-race`, Playwright e2e, a self-enforcing **workflow guard**
  (`scripts/check-workflows.sh`) wired into CI *and* `make lint`.

So the lens for everything below is **marginal value over what exists** — not "is this a good idea
in the abstract."

---

## Part A — `research.html` triage

The external doc proposes 5 new skills + best-practices, on a roadmap to Q1 2027. Triaged against
our state:

| Proposal | Verdict | Why (grounded here) |
|----------|---------|---------------------|
| **`security-reviewing`** (`govulncheck`, `gosec`, secrets, OWASP) | **Adopt — highest value** | We have a `/security-review` *command* but **no Go-specific SAST/CVE gate** in CI. `govulncheck` + `gosec` are zero-dependency, catch real issues, and fit the "enforce with hooks" doctrine. Wire as a CI job + `make security`. This is the one clear gap. |
| **`fuzzing-inputs`** | **Adopt-lite — we're half-way** | We already fuzz pricing (`make fuzz`). The gap is *systematizing target selection + corpus management*, not net-new capability. Fold a short "finding fuzz targets + `testdata/fuzz/` corpus" reference into the existing `hardening-tests`/`authoring-tests` skill rather than a whole new skill. |
| **`profiling-performance`** (P95 SLOs, `benchstat`, soak) | **Adopt-lite, defer SLO gating** | `bench_test.go` exists; `benchstat` baseline-compare is cheap and worth a `make bench-compare`. But **latency SLOs in CI are premature** for a local-first single-user app — they add flaky gates without a perf contract to defend. Take the comparison tooling, skip the CI threshold for now. |
| **`stress-testing-resilience`** (toxiproxy, chaos) | **Skip for now** | Chaos/fault-injection pays off for multi-service/networked systems. This is a single-binary local app whose only network dep is the Copilot SDK; toxiproxy infra is overhead without a topology to harden. Revisit **only if** a multi-service architecture emerges (their Phase 3 caveat agrees). |
| **Visual regression** (Playwright `toHaveScreenshot`) | **Adopt — low effort, real gap** | We have `screenshot-states.sh` with **no baseline diffing**, and a UI/UX refresh epic (0045) just shipped a token/theme system worth pixel-locking. `expect(page).toHaveScreenshot()` is zero-dependency and CI-native. Fold into `authoring-tests` + `governing-qa-framework`, gated to the stable pages first. |

**Best-practices worth importing verbatim** (most we already do — these are the deltas):

- **"Write ≥3 evals before the skill body."** Our `SKILLS_PLAN.md` drafts starter evals per skill;
  make this a hard precondition in any new-skill work, and keep the evals **in the skill dir** so
  they're runnable, not just listed.
- **"Every skill states what it does NOT do."** We do this informally; the strongest in-repo skills
  (`cut-release`, `tracking-issues`) have explicit boundaries. Make a **Boundaries** section
  mandatory — it's what keeps triggers from colliding as the skill count grows. (`get-next` models it.)
- **"No time-sensitive text; forward-slash paths; errors not swallowed."** Already our convention;
  worth a one-line lint idea (grep skills for `C:\\`, hardcoded dates) if drift appears.

**Net Part-A recommendation:** one new skill (`security-reviewing` + a CI SAST/CVE gate); two
*enhancements* (visual-regression + benchstat-compare into the existing QA skills); fold fuzzing
guidance into `hardening-tests`; **skip** chaos/stress and CI latency-SLOs as premature for this
app's shape. This is a ~3-item slice, not the external doc's 5-skill expansion — most of their
roadmap is already standing here.

---

## Part B — GitHub product processes → workflow enhancements

Our issue store is **local-markdown-first, mirrored to GitHub** (`tracking-issues`). GitHub shipped
its hierarchy/planning primitives to GA in 2026 — **sub-issues, issue types, advanced search,
50k-item Projects**. The question isn't "switch to GitHub" (the local store is deliberate: diffable,
offline, greppable, the source of truth) — it's **which GitHub-native primitives to mirror *into*,
so the GitHub side stops being a flat dump and becomes a real planning surface** without moving the
source of truth.

| GitHub primitive | Maps to our concept | Enhancement | Effort |
|------------------|--------------------|-------------|--------|
| **Sub-issues** (GA 2026) | epic → child (`group:` field) | Mirror the `group:` back-reference as a real **sub-issue link**, so the GitHub epic shows its children as a checklist with progress — today the hierarchy only exists in our markdown. `tracking-issues` `sync-github.sh` gains a parent-link step. | **S** — highest leverage |
| **Issue types** (GA 2026) | epic / feature / bug / tech-debt | Replace ad-hoc labels with org **issue types**; our `severity` + epic-vs-leaf already carry the data. Makes the GitHub backlog filterable by *kind* without label drift. | **S** |
| **Milestones** | roadmap version (v8, v9, v10…) | One milestone per roadmap version; assign each epic's children. Gives a burn-down per roadmap cut that our `INDEX.md` can't render. Ties to the SemVer-per-epic rule (epic → minor). | **S** |
| **Projects (roadmap view)** | the whole `INDEX.md` | A single Project board (table + roadmap layout) as the **read view** over the mirrored issues — never the source of truth, just the cross-team lens. Auto-triage workflows can set status from PR state. | **M** |
| **GitHub flow / status automation** | our branch→PR→merge loop | Project workflows that flip issue status on PR open/merge close the loop we currently update by hand in `INDEX.md`. Pairs with the issue close-out we already fold into the feature branch. | **M** |

**The doctrine guardrail:** GitHub becomes a **projection**, markdown stays canonical. Anything that
would make GitHub the source of truth (editing issue text on GitHub, planning only on the board)
violates "one fact, one home" and the offline/diffable rationale in `tracking-issues`. The win is a
*better mirror*, not a migration.

**A concrete first slice (recommended):** extend `tracking-issues`'s `sync-github.sh` to (1) set an
**issue type** from the file (epic vs. leaf), and (2) create the **sub-issue parent link** from
`group:`. That alone turns the GitHub side from a flat list into the epic-hierarchy our markdown
already encodes — small, reversible, and it leaves the source of truth untouched. Milestones and a
Projects board are a fast follow once the hierarchy mirrors cleanly.

> Note on "GitHub product processes" as *methodology* (shape-up / GitHub-flow cadence): our loop
> already embodies the useful parts — small PRs, one item per branch, CI-green-before-merge,
> research-doc-then-promote. The gap is **tooling fidelity on the mirror**, not process — so the
> recommendations above are primitives, not a new ceremony.

---

## Part C — the `get-next` skill (shipped this pass)

The session-start ritual was tribal knowledge re-performed by hand every branch: verify the base,
branch fresh, find the next item, set up. Two failure modes made it worth hardening into a skill —
both have **already bitten** (RETROS 0002): work cut from a **stale `origin/main`**, and from the
repo's **`main` *tag*** that shadows the branch. This pass ships
[`.claude/skills/get-next`](../.claude/skills/get-next/SKILL.md):

- **`scripts/start-fresh.sh <branch> [--expect-sha] [--require a,b]`** — encodes the CONVENTIONS
  "verify the base" rule: fetches, fast-forwards `main`, prints the resolved SHA, and cuts the branch
  from `refs/remotes/origin/main` (**never** the bare `main` the tag shadows). `--require <seam-file>`
  asserts the foundation is present so a wrong base **fails loud, not silent**. (Verified: it detects
  this repo's real `main`-tag collision and rejects a bad `--expect-sha`.)
- **`scripts/next-issue.sh`** — reads `INDEX.md`, reconciles **epic status vs. child status**, and
  recommends: BUILD an open child → BREAK DOWN an open epic with no children → FLAG a stale epic →
  fall back to a NEXT_FEATURES research pass. Handles both INDEX epic representations and resolves
  children by `group` back-reference. (Verified against the live INDEX: correctly surfaces epics
  0050/0051 as the next work and flags 0042/0045 as stale-open.)
- **`SKILL.md` + `references/picking-the-next-item.md`** — the checklist + the precedence/sequencing
  *why*, with explicit **Boundaries**: it selects and sets up; it does **not** build (`practicing-tdd`),
  file issues (`tracking-issues`), write ADRs (`recording-decisions`), or open/merge PRs.

**The loop it closes:** `get-next` (start + pick) → `tracking-issues` (file the child if needed) →
`practicing-tdd` (build) → `/code-review` + gates → PR → close-out folded into the branch (ADR-0004)
→ `get-next` again. It's the missing front-door to a loop whose other stations were already skills.

**Dogfood signal:** running the picker today says the next real work is breaking down **epic 0050
(billing fidelity, roadmap v10)** into its first child — matching `NEXT_FEATURES` v10 sequencing
(P0 consistency spine first). The two "open" epics with all-closed children (0042, 0045) are
flagged for a human to close — a real INDEX-hygiene catch the skill surfaced on first run.

---

## Recommended sequencing

1. **Now (this pass):** `get-next` skill — *shipped* — plus this research doc.
2. **Next (small, high-leverage):** mirror **sub-issues + issue types** from `tracking-issues`
   (`sync-github.sh`) — turn the GitHub mirror into the epic hierarchy our markdown already encodes.
3. **Then (one new skill + one gate):** `security-reviewing` + a `govulncheck`/`gosec` CI job — the
   one clear capability gap from Part A.
4. **Fold-in enhancements (no new skills):** Playwright **visual-regression** baselines into
   `authoring-tests`/`governing-qa-framework` (pixel-lock the new token/theme system); `benchstat`
   baseline-compare; a short fuzz-target reference into `hardening-tests`.
5. **Milestones + a Projects roadmap board** as the read-view over the mirror — once the hierarchy
   mirrors cleanly.
6. **Skip until the architecture warrants it:** chaos/stress (`toxiproxy`) and CI latency-SLOs —
   premature for a single-binary local app; revisit if it goes multi-service.

**Out of scope deliberately:** moving the issue source of truth to GitHub (violates one-fact-one-home);
the external doc's full 5-skill expansion (most already exists here); time-boxed roadmap dates
(they rot — sequence by dependency/value, as `NEXT_FEATURES` already does).

---

*Sources:* external skills research (`dotts-h.github.io/claude-skills/research.html`);
GitHub Issues/Projects GA 2026 — sub-issues, issue types, advanced search, 50k-item Projects
([GitHub blog: sub-issues](https://github.blog/engineering/architecture-optimization/introducing-sub-issues-enhancing-issue-management-on-github/),
[Best practices for Projects](https://docs.github.com/en/issues/planning-and-tracking-with-projects/learning-about-projects/best-practices-for-projects)).
Grounded in this repo's `CONVENTIONS.md`, `SKILLS_PLAN.md`, `NEXT_FEATURES.md`, `docs/issues/`, and RETROS 0002.
