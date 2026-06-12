# Binding: api

> An adapter doc, not a recipe. Copy it to `docs/bindings/api.md` in the target
> repo and fill the backfill slots. It binds the open slots Layer 0/1 leave for
> an API/service repo: schema discipline, contract tests, and versioning.

## Schema discipline

- **The schema is the contract's source of truth** — OpenAPI/JSON Schema/proto,
  checked into the repo, regenerated or hand-edited *deliberately* (a schema
  diff in a PR is a headline, not a footnote).
- Request/response models in code are generated from (or validated against) the
  schema in CI — drift between code and schema is a failing gate, not a doc bug.
- Every **breaking** schema change bumps the contract's version id (the
  `-v1`/`-v2` suffix in `docs/CONTRACTS.md`) and keeps the old version serving
  until consumers confirm migration (fleet-doctor shows who they are).

## Contract tests

- Each **Provides** entry in `docs/CONTRACTS.md` has a contract test: a
  deterministic check that the served shape matches the schema (provider-side),
  runnable in CI without external dependencies.
- Each **Consumes** entry has a consumer-side test pinned to the provider's
  schema version — an upgrade is a deliberate diff to that pin.
- Error shapes are part of the contract: status codes, error body schema, and
  at least one negative-path contract test per endpoint family.

## Gates (fills the quality recipe's slots)

- `lint` includes a schema lint/validate step (the schema file itself can't be
  malformed on the default branch).
- Integration tests run against an in-process or containerized instance — no
  test reaches a shared/staging environment from CI.

## Versioning & releases

- The release recipe's tag drives the published artifact/image version; the API
  version in the schema is independent and only moves on contract changes.

## Backfill checklist (flagged by adopt-recipes)

- [ ] Point this doc at the repo's actual schema file(s).
- [ ] Add the schema-validate step to the lint gate.
- [ ] Register the served API as a **Provides** entry with its version id.
