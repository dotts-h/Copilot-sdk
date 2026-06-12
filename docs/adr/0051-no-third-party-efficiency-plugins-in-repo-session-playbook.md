# 0051. No third-party efficiency/observability plugins — an in-repo session playbook instead

- Status: accepted
- Date: 2026-06-12
- Deciders: Horia
- Related: [RETROS 0007](../RETROS/0007-loop-engineering-and-efficiency-plugins.md),
  [ADR-0050](0050-no-mini-rag-file-based-docs-and-agentic-search.md) (the sibling
  infra-adoption decision), [CONVENTIONS.md](../CONVENTIONS.md) "Session playbook",
  [ADR-0029](0029-hooks-forge-entity-bridge-enforced-allow-deny-ask-safe-read-defaults.md),
  [ADR-0030](0030-dangerous-action-deny-and-mandatory-hitl-unbypassable-by-config.md)

## Context

Two adjacent ideas are circulating in the Claude Code ecosystem (mid-2026): **loop
engineering** (designing the system around the agent — goal, tools, context management,
failure exits, feedback — rather than the single prompt) and a wave of **token-efficiency /
context-observability plugins** (context optimizers, terse-output instruction files,
MCP-schema trimmers, structured-process skill packs) claiming 30–70% token savings. The
question: adopt any of them here, and/or build something similar?

## Considered options

- **Install third-party efficiency/observability plugins from a marketplace.** Rejected. The
  benefit is largely unproven (the headline savings are vendor- or blogger-self-reported,
  mostly unreproduced) and the risk class is real and documented: a Feb-2026 Snyk audit of
  ~4,000 third-party marketplace skills found **13.4% with critical issues, 76 with
  confirmed malicious payloads**, 10.9% leaking secrets, 17.7% fetching untrusted external
  content. Skills/plugins run with the **full privileges of the Claude Code process**
  (filesystem, network, shell), and any URL-fetching plugin crosses a prompt-injection trust
  boundary. For a repo whose *product* is a governed agent harness (deny-wins hooks ADR-0029,
  unbypassable HITL ADR-0030), importing an unaudited privileged plugin to save tokens is
  self-contradictory.
- **Install only the read-only observability plugins (lower risk).** Rejected, narrowly. A
  read-only reporter can't hijack the loop, but it still runs with process privileges and
  adds a dependency — and **the value is the practice, not the binary**. We can capture the
  same signal (what a session cost, where it went) in-repo with zero trust boundary.
- **Build an in-repo equivalent (chosen).** The repo already *is* a loop-engineering case
  study — `get-next` is an engineered loop with a testable termination condition, the
  workflow guard / PostToolUse governance / budget leash are its guardrails, and the retros
  are its feedback mechanism. The two concrete, zero-risk improvements are: (1) consolidate
  the four scattered retro "session-optimization checklists" into one canonical **Session
  playbook** in CONVENTIONS (a "one fact, one home" fix — the doctrine was sprawling across
  four overlapping lists), and (2) add a **one-line session-cost figure** to each retro — the
  local, dependency-free equivalent of the context-optimizer plugins.

## Decision

No third-party efficiency/observability plugins (including read-only ones). The session
playbook lives in CONVENTIONS as the single carry-forward list; each retro carries only the
delta plus a one-line session-cost figure. Loop engineering is adopted as a **lens** on the
existing skills/scripts/hooks, not as new machinery.

## Revisit triggers

- A **first-party** (Anthropic-shipped) efficiency or observability feature lands — then it
  carries no third-party trust boundary and is evaluated on merit (as deferred-tool search
  already was).
- A specific plugin becomes **auditable and pinned** (vendored, source-reviewed in-repo,
  no URL fetching) *and* shows reproduced savings on our own sessions.
- Dev-session token cost becomes a measured pain (the session-cost line shows a trend worth
  tooling), justifying a small in-repo analyzer — still in-repo, still no marketplace.

## Consequences

- No new dependency, no privileged third-party code; the in-repo playbook + session-cost
  line are the whole mechanism.
- The retro template gains a required session-cost sentence (dogfoods the product's own
  spend-measurement discipline on our dev loop).
- The four historical retro checklists remain as dated snapshots; the canonical list is
  CONVENTIONS "Session playbook" going forward.
