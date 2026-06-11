# 0044. The per-sub-agent chat overlay: a per-instance named SSE listener for the live transcript, and a steer seam gated on a lane-backed flag

- Status: accepted
- Date: 2026-06-11
- Deciders: Horia
- Related: epic [0069](../issues/0069-epic-first-class-subagents.md) (first-class
  sub-agents), issue [0074](../issues/0074-subagent-chat-overlay.md) (S5, this ADR's
  child), [ADR-0040](0040-subagent-identity-instance-agentid-vs-spawn-toolcallid.md)
  (the instance↔spawn identity the overlay keys on),
  [ADR-0041](0041-subagent-registry-roster-semantics-status-vocabulary-unverified-done.md)
  (the S2 registry the transcript hangs off and the overlay opens from),
  [ADR-0043](0043-pause-ledger-escalate-back-channel-and-input-required-lane-state.md)
  (the pause form the overlay renders + the `input-required` attention state),
  [ADR-0026](0026-grouped-sidebar-navigation-and-command-palette.md) (the overlay/dialog
  pattern this reuses), [CONTRACTS §2/§3](../CONTRACTS.md)

## Context

S2 gave a live sub-agent **list** (status + activity + credits); S4 gave the **pause**
record. S5 is the drill-down the field converged on (Devin session links, Cursor's
Agents window, GitHub mission control): click a sub-agent open into a **popup chat
overlay** with its full live transcript, the pause form inline, and — where the
sub-agent has a `Send` target — a composer to steer it mid-run.

Three decisions had to be pinned, and one is a genuine fork (asked, not assumed):

1. **How the open overlay streams live** without a second transport or a per-overlay
   connection.
2. **Where the per-sub-agent transcript lives** and how it stays bounded and
   idempotent on reopen.
3. **What "steer" means** when the surface the overlay opens from (the S2 registry) is
   fed by **SDK-native, in-session** sub-agents that have **no `Send` target**.

## Decision

**1. A per-instance named SSE listener on the *shared* `/events` connection.** The
overlay is a native `<dialog>` loaded by an htmx `GET /subagent/{id}` (button **and**
double-click — double-click alone is undiscoverable and has no touch equivalent). Its
transcript region carries its **own** `sse-swap="subagent-{spawnID}"` listener; because
the page `<body>` owns the single `hx-ext="sse" sse-connect="/events"` connection,
htmx-ext-sse attaches the child listener to that **same** connection (many listeners,
one stream). The server emits a `subagent-{spawnID}` fragment whenever the instance's
transcript changes. An **unopened** overlay has no matching listener, so those events
are a **silent no-op** — and the open path doesn't rely on catching them mid-flight: the
`GET` renders the full transcript so far, and every live update is an **idempotent
full-fragment `innerHTML` re-render** (the epic's "render the listening container first;
keep idempotent re-renders the foundation" risk control). The native `<dialog>`
`showModal()` buys the focus-trap, Esc-to-close, `::backdrop`, and focus-restore for
free; a backdrop click (`event.target === dialog`) closes it.

**2. The transcript is bounded state on the S2 registry.** `convo.Subagents` grows a
per-entry `Transcript []SubagentEntry` (message / reasoning / tool-call / steer),
folded from the already-routed tagged stream: deltas **coalesce** into a trailing run of
the same kind, a tool call is a discrete one-liner (deduped by tool-call id so a repeated
start event doesn't double it), and a non-streaming full block **replaces** the trailing
run (`CommitText`, mirroring the main timeline's `Finish`) so deltas + a final message
don't double. It is **capped** (`subagentTranscriptCap`) — the full record is the
session, the overlay is a bounded recent view. `ByID(spawnID)` hands the overlay a **deep
copy** (transcript cloned) so a render can't corrupt live state.

**3. Steer is a seam gated on a lane-backed flag — SDK-native sub-agents are
read+pause-only.** The registry entry grows an optional `LaneSession` (the backing
`copilot` session). `POST /subagent/{id}/steer` delivers the composer's `prompt` via
`Client.Send(laneSession, …)` — the mission-control contract (queued, applied after the
current tool call) — and annotates it as a `steer` entry in the transcript. The composer
renders **only** when `LaneSession != ""` **and** the sub-agent is still live. An
SDK-native (in-session) sub-agent has **no `Send` target**, so the overlay is
**read + pause-only** for it (the issue's own out-of-scope boundary). No production path
sets `LaneSession` yet — the seam is built and fully tested (a recording mock asserts the
`Send` target + payload), and wiring a lane's sub-run into the registry as a steerable
entry is a deliberate follow-up, not smuggled into this slice.

The list and overlay **agree on the `input-required` attention state**: when a pause is
registered/resolved, the escalate path flips the matching registry entry
(`MarkInputRequired` / `ClearInputRequired`, matched by either identity key) and
re-broadcasts the list, and the overlay renders the pending pause forms addressed to that
sub-agent (`pausesFor`, matched by instance **or** spawn id).

## Consequences

- **No new transport, no new route family for streaming.** One named SSE event per
  sub-agent rides the existing connection; the cost is one (bounded) fragment per
  content event per active sub-agent, emitted whether or not an overlay is open — an
  accepted, bounded waste that keeps the open path stateless and the re-render idempotent.
- **Steer is honest about its gate.** The composer never offers an action that can't be
  delivered; the carve-out is visible in the rendered state, not a silent dead button.
- **Follow-up:** no path sets `LaneSession`. A later slice can register a workflow lane's
  sub-run as a steerable registry entry to light the composer end-to-end; until then
  steering is unit-covered but not user-reachable.
- **Only the transcript region streams live; the header, pause form, and steer composer
  are snapshotted at open time.** This is deliberate: those regions carry **input fields**
  (the pause reply, the steer prompt), and an `innerHTML` re-render on every transcript
  delta would wipe a half-typed value. A pause that arrives *after* the overlay is open is
  therefore not shown inside it live — but it is **not lost**: it renders on the main
  `#pauses` region and flips the list row amber, so the user opens (or reopens) the overlay
  to act on it (the `GET` renders it via `pausesFor`). A future slice can give the pause
  region its own narrowly-scoped SSE event (fired only on pause register/resolve, never on
  a delta) to update it live without the input-wipe hazard.
- All model/SDK/human text is escaped at the template seam (ADR-0001); the transcript is
  capped (no unbounded growth); the overlay is a labelled modal with glyph+text status
  (a11y, not color-only).
