# 0002. Restore SDK session resume for session pick/start/continue

- Status: accepted
- Date: 2026-06-04
- Deciders: Horia
- Related: backlog item #8 (roadmap memory), supersedes the prune in 6789fe2, `internal/copilot/copilot.go`, `internal/web`

## Context

The web app runs one live conversation per browser cookie; `/clear` is the only
way to start over and there is no way to reopen a past chat. The
`copilot.Client` seam once had `ResumeSession`/`LastSessionID`; they were pruned
(6789fe2) as dead code after the TUI hard cut removed their only caller, not
because they were wrong. The underlying GitHub Copilot SDK persists each
session's full history on disk and exposes a complete management API:
`ListSessions`, `GetSessionMetadata`, `DeleteSession`, `GetLastSessionID`,
`ResumeSession`, and `Session.GetEvents` (full history). So the durable store
already exists in the runtime — the app only needs to surface it.

## Considered options

- **In-memory switcher** — hold multiple conversations in RAM per browser, swap
  the live one. No seam change, but lost on restart and the swap code is largely
  thrown away once real persistence is wanted.
- **Hand-rolled persistence** — persist our own session index + serialized
  transcripts to disk. Rejected: duplicates what the CLI runtime already does;
  two sources of truth to keep consistent.
- **SDK-native resume** — restore the resume methods on the seam and drive the
  picker from `ListSessions`/`GetEvents`/`DeleteSession`.

## Decision

We chose **SDK-native resume**. The `copilot.Client` seam regains session
enumeration (`ListSessions` → normalized `SessionMeta`), `ResumeSession` (reattach
+ wire the same event handlers as create), and history rehydration
(`SessionHistory` → normalized `[]Event` from `Session.GetEvents`). The web layer
gets a sessions picker (list past sessions, + New chat, resume, delete). On
resume the transcript is rebuilt by replaying normalized history through
`convo.State`. We persist nothing ourselves — the runtime is the single source of
truth. The per-event SDK→normalized mapping is refactored into a pure function
shared by the live handler and history rehydration.

## Consequences

- Positive: real past chats reopen across restarts; no bespoke persistence; the
  picker doubles as the cost control (New chat = empty context; Resume = full
  context). Reused mapping keeps live + rehydrated transcripts identical.
- Negative / cost we accept: **resume restores the entire context**, and the
  prompt cache is cold after a gap/restart, so the first post-resume turn pays
  full uncached input price for the whole history (visible in the cache-hit %
  statusline). Resume is all-or-fresh — the SDK has no "last N messages" knob;
  runtime auto-compaction bounds runaway context. A resumed SDK session may have
  expired/been deleted out of band; that path must degrade gracefully.
- Follow-ups: MockClient gains settable sessions/history for tests; guard tests
  for list/resume-rehydrate/new/delete and the expired-session fallback.
