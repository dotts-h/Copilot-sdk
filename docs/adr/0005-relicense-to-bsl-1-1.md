# 0005. Relicense from Apache 2.0 to Business Source License 1.1

- Status: accepted
- Date: 2026-06-04
- Deciders: Horia
- Related: `LICENSE`, `README.md`, dependency `github.com/github/copilot-sdk/go` (MIT)

## Context

The project shipped under Apache License 2.0 from its initial commit (2026-06-02)
on a public repository. Apache 2.0 is permissive: it grants everyone the right to
use, modify, and **sell** the work, with no obligation to contribute changes back.
That is the wrong default for a project we may want to monetize later — a third
party could fork it and offer a competing commercial product legally.

This is a **protective** move, not the start of active selling. Facts that shape it:

- **Copyright ownership is clean.** The only human author is Horia (two git
  identities, same person); other commits are AI-generated output (not
  independently copyrightable) and bot dependency bumps (no creative authorship).
  A solo owner is free to relicense future versions.
- **The runtime model is bring-your-own-Copilot.** The app is a wrapper over the
  GitHub Copilot SDK; every user supplies their own GitHub Copilot entitlement.
  We are **not** reselling or proxying the Copilot service.
- **The SDK dependency is MIT** (`github/copilot-sdk/go`), which permits use,
  modification, and sale — so the dependency imposes no obstacle to a restrictive
  license on our own code.

## Considered options

- **Keep Apache 2.0 (open-core).** Core stays forkable; monetize a Pro tier or
  hosting later. Rejected for now: leaves the core legally resellable by anyone,
  which is exactly the exposure we want to close.
- **Proprietary / closed EULA.** Maximum protection. Rejected: stops publishing
  source, loses the open-source transparency and goodwill of a public repo, and is
  heavier than needed for a pre-revenue project.
- **Business Source License 1.1 (source-available).** Source stays public and
  readable; commercial/competing use is restricted; each version auto-converts to a
  permissive Change License after the Change Date. Chosen.

## Decision

Relicense to **BSL 1.1** with these parameters:

- **Licensor:** Horia C. Rădulescu
- **Licensed Work:** my-orchestra — wrapper over the GitHub Copilot SDK
- **Additional Use Grant:** personal or internal individual use, including
  production, is permitted; offering the work (or a derivative) to third parties as
  a hosted or packaged commercial product or service is not
- **Change Date:** 2030-06-04 (four-year window)
- **Change License:** Apache License, Version 2.0

## Consequences

- Going forward, the work is source-available and protected against commercial
  forks; ordinary users with their own Copilot subscription can still use it freely,
  including in production.
- **The Apache-2.0 history is not retroactively revoked.** Commits already published
  under Apache 2.0 remain available under that license for those snapshots. For a
  young, unwatched repo the practical leakage is negligible, but it is true and
  intentional to record here.
- BSL is **not** an OSI-approved open-source license; the repo should not be
  described as "open source" — use "source-available."
- Each released version carries its own Change Date; on that date the version
  becomes Apache 2.0. The `Change Date` in `LICENSE` is updated per release line.
- No impact on dependencies (MIT SDK) or on the bring-your-own-Copilot runtime
  model; this changes only the terms under which our own source is distributed.
