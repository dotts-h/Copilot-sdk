# Contributing to my-orchestra

Thanks for helping! This project is built **test-first** and kept lint-clean.

## Workflow

1. Branch from `main`.
2. Write a failing test, then the code to pass it. Keep changes small.
3. Before pushing:
   ```bash
   make lint    # gofmt + vet + golangci-lint (v2)
   make test    # race + coverage
   ```
4. Open a PR. CI must be green (lint, race tests + coverage floor, fuzz, build matrix).

## Conventions

- **No SDK imports in the TUI.** The UI depends only on `copilot.Client`. New
  runtime behavior goes in `SDKClient`; new UI behavior is tested via `MockClient`.
- **Domain logic stays pure.** `telemetry`, `ctxforge`, and `config` are
  dependency-free and fully unit-tested. Add table-driven tests for edge cases.
- **Determinism.** `Forge.Compile` and pricing must be deterministic; add a test
  if you touch ordering or rates.
- **Atomic persistence.** Config/forge writes go through temp-file + rename and
  validate before saving.

## Telemetry rates

Default model rates in `internal/telemetry/pricing.go` approximate GitHub's
published pricing and are user-overridable from settings. If GitHub changes
pricing, update `DefaultPriceBook` and the corresponding tests.

## Adding a page

1. Add a `Page` constant + `String()` case in `internal/tui/model.go`.
2. Add a `view*` method in `internal/tui/views.go` and wire it in `renderBody`.
3. Add key handling if interactive, and a `View` smoke test
   (`TestViewRendersWithoutPanicAllPages` already iterates all pages).
