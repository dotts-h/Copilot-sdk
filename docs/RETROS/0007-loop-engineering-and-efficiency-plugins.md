# Retro 0007 — loop engineering, the efficiency-plugin wave, and a playbook consolidation

- Date: 2026-06-12
- Scope: a research-and-maintenance session (no product code). Two ecosystem questions —
  **loop engineering** and the **token-efficiency/observability plugin** wave — researched
  to a decision ([ADR-0051](../adr/0051-no-third-party-efficiency-plugins-in-repo-session-playbook.md)),
  plus the consolidation that research exposed: four scattered retro checklists collapsed
  into one canonical **Session playbook** in CONVENTIONS, and a new session-cost retro line.

## What prompted it

A standing question — "should we adopt the efficiency plugins / loop-engineering tooling
everyone's talking about?" Researched both terms properly (web + grounding against this
repo) rather than from memory.

## What the research found

- **Loop engineering is mostly a new name for what this repo already does.** The term (the
  2026 successor to prompt/context engineering) means designing the *system around* the agent
  — goal with a testable termination condition, tools, context management, failure exits,
  feedback. Map it onto the repo: `get-next` is the engineered loop (verify base → pick →
  build test-first → PR → review → merge on green); the workflow guard / PostToolUse
  governance / budget leash (ADRs 0029–0032, 0042) are the guardrails; **the retros are the
  feedback mechanism**. RETROS 0003→0004 literally show the loop being debugged until it ran
  clean. So the value is a *lens* ("where does our loop still leak?"), not new machinery.
- **The efficiency plugins are a mixed bag, and the working mechanisms are first-party.**
  Four categories: read-only observability (honest, low-risk), terse-output instruction files
  (weak — the file costs input tokens every turn), MCP-schema trimmers (real, but now a
  first-party feature — this very session used deferred-tool search), and structured-process
  packs (claims are self-reported, mostly unreproduced). The mechanisms that demonstrably
  work — caching, scoped subagents, progressive disclosure, map-first reads — are already our
  doctrine. Our own measured datum beats any plugin README: RETROS 0005's three scoped audits
  at ~150K each vs 364K for 0001's unscoped pair (~60%), self-measured.
- **The risk is real, not hypothetical.** A Feb-2026 Snyk audit of ~4,000 marketplace skills:
  13.4% critical, 76 confirmed-malicious, secret leaks, untrusted-URL fetches. Plugins run
  with full process privileges. For a repo whose product is a *governed* harness, importing
  an unaudited privileged plugin to save tokens is self-contradictory. → **ADR-0051: no
  third-party plugins, build in-repo.**

## What we changed (the consolidation the research exposed)

The research surfaced a "one fact, one home" violation **in the doctrine that preaches one
fact, one home**: RETROS 0001/0002/0005/0006 had each appended a "Session-optimization
checklist (carry forward)" — **24 items across four lists, heavily duplicated** ("map-first
windowed reads" in three; "/code-review before push" in three; base-verification restated).
A new session was told to carry forward four overlapping lists; nobody can.

- **One canonical home:** CONVENTIONS gained a **Session playbook** section — the deduped
  union, grouped (Discovery & reading / Remote sandbox / Quality loop / Close-out). Future
  retros edit *that* list and carry only the **delta**.
- **A session-cost line:** each retro now ends with one sentence — roughly what the session
  cost and where it went. This is the in-repo, zero-dependency equivalent of the
  context-optimizer plugins: we hold our own dev loop to the same spend-measurement standard
  the *product* holds the agent to (Meter/ReportedAIU/ledger), without installing anything
  privileged.
- The four historical checklists stay as dated snapshots (retros are records); the canonical
  list is CONVENTIONS going forward.

## Skill / tool usage

- Docs-only research session: three targeted web searches (loop engineering, efficiency
  plugins + evidence, plugin security) + repo grounding (CODEMAP telemetry decls, the four
  retro checklists). No sub-agents needed at this scale.
- Followed ADR-0050's own rule: an infra-adoption question gets an **ADR with revisit
  triggers** (0051), not just a retro paragraph — the question will be re-asked as the plugin
  ecosystem matures.

## Action items

- **A1 — the Session playbook is now the single carry-forward home.** Retros carry deltas +
  the session-cost line, not a fresh full list. ✅ (CONVENTIONS section + this retro.)
- **A2 — no third-party efficiency/observability plugins; revisit only on ADR-0051's named
  triggers** (a first-party feature, or an auditable pinned plugin with reproduced savings).
  *(the ADR is the record.)*
- **A3 — every retro ends with a session-cost sentence.** ✅ Folded into the playbook;
  dogfooded below.

## Session-optimization (delta only — canonical list now in CONVENTIONS "Session playbook")

- New this session: the playbook **is** the consolidation; nothing to add beyond it.

**Session cost:** ~light — a research/docs session, no code or test runs in the loop; spend
dominated by three web searches + a handful of targeted grep/CODEMAP reads (no full-file
dumps, no sub-agents). The cheapest class of session, and the baseline the new cost-line
will measure heavier ones against.
