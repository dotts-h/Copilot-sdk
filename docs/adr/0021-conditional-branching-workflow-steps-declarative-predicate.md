# 0021. Conditional / branching workflow steps — a declarative step predicate

- Status: accepted
- Date: 2026-06-06
- Deciders: Horia
- Related: extends [ADR-0013](0013-multi-agent-workflow-run-handoff-surface.md)
  (multi-agent workflow run / handoff surface) and
  [ADR-0017](0017-per-lane-tool-and-permission-surface-for-parallel-workflow-lanes.md)
  (per-lane surface); `internal/ctxforge` (`workflow.go` — `WorkflowStep.When`,
  `StepCondition`, `Validate`, `CompileWorkflow`), `internal/web` (`workflow.go` —
  the pure `workflowRun` engine: `start`/`finishLane`/`failLane`/`advance`/
  `evalPending`/`evalWhen`, `laneSkipped` status, `renderLanes`/`laneGlyph`, the
  steps textarea round-trip), `internal/bootstrap` (`SeedForge` branching demo),
  `e2e/tests/e2e.spec.ts`; `docs/NEXT_FEATURES.md` item B2,
  [issue 0020](../issues/0020-conditional-branching-workflow-steps.md),
  [ADR-0001](0001-render-markdown-server-side-for-committed-agent-turns.md)

## Context

ADR-0013 shipped `Workflow` as a fixed pipe: a sequential handoff (each lane's
output feeds the next) or a parallel fan-out (every lane at once). Every step
always runs. That is fan-out / hand-off, not yet *control flow*: there is no way
to say *"if the review lane flags issues, run the fix agent — otherwise skip it."*
B2 (`docs/NEXT_FEATURES.md` Tier B) is the first genuinely orchestration-shaped
capability beyond fan-out/handoff: a step **gated on a prior lane's outcome**.

The roadmap is explicit that this **needs an ADR** for the predicate model, and
that the run engine (`internal/web` `workflowRun`) and `CompileWorkflow` must stay
pure additions. Three questions had to be answered: **what the predicate model is**
(and where it lives / how it Validates), **how an unsatisfied step behaves**, and
**how a gated step fits the existing sequential + parallel engines** without
breaking either.

## Considered options

- **The predicate model.**
  - **A small declarative enum on `WorkflowStep` (chosen).** A nullable
    `When *StepCondition` where `StepCondition` is
    `{Step int, Condition string, Value string}`: `Step` is the **1-based index of
    a prior step** the predicate reads, `Condition` ∈
    {`succeeded`, `failed`, `output-contains`, `always`}, and `Value` is the
    substring for `output-contains`. It is pure data — file-backed JSON like the
    rest of the forge, `Validate`-able without a forge or a client, and predicate
    evaluation is a **pure function over prior lanes' settled outcomes**
    (deterministic, no expression engine). `When == nil` (the on-disk default) means
    *always runs* — so every pre-B2 workflow reads and behaves exactly as before.
  - *A free-form expression / CEL-style condition.* Rejected — it pulls a parser /
    eval dependency into a zero-dep, single-user localhost tool (against the grain
    that keeps `ctxforge` pure, cf. the markdown-renderer dead-end in REGRESSIONS),
    is hard to validate statically, and is far more power than "gate on a prior
    lane's outcome" needs.
  - *Reuse the existing `Mode` instead of a per-step predicate.* Rejected — `Mode`
    is sequential/parallel for the **whole** run; branching is inherently
    **per-step** gating. They compose (a gated step works in either mode); they
    don't substitute.

- **Referencing the gated-on step: by index vs. by a new step id.**
  - **By a 1-based prior-step index (chosen).** `WorkflowStep` has no id today and
    the steps textarea is `agentID: prompt` per line; a positional reference needs
    no new identifier and the UI already prints "step N". Crucially, **a reference
    that must point strictly backward (`Step < this step's position`) makes a cycle
    structurally impossible** — acyclicity is a `Validate` arithmetic check, not a
    graph walk, and the run is guaranteed to terminate.
  - *Add a `WorkflowStep.ID` and reference by id.* Rejected for this cut — it adds
    an identifier and a uniqueness rule for no gain over the index at current scale,
    and complicates the one-line-per-step textarea. (Revisit if steps ever need to
    be reordered without rewriting predicates.)

- **What an unsatisfied step does.**
  - **It is *skipped* — a distinct, terminal lane status (chosen).** An unsatisfied
    gated step becomes `laneSkipped` (rendered distinctly: a `⊘` glyph, a
    `lane-skipped` state, a reason in the lane detail), **not** failed. Skipping is a
    *settle* event: `allSettled` counts a skipped lane as settled, so a run with a
    skipped lane still terminates, and a sequential handoff steps over it. A skip is
    a normal branch outcome, not an error — conflating it with failure would abort a
    sequential run (see below) and mis-colour the lane.
  - *Treat an unsatisfied step as failed.* Rejected — it would abort a sequential
    run and report a red error for what is the *expected* "don't run this branch"
    outcome.

- **How a gated step fits both engines.**
  - **A pure `advance`/`evalPending` over settled lanes, evaluated lazily
    (chosen).** *Sequential:* when a lane settles, walk forward to the next step;
    evaluate its `When` against the (already-settled) prior lanes — run it if
    satisfied, else mark it skipped and keep walking. *Parallel:* `start` launches
    every **ungated** step (a `When` that is nil/`always`, or whose dependency has
    already settled); each time a lane settles, re-scan pending lanes whose
    dependency is now settled and run-or-skip them to a fixpoint (so a skip cascades
    correctly). Both reduce to one `evalWhen(lane) (satisfied, ready bool)` helper.
    A hard **failure** keeps its ADR-0013 semantics — a sequential run still aborts
    on a failed step (REGRESSIONS "a workflow run owns the turn"); a parallel run
    lets survivors finish *and* may now unblock a `When: failed` gated lane.
  - *A separate branching executor / DAG scheduler.* Rejected — over-engineered; the
    existing run is already a small pure state machine, and lazy evaluation over the
    two existing modes is a strictly additive set of methods.

## Decision

Add `ctxforge.StepCondition {Step, Condition, Value}` and a nullable
`WorkflowStep.When *StepCondition` (`omitempty`, so older `forge.json` reads
clean). `Workflow.Validate` checks each non-nil `When`: a known `Condition`; for
`succeeded`/`failed`/`output-contains` a `Step` in `[1, i]` (a strictly-prior step
— forbidding self/forward references, which guarantees acyclicity); a non-empty
`Value` for `output-contains`. `CompileWorkflow` carries `When` into `CompiledStep`
unchanged. The `When` reference is **within-workflow** (a step index), so it is
validated by `Workflow.Validate` itself; step→agent referential integrity stays the
whole-forge check from ADR-0013.

In the web layer, the pure `workflowRun` engine gains a `laneSkipped` status and
three pure methods — `evalWhen` (the predicate over settled lanes), `advance`
(sequential: walk forward, run-or-skip), and `evalPending` (parallel: run-or-skip
pending lanes to a fixpoint). `finishLane` and `failLane` route through them and
both now return the lane indices to launch next (`failLane`'s signature changes
from `bool` to `[]int` so a parallel failure can unblock a `When: failed` lane);
the Server adapter launches whatever they return, exactly as it already did for
`finishLane`. `renderLanes`/`laneGlyph` render a skipped lane distinctly and surface
the skip reason. The steps textarea round-trips a predicate as an optional
`[step N <condition> <value>]` prefix on a step line, so editing a branching
workflow in the form preserves its `When` (parsed/formatted purely, unit-tested).
The offline demo seeds a sequential **branching** workflow whose first lane's
output gates a second lane (which **runs**) and a third lane (which **skips**), so
the browser suite drives a real branch and asserts the skipped lane's state — never
timing.

## Consequences

- Positive: `Workflow` moves from a fixed pipe to real control flow with **no new
  dependency** and **no new seam method**. The predicate is pure data, statically
  validated (including acyclicity, for free, from the strictly-backward index), and
  evaluated by a pure function over settled lanes — so the whole branching engine is
  unit-tested with no browser and no client, like the rest of `workflowRun`. Skip is
  a first-class, terminal lane outcome that still lets a run terminate.
- Backward-compatible: `When` is `omitempty` and nil means *always*, so every
  pre-B2 workflow loads and runs identically; a v1/pre-B2 reader ignores the new key
  (CONTRACTS §4 additive-schema rule). No migration needed.
- Escaping (ADR-0001 held): a skipped lane's reason/detail and any predicate `Value`
  are forge-originated text and reach the browser through the same `richtext` /
  `html/template` auto-escaping as agent names and lane detail.
- Failure semantics preserved (REGRESSIONS): a sequential run still aborts on a hard
  step **failure** (distinct from a **skip**), so `failed` predicates are evaluated
  in the parallel/fan-out context where survivors run on; `output-contains` /
  `succeeded` are the sequential branching primitives. `failLane` returning `[]int`
  is an internal engine-signature change (the pure engine + its callers), not a
  contract on the wire.
- Trade-off accepted: a predicate references a prior step **by position**; reordering
  steps means re-checking predicates (a `WorkflowStep.ID` is deferred until reorder
  pressure appears). The condition vocabulary is a fixed four — richer predicates
  (numeric thresholds, regex, AND/OR) are out of scope for this cut and would be a
  follow-up, deliberately not an expression engine.
- Contract change: `ctxforge.WorkflowStep.When` / `StepCondition` is an additive
  persisted-schema field (CONTRACTS §4) and `laneSkipped` is a new rendered lane
  state — recorded in CONTRACTS and ARCHITECTURE; guarded in REGRESSIONS.
</content>
</invoke>
