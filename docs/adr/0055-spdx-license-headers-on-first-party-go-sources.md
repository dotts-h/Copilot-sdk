# 0055. SPDX license headers on first-party Go sources (CI-enforced)

- Status: accepted
- Date: 2026-06-13
- Deciders: Horia
- Related: [ADR-0005](0005-relicense-to-bsl-1-1.md) (BSL relicense), `LICENSE`,
  `.golangci.yml`, [CONVENTIONS.md](../CONVENTIONS.md)

## Context

The project is licensed under BSL 1.1 (ADR-0005). The root `LICENSE` file already
satisfies BSL's mandatory covenant ("conspicuously display this License on each
original or modified copy"). What it does **not** do is make the license travel
with an individual file: a `.go` file copied out of the repo carries no license,
and license-scanning/SBOM tooling has nothing machine-readable to read per file.
Per-file SPDX headers are the standard hardening for a source-available repo, and
"missing license info in source headers" is a commonly cited BSL adoption gap.

A header that any contributor — human or coding agent — can silently delete is
worthless. Documentation alone does not prevent removal; only a gate does.

## Decision

Every **first-party** Go source file begins with this two-line header (followed by
a blank line, before any `//go:build` constraint or the package clause):

```go
// Copyright (c) <year> Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1
```

- **The identifier is `BUSL-1.1`** — the SPDX id for Business Source License 1.1.
  (Note: `BSL-1.0` is the *Boost* license; do not use it.)
- **Enforced in CI** via the `goheader` linter in `.golangci.yml`, which runs in
  both `make lint` and the CI lint job. A missing or malformed header fails the
  build, so the header cannot be stripped without turning CI red.
- The copyright line is matched as a **regexp** (the ASCII parts pinned, the
  accented surname left as `.+`) to sidestep a multibyte-rune comparison quirk in
  goheader's literal matcher; the **SPDX line is a strict literal** — it is the
  load-bearing part.
- **Scope:** first-party Go only. The vendored/third-party MIT SDK dependency
  (`github.com/github/copilot-sdk/go`) is untouched. The `desktop` build-tagged
  file carries the header too (placed above its `//go:build` line), even though
  golangci-lint skips tag-excluded files from analysis.
- **New files must include the header**; any future code generator must emit it.

## Consequences

- Removing or omitting a header on a first-party `.go` file is a CI failure
  (`goheader`), which is the point: it survives careless edits and agent rewrites.
- The header is additive metadata — no behavior change, no effect on the build,
  no new dependency (goheader ships with golangci-lint v2).
- Non-Go assets (scripts, templates, CSS) are **not** covered today; the root
  `LICENSE` still governs them. Extending headers to other file types is a future
  option, not done here.
- The copyright **year** is matched loosely (`\d{4}`); we don't churn years across
  files. The canonical terms and parameters live in `LICENSE` / ADR-0005 — the
  header points at the license, it doesn't restate it.
