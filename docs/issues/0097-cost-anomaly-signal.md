---
id: 0097
title: "Cost anomaly signal — high-frequency DetectAnomalies reader over run/step credits (P2)"
status: open
severity: medium
group: 0095
depends_on: [0096]
github:
links:
  adr:
  prs: []
  issues: [0095]
  regression:
assets: []
---

## Summary

A **pure reader** that flags out-of-band spend so a slow burn surfaces without watching live.
No enforcement — the dual of the P1 cap: the cap *stops* a runaway, the anomaly signal
*notices* an unusual one. Over the run records (and the per-step credits from issue 0092), a
`DetectAnomalies`-style function flags a run or step whose **cost-per-step** or **burn-rate**
jumps beyond a threshold (the FinOps-for-AI maturity bands — <2% / 2–7% / >7% of spend hit by
anomalies — as inspiration for the threshold framing, not a hard spec). The signal is
**ambered** on the run inspector and the Telemetry page, the same discipline as the
reconcile-drift amber.

## Why now

The roadmap-v16 research (FinOps Foundation, State of FinOps 2025) found that the converged
control set is **cap + anomaly detection + digest**, and that real-time anomaly *signal* (not
just monthly reconciliation) is what catches the slow burns a hard cap's binary threshold
misses. It's a pure reader over data the repo already persists (spend records + O2 step
credits), so it carries near-zero correctness risk.

## Scope

- A pure `DetectAnomalies(records, opts) []Anomaly` reader (no I/O), table-tested.
- A surface: ambered rows/badges on the run inspector + Telemetry (read-only).

## Out of scope

- Enforcement (that's the P1 cap); ML/statistical anomaly models (a threshold + ratio reader
  is the minimum-viable form for a single-user tool — the research found enforcement and
  detection *frequency* are unstandardized, so we ship a deterministic threshold, not a model);
  alerting transport (the digest, 0098, is the delivery).

## Acceptance

- [ ] `DetectAnomalies` flags an out-of-band run/step deterministically and is table-tested.
- [ ] The signal surfaces ambered on the inspector + Telemetry, read-only.
- [ ] `make lint && make test` green (coverage ≥ floor); `make e2e` if the UI changed.
