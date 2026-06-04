# Planner / generator / healer agent setup

Modern Playwright ships agent definitions that let an LLM plan, generate, and heal tests. Keeping them
installed and current is part of framework health (the *use* of them to write tests is `authoring-tests`).

## Install / refresh
```bash
cd e2e && npx playwright init-agents --loop=claude
```
This writes agent definitions (collections of instructions + MCP tools). **Regenerate after every
Playwright upgrade** so the agents get new tools/instructions — stale definitions silently lose capability.

## Convention-based layout the agents expect
- `specs/` — human-readable test plans (planner output).
- `tests/seed.spec.ts` — the seed test that bootstraps the app to the common start state.
- `tests/` — generated test files (generator output).
- agent definition files — regenerated with Playwright; don't hand-edit.

## Health checks
- The seed test exists and actually lands the app ready (navigation + any demo seeding).
- `init-agents` has been run for this repo (agent files present).
- The installed Playwright version matches between `e2e/package.json` and the CI browser-install step.

## Why keep them current
The agent loop is what makes browser-test authoring cheap and the suite self-healing through UI churn.
Letting the definitions go stale quietly removes that leverage — newcomers fall back to brittle hand-written
selectors without knowing the better path existed.
