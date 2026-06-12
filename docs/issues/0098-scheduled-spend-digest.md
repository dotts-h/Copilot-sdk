---
id: 0098
title: "Scheduled spend digest — periodic rollup of spend, cap-hits, and anomalies (P3)"
status: open
severity: low
group: 0095
depends_on: [0096, 0097]
github:
links:
  adr:
  prs: []
  issues: [0095]
  regression:
assets: []
---

## Summary

A **periodic rollup** so slow burns surface without live watching — the delivery layer for
the P1 cap and P2 anomaly signal. A digest summarizes a period's spend (by workflow / agent /
run), the cap-hits (P1), and the anomalies (P2) into one view (and/or a written artifact),
the FinOps-for-AI "scheduled digest" that pairs with usage limits and anomaly detection. On
this child's merge, epic 0095 closes.

## Why now

The research found the converged active-cost-governance control set is **cap + anomaly +
digest**; with P1 (cap) and P2 (anomaly signal) shipped, the digest is the cheap rollup that
turns point-in-time controls into a recurring summary — the last slice of the epic. It builds
purely on the spend/run stores and the P2 reader.

## Scope

- A pure digest builder over the spend/run records + P2 anomalies (no I/O), table-tested.
- A surface/artifact: a Telemetry digest view and/or a written summary file; a configurable
  period (the existing `?window=` discipline).

## Out of scope

- External transport (email/Slack — a single-user local-first tool surfaces in-app/on-disk);
  scheduling infra beyond the existing run loop (the digest is on-demand + on the established
  window selector, not a cron daemon — that's the v17 scheduling direction).

## Acceptance

- [ ] A pure digest builder rolls up spend + cap-hits + anomalies for a period, table-tested.
- [ ] A Telemetry digest surface and/or written artifact renders it.
- [ ] `make lint && make test` green (coverage ≥ floor); `make e2e` if the UI changed.
- [ ] Epic 0095 closed on merge (its row ticked, children all closed).
