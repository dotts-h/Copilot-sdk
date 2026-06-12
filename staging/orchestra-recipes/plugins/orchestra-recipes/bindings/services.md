# Binding: services

> An adapter doc, not a recipe — deliberately thin. Copy it to
> `docs/bindings/services.md` in the target repo. It binds the open slots for a
> repo that is one service among several (workers, functions, small backends).

## Inter-service contracts (the spine)

- Every queue message, event, RPC, or shared-store schema this service emits or
  consumes is a row in `docs/CONTRACTS.md` — **Provides** for what it emits/
  serves, **Consumes** for what it depends on. The fleet doctor (contracts
  recipe, `fleet_member=true`) is the cross-check; keep `constellation.yaml`
  listing the real sibling checkouts.
- Contract ids carry a version suffix (`-v1`); breaking a message shape means a
  new id served alongside the old until consumers move.
- **Idempotency and retry semantics are part of the contract.** State "at least
  once" or "exactly once" per consumed message type in the registry entry —
  assumptions here are where fleets break quietly.

## Operational baseline

- The service starts, passes health checks, and shuts down cleanly from one
  documented command (Environment facts in CONVENTIONS).
- Config comes from the environment; secrets are referenced (`${VAR}`), never
  committed — the lint gate's secret scan enforces the letter, this rule the spirit.

## Deferred (recorded so it isn't re-litigated)

- **Chaos/fault-injection testing is deliberately out of scope** for this
  binding's first version. When the fleet has >3 services and a real incident
  history, revisit: start with dependency-timeout injection in contract tests,
  not a chaos platform. Record the revisit trigger in TECH_DEBT (tier L) or the
  Decisions section (tier S).

## Backfill checklist (flagged by adopt-recipes)

- [ ] Register every emitted/consumed message or endpoint in CONTRACTS.md.
- [ ] State retry/idempotency semantics per consumed contract.
- [ ] Confirm constellation.yaml lists the real sibling repos.
