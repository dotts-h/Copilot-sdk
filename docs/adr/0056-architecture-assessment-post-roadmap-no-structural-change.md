# 0056. Post-roadmap architecture assessment — sound, no structural change

- Status: accepted
- Date: 2026-06-13
- Deciders: Horia
- Related: [ADR-0029](0029-hooks-forge-entity-bridge-enforced-allow-deny-ask-safe-read-defaults.md)
  (the `copilot → ctxforge` edge), [ADR-0033](0033-reportedaiu-is-source-of-truth-for-actual-spend-price-book-is-the-estimate.md)
  / [ADR-0048](0048-per-run-event-log-for-replay-vs-summary.md) (the `AppendOnlyStore[T]` IO edge),
  epic [0086](../issues/0086-epic-code-health-followups.md) (the god-file split this builds on),
  [ARCHITECTURE.md](../ARCHITECTURE.md), [CONVENTIONS.md](../CONVENTIONS.md) (the pure-core/thin-edges + seam doctrine)

## Context

With the promoted roadmap fully shipped (every epic and child issue closed through
roadmap v16 / epic 0095), a milestone architecture review was run before opening new
work: assess module boundaries, coupling, dependency direction, and drift from the
stated design goals, and decide whether a structural change is warranted. Two parallel
package-level code-quality audits had already found the internals clean (no correctness
bugs, races, or determinism violations); this ADR records the **structural** verdict so
the conclusion — and the reasoning for *not* refactoring — is institutional memory rather
than a re-derivable assumption. The repo has precedent for recording a decision *not* to
act ([ADR-0050](0050-no-mini-rag-file-based-docs-and-agentic-search.md),
[ADR-0035](0035-no-live-price-book-refresh-catalog-has-no-pricing.md)).

The internal dependency graph (verified, acyclic):

```
pure leaves (no internal deps): config · convo · ctxforge · pause · telemetry
seam:                            copilot → ctxforge
edge:                            web → {config, convo, copilot, ctxforge, pause, telemetry}   (no SDK import)
wiring:                          bootstrap → {config, copilot, ctxforge, telemetry, web}
```

## Considered options

- **Invert the `copilot → ctxforge` edge** (define a parallel hook/policy type inside
  `copilot`, translate at the boundary). **Rejected.** The seam imports a tightly-scoped
  slice of `ctxforge` — `Hook`, `HookMatch`, `Evaluate`, `Decision`, `PostToolUseCommands`
  and the hook enums (the governance-policy type + its pure evaluator), not the whole forge
  model. The direction is *seam → pure domain*, and **no SDK type ever crosses into
  `ctxforge`** (it has zero internal imports and no SDK import). The doctrine's real
  invariant — "no SDK import escapes the seam" — holds. ADR-0029 already anticipated and
  blessed this exact edge ("the dependency is one-directional onto pure domain and does not
  violate seam purity… ctxforge stays SDK-free"). Inverting it would duplicate the domain
  model and add a translation layer for zero isolation gain — a net negative.

- **Split `web.Server` (~45 fields) into sub-structs.** **Rejected as speculative.** After
  the epic-0086 split the struct is *organized*, not *hidden*: fields are grouped (shared
  deps → stores → budget knobs → seams → per-session mutable state under `mu` → leash
  accounting → counters → SSE subs), each non-obvious one carrying an intent comment citing
  its ADR. The count reflects genuine single-user-session state, all guarded by one `mu`
  with a documented `forgeMu → s.mu` lock order. Splitting it would either **fragment the
  single mutex** (a correctness risk this repo explicitly guards against) or be a cosmetic
  field-grouping the comments already achieve — no isolation or testability gain. It fails
  the "clearly net-positive" bar.

- **No structural change; record the assessment.** **Chosen.** See below.

## Decision

**The architecture is sound; make no structural change.** The layering is honest and
acyclic, dependency direction is correct (edges depend on pure core, never the reverse),
the one seam→domain import is correct and ADR-0029-blessed, the IO edges are isolated
behind `AppendOnlyStore[T]` with pure readers, and determinism/atomic-write discipline are
intact. No layering violation exists to fix and no cross-cutting coupling to resolve. A
forced refactor here would be speculative and net-negative.

The one line-level architectural cleanup that *was* warranted — folding the hand-rolled
Workflow CRUD handlers onto the shared `forgeCRUD[T]` generic — shipped in the
code-quality pass that preceded this assessment (PR #173), not as a separate structural
change.

## Consequences

- The assessment is recorded; a future session does not re-litigate "should we split
  `Server`?" or "should we invert `copilot → ctxforge`?" without new evidence — the answers
  and their rationale live here.
- **Watch-items** (tripwires, not debt):
  - **`web.Server` field growth.** It is a coherent aggregate *today*. If a future feature
    adds state that is **not** per-session-under-`mu` (e.g. a second independently-locked
    subsystem), that is the signal to extract it into its own type with its own lock —
    crossing the single-mutex assumption, not the field count, is the real trigger.
  - **The `copilot → ctxforge` edge must stay one-directional.** If `ctxforge` ever needs to
    import `copilot` (or any SDK type), the governance types should move to a shared pure
    package instead — never create a cycle.
- This ADR is a milestone marker: it certifies the architecture as sound at the close of the
  v16 roadmap, the baseline the next epic (the unscoped roadmap-v17 "durable autopilot"
  candidate in NEXT_FEATURES) will build on.
