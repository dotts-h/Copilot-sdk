---
id: 0095
title: "Epic: Active cost governance — run-grain budget enforcement + anomaly signal + spend digest (roadmap v16)"
status: open
severity: high
group:
depends_on: []
github:
links:
  adr: [0053]
  prs: []
  issues: [0096, 0097, 0098]
  regression:
assets: []
---

## Charter

The meter **sees** spend but cannot **stop** it at run grain. The repo already enforces
pre-Send *admission control* at two grains — the account-wide hard cap (ADR-0008,
`s.hardCap`) and the per-persona budget leash (ADR-0042, issue 0072, `leashFor` →
`pendingLeashGate` → `budgetGate`) — but a workflow **run** fans out across lanes and
sub-agents (`launchLanes` → `startLane`, summed into `RunRecord.Credits`) with **no
cumulative ceiling**. A looping or wide run, and especially an **unattended / queued** run,
can burn without limit.

The roadmap-v16 deep-research pass (five parallel angles, load-bearing claims adversarially
verified — see `docs/NEXT_FEATURES.md` "Roadmap v16") found this is the converged 2025–2026
failure mode: monthly/account caps **structurally cannot** stop a run-grain runaway (the
verified $47K-in-11-days incident had logging and monitoring but "no hard limit, no
per-conversation budget"), and the settled fix is **admission control** — a gate that runs
*before* the next model call, reads the running cost, and refuses once the cap would be
crossed (Portkey HTTP 412, LiteLLM `max_budget`, OpenRouter key limits — all pre-call,
run/key-scoped). FinOps-for-AI guidance pairs that hard cap with **high-frequency anomaly
detection** and a **scheduled digest** so slow burns surface without watching live.

This epic is the repo's own established move — it **mirrors the 0072 sub-agent leash at run
grain** (the next attribution level up) and is the lowest-risk highest-value slice in the
v16 candidate set: a deterministic, side-effect-free `leash.Breached(...)` decision layered
over the existing dispatch, reusing `telemetry.Leash` and the `budgetGate` pause with **zero
new infra**. It is also the **safety precondition for v17** (durable / unattended runs): a
run nobody is watching must not burn past its ceiling before that direction is built.

**Out of scope (research-rejected / deferred):** account-level monthly caps (already exist;
they don't stop run-grain runaways — that's the gap); a forward per-call cost *estimate* gate
(realized cumulative-vs-cap between lanes is simpler and sufficient at run grain — ADR-0053);
heavy gateway infra (Portkey/LiteLLM-style proxy — over-engineering for a single-user
local-first tool); per-workflow cap overrides (a clean additive follow-up on the workflow
forge entity, not P1).

## Children

- **0096 — P1 (BUILD FIRST, ADR-0053):** per-run budget cap + pre-Send admission control —
  mirror `telemetry.Leash` at run grain, gate at `startLane`, reuse `budgetGate`
  (interactive proceed/raise/cancel; unattended → deterministic cancel).
- **0097 — P2:** cost anomaly signal — a pure `DetectAnomalies` reader over the run/step
  credits (cost-per-step or burn-rate jump beyond a threshold; FinOps <2 / 2–7 / >7% bands as
  inspiration), surfaced ambered on the run inspector + Telemetry. No enforcement — a reader.
- **0098 — P3:** scheduled spend digest — a periodic rollup of spend + cap-hits + anomalies so
  slow burns surface without live watching. On its merge the epic closes.

## Acceptance

- [ ] A run carries an optional credit ceiling; cumulative run spend is checked **before** the
      next lane is admitted, and a breach pauses (interactive) or cancels (unattended) the run.
- [ ] The streaming / record path is byte-identical when no cap is set (`RunCapCredits == 0`).
- [ ] An anomaly reader flags an out-of-band run/step and surfaces it without enforcing.
- [ ] A spend digest rolls up the period's spend, cap-hits, and anomalies.
- [ ] Each child: `make lint && make test` green (coverage ≥ floor), `make e2e` when the UI
      changed; the close-out + any ADR ride the child's branch (ADR-0004).
