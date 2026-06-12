# DESIGN — why the recipe system is built this way

> Condensed rationale. The schemas live in [SPEC.md](SPEC.md); this is the
> *why*, extracted and generalized from a mature project's process stack
> (thin-pointer CLAUDE.md, constitution + glossary docs, self-enforcing CI
> guards, an engineered dev loop, hybrid issues, verified releases).

## Doc-based software

The processes that make agent-driven repos work are mostly *documents and
small scripts* — and prose-only conventions drift, don't travel, and get
re-derived (wrong) per project. Treating them as **software** — versioned
packages with manifests, parameters, installers, conformance checks, and
evals — is the difference between "we have good habits" and "good habits are
the default state of every repo we touch."

## Recipes vs. skills

A skill improves *one session* on *one host*; it leaves no trace. A recipe
installs **durable state into the repo**: any agent (any vendor — hence the
dual `CLAUDE.md`/`AGENTS.md` thin pointers), and any human, gets the process
with zero plugins installed. The skills in this plugin are only the operators:
adopt, update, doctor.

## Layering

- **Layer 0 (`core`)** is the part every repo needs and nothing depends on:
  pointers, constitution (with the five-point doctrine), glossary, lock.
- **Layer 1** packages are separable practices (`quality`, `loop`, `issues`,
  `release`, `contracts`) with explicit `requires.recipes` edges — a library
  doesn't need the issue loop wired the way an app does, and the dependency
  graph (not a monolith) is what lets profiles mix them.
- **Layer 2 (bindings)** are *adapter docs*, deliberately not recipes: the
  frontend/api/qa/services specifics are conventions to read, not machinery to
  install, and keeping them as ~100-line docs avoids a recipe-per-stack
  explosion. They fill the named slots Layers 0/1 leave open (npm gate
  commands, schema discipline, locator rules, inter-service contracts).

## Tiers — process must fit the repo

The biggest failure mode of process kits is uniform weight: a 4-file lambda
repo with an ADR log and a RETROS directory is theater, and theater rots
trust in the whole system. Tier S is *genuinely* lean (~4 files; decisions
inline in CONVENTIONS until ~5, then graduate); M adds the memory structures
(ADRs, regressions, issues); L adds the navigation/observability layer
(generated CODEMAP, RETROS, TECH_DEBT) that only pays for itself at size.

## Harvest mode — brownfield is the common case

Most repos already have *some* process (a CONTRIBUTING, Makefile targets, CI).
Overwriting it would destroy exactly the information the system exists to
preserve. So adoption **harvests**: the constitution is generated from the
repo's evidenced facts; existing files are merged into, never clobbered (the
installer hard-codes this); and what the recipes expect but the repo can't
evidence becomes a *flagged backfill checklist*, not a fabricated fact.

## Lock + 3-way merge — the update model (borrowed from Copier)

Template systems that can't update become abandonware in the repos they
seeded. The lock records `(version, tier, answers)` per recipe — enough to
re-render the **old** template and diff three texts: old template (what we
gave you), repo file (what you made of it), new template (what we'd give you
now). Local adaptations are preserved by construction; upstream improvements
apply; collisions escalate to a human in a PR. Corollaries: the repo's copy is
canonical-for-that-repo, doctors flag *missing shape* rather than forbidding
divergence, and updates never auto-overwrite.

## The contracts spine — fleets of small repos

The mini-repo profiles only work if the *relationships* between repos are
machine-checked, because nobody re-reads four small repos before changing one.
`CONTRACTS.md` gets explicit **Provides/Consumes** sections with fleet-unique
ids; `constellation.yaml` maps the sibling checkouts; `fleet-doctor.sh`
cross-checks every consumer against exactly one provider. The registry stays
prose-friendly (an index, not a schema dump) while the ids give the doctor
something deterministic to verify.

## Model portability

Recipes assume nothing about which model operates the repo. The design rules
that make that true:

- **Determinism lives in scripts** (installer, doctors, pickers, guards) —
  outcomes that must not vary with the operator are code, not judgment.
- **Judgment steps are checklists with testable termination** (gates pass, CI
  green, doctor exits 0) — a smaller model can follow them; a larger one just
  follows them faster.
- **`min_model` per workflow step** records the floor the author certifies
  (haiku-class for retrieval/doctor reading, sonnet-class for installs and
  mechanical merges, opus-class for harvesting and registry curation). It is a
  *certification placeholder*: the evals are the eventual proof, and the
  spend-routing rule it encodes — cheap models for retrieval, strong models
  for judgment — comes straight from the source project's playbook.

## Enforce with hooks, not memory

Every invariant that has ever burned the source project is encoded as a
deterministic check rather than a remembered rule: the workflow guard (CI
double-runs, release mis-tagging), the doctors (presence/shape of the whole
stack), the repo's own lint (parse every script, no secrets, no literal
dates). The recipes don't just *say* "enforce with hooks" — they are built out
of the same principle, applied to themselves.
