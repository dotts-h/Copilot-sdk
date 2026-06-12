# Retrospectives

Process learnings, numbered and dated — the counterpart to [REGRESSIONS.md](../REGRESSIONS.md)
(which logs *bug* learnings) and [ADRs](../adr/) (which log *decisions*). A retro
reviews how a stretch of work actually went — what worked, what cost too much,
how skills/tools were used — and turns the lessons into **action items** that
change CONVENTIONS, tooling, or the docs system so the gains compound.

The carry-forward session-optimization lessons live in **one** place — CONVENTIONS
"[Session playbook](../CONVENTIONS.md)" (consolidated from the per-retro checklists in
RETROS 0007). A retro edits that list and carries only the **delta** here, and ends with a
one-line **session-cost** figure (the in-repo equivalent of a context-optimizer plugin —
ADR-0051).

| # | Title | Date |
|---|-------|------|
| [0001](0001-quality-and-architecture-hardening.md) | Quality & architecture hardening (v1 feature run → roadmap-v2 research → hardening PRs) | 2026-06-06 |
| [0002](0002-v28-postooluse-governance-close.md) | V28 PostToolUse command execution & closing epic 0052 (base-verification + MCP-verbosity + wait-pattern learnings) | 2026-06-09 |
| [0003](0003-get-next-script-resolution-and-pillar-guessing.md) | get-next stumbled twice before W1 shipped clean (script-path resolution + plumbing-is-not-intent learnings) | 2026-06-09 |
| [0004](0004-get-next-remote-env-branch-and-ci-merge.md) | get-next end-to-end in the web env — R6/epic 0076 closed (hard push-target branch + no-`gh` CI-poll-and-merge learnings) | 2026-06-11 |
| [0005](0005-deep-quality-architecture-and-test-hardening.md) | Deep quality, architecture & test hardening over the v0.2→v0.7 arc (three scoped audits → behavior-preserving fixes; `editConfig` drift, `opacity`-contrast, dark-branch + shutdown-path test gaps) | 2026-06-11 |
| [0006](0006-epic-0086-closeout-and-docs-system-review.md) | Epic 0086 close-out + docs-system review (epic-body close-out drift ×2, ADR-index regen discipline, no-mini-RAG decision → ADR-0050) | 2026-06-12 |
| [0007](0007-loop-engineering-and-efficiency-plugins.md) | Loop engineering + the efficiency-plugin wave (no third-party plugins → ADR-0051; four retro checklists consolidated into one CONVENTIONS "Session playbook" + a session-cost line) | 2026-06-12 |
