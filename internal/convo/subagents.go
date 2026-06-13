// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package convo

// Sub-agent registry (issue 0071, epic 0069 S2, ADR-0041): the UI-agnostic model
// of every sub-agent instance observed this session — who is running, what each
// is doing right now, how it ended, and (once S3 wires the value) what it has
// spent. It lives beside State because it is the same kind of thing: a pure,
// HTTP-free reducer target the web layer feeds from normalized events and
// renders from.
//
// Identity is the ADR-0040 two-key model. A lifecycle start event carries the
// spawn key (the parent tool-call id); the sub-agent's own streamed events carry
// the instance key (the envelope AgentID); no event carries both. The registry
// owns the join: the first tagged event for an unknown instance binds it to the
// oldest still-working entry that has no instance yet (chronological
// first-tag-after-start). A tag that matches nothing is ignored gracefully —
// dropping an unknown instance's activity is safe, inventing an entry for it is
// not.

// SubagentStatus is the 4-state status vocabulary the field converged on
// (Devin / Cursor / GitHub mission control — SUBAGENTS_RESEARCH §5).
// SubagentInputRequired is the attention state: the enum exists now so the
// vocabulary is complete, S4 wires the transition.
type SubagentStatus int

const (
	SubagentWorking SubagentStatus = iota
	SubagentInputRequired
	SubagentDone
	SubagentFailed
)

// Label is the status as user-facing text. Status is always conveyed by text,
// not color alone (a11y), so every state has a stable label.
func (st SubagentStatus) Label() string {
	switch st {
	case SubagentInputRequired:
		return "input required"
	case SubagentDone:
		return "done"
	case SubagentFailed:
		return "failed"
	default:
		return "working"
	}
}

// Class is the stable CSS class hook for the state's glyph/color treatment.
func (st SubagentStatus) Class() string {
	switch st {
	case SubagentInputRequired:
		return "sa-input-required"
	case SubagentDone:
		return "sa-done"
	case SubagentFailed:
		return "sa-failed"
	default:
		return "sa-working"
	}
}

// SubagentEntryKind classifies one transcript entry: a run of model prose, a run
// of reasoning, or a single tool invocation. The overlay (S5) renders prose/
// reasoning as text blocks and a tool call as a collapsed one-liner.
type SubagentEntryKind int

const (
	SubagentMessage SubagentEntryKind = iota
	SubagentReasoning
	SubagentToolCall
	SubagentSteer // a human-authored steer message delivered into the sub-agent (S5)
)

// SubagentEntry is one entry in a sub-agent's drill-down transcript (issue 0074):
// either a coalesced run of model/reasoning text, or a tool call (Text = tool
// name, Args = its one-line arguments). All fields are model/SDK-originated and
// MUST flow through html/template escaping at the render seam (ADR-0001).
type SubagentEntry struct {
	Kind      SubagentEntryKind
	Text      string // prose/reasoning text, or the tool name for a tool call
	Args      string // tool arguments (tool calls only)
	ToolID    string // tool-call id (tool calls only) — dedupes a repeated start event
	committed bool   // a finalized text run (CommitText) — a new message starts a fresh entry, never overwrites it
}

// subagentTranscriptCap bounds the retained per-sub-agent transcript: the full
// record is the session, the overlay is a bounded recent view (issue 0074's
// "cap retained turns per sub-agent"). Oldest entries drop on overflow.
const subagentTranscriptCap = 200

// SubagentView is one registry entry, displayable as a list row: identity
// (both keys), naming, live status + current activity, the completion detail,
// and accumulated credits (0.00 until S3 feeds it). The Transcript and
// LaneSession fields back the S5 overlay drill-down.
type SubagentView struct {
	SpawnID     string // lifecycle tool-call id — the spawn key (ADR-0040)
	InstanceID  string // envelope AgentID — the instance key; "" until joined
	Name        string
	DisplayName string
	Description string
	Model       string
	Status      SubagentStatus
	Activity    string  // latest tool name, or "thinking…"; "" once finished
	Detail      string  // completion summary, or the error on failure
	Credits     float64 // live credits (S3 wires the value; renders 0 until then)
	Unverified  bool    // done, but with zero tokens and no observed stream
	// LaneSession is the backing copilot session of a lane-backed sub-agent — the
	// Send target the overlay steers into (issue 0074 S5). Empty for an SDK-native
	// (in-session) sub-agent, which has no Send target and is read+pause-only.
	LaneSession string
	// Transcript is the bounded drill-down record of this instance's own stream
	// (the overlay's content). Empty until the first tagged text/tool is observed.
	Transcript []SubagentEntry
}

// Subagents is the registry. Its zero value is ready to use. Entries are kept
// in start order; finished entries stay listed (the list is the session's
// sub-agent roster, not a transient busy indicator).
type Subagents struct {
	entries []SubagentView
}

// SubagentThinking is the between-tools activity label — the registry's only
// non-tool activity vocabulary, shared with the reducer that feeds it.
const SubagentThinking = "thinking…"

// Start registers a spawned sub-agent as a working entry. A duplicate spawn id
// is ignored (the entry already exists).
func (r *Subagents) Start(spawnID, name, displayName, description, model string) {
	if r.bySpawn(spawnID) != nil {
		return
	}
	r.entries = append(r.entries, SubagentView{
		SpawnID: spawnID, Name: name, DisplayName: displayName,
		Description: description, Model: model,
		Status: SubagentWorking, Activity: SubagentThinking,
	})
}

// Observe folds one tagged stream event into the instance's entry, joining the
// instance to its spawn on first sight — the join itself is the record that
// the instance's stream was seen doing work (the completion cross-check).
// activity is the tool now running, SubagentThinking between tools, or "" to
// record the observation without touching the display. It reports whether
// anything displayable changed.
func (r *Subagents) Observe(instanceID, activity string) bool {
	e := r.join(instanceID)
	if e == nil {
		return false
	}
	if activity == "" || activity == e.Activity {
		return false
	}
	e.Activity = activity
	return true
}

// AddCredits accumulates priced spend onto the instance's entry (the S3
// caller's seam — tagged EvUsage, priced, lands here). It joins like Observe
// and reports whether the displayed value changed.
func (r *Subagents) AddCredits(instanceID string, credits float64) bool {
	e := r.join(instanceID)
	if e == nil || credits == 0 {
		return false
	}
	e.Credits += credits
	return true
}

// AppendText folds one streamed text delta (or a committed message) from the
// instance into its drill-down transcript, joining the instance like Observe.
// Consecutive text of the same kind coalesces into the trailing entry (the
// delta-append fast path); a kind change or an intervening tool call starts a
// fresh run. Empty text, or text from an instance that can't be joined, is a
// no-op. It reports whether the transcript changed.
func (r *Subagents) AppendText(instanceID string, reasoning bool, text string) bool {
	if text == "" {
		return false
	}
	e := r.join(instanceID)
	if e == nil {
		return false
	}
	kind := SubagentMessage
	if reasoning {
		kind = SubagentReasoning
	}
	// Coalesce into the trailing run only while it is still open (uncommitted); a
	// finalized run belongs to a prior message, so a new delta starts a fresh entry.
	if n := len(e.Transcript); n > 0 && e.Transcript[n-1].Kind == kind && !e.Transcript[n-1].committed {
		e.Transcript[n-1].Text += text
	} else {
		e.Transcript = append(e.Transcript, SubagentEntry{Kind: kind, Text: text})
	}
	r.capTranscript(e)
	return true
}

// CommitText folds the canonical, non-streaming full text of a message/reasoning
// block: it REPLACES the trailing run of the same kind (the deltas that streamed
// it) rather than appending, so streamed deltas followed by the final full
// message don't double — mirroring the main timeline's Finish. When no same-kind
// run trails (e.g. a tool intervened, or a non-streaming block with no deltas) it
// appends a fresh run. An identical re-commit, empty text, or an unjoinable
// instance is a no-op. Reports whether the transcript changed.
func (r *Subagents) CommitText(instanceID string, reasoning bool, text string) bool {
	if text == "" {
		return false
	}
	e := r.join(instanceID)
	if e == nil {
		return false
	}
	kind := SubagentMessage
	if reasoning {
		kind = SubagentReasoning
	}
	// The commit finalizes the run the deltas streamed — replace the trailing
	// same-kind run IF it is still open (uncommitted). An identical text is an
	// idempotent no-op (but still seals the run). A trailing run that is already
	// committed belongs to a PRIOR message, so a distinct commit appends a fresh
	// entry instead of overwriting it (no silent loss of back-to-back messages).
	if n := len(e.Transcript); n > 0 && e.Transcript[n-1].Kind == kind {
		last := &e.Transcript[n-1]
		if last.Text == text {
			last.committed = true
			return false
		}
		if !last.committed {
			last.Text = text
			last.committed = true
			return true
		}
	}
	e.Transcript = append(e.Transcript, SubagentEntry{Kind: kind, Text: text, committed: true})
	r.capTranscript(e)
	return true
}

// RecordTool appends a tool invocation to the instance's transcript as its own
// one-line entry (Text = name, Args = arguments, keyed by toolID), joining like
// Observe. It is a discrete entry so a following text delta starts a fresh run
// rather than merging across the tool. A repeated start for the SAME non-empty
// toolID as the trailing tool entry is idempotent (the same call, not a new one),
// so a duplicate stream event doesn't double the line. An empty name, or an
// unjoinable instance, is a no-op. Reports whether the transcript changed.
func (r *Subagents) RecordTool(instanceID, toolID, name, args string) bool {
	if name == "" {
		return false
	}
	e := r.join(instanceID)
	if e == nil {
		return false
	}
	if n := len(e.Transcript); n > 0 && toolID != "" {
		if last := e.Transcript[n-1]; last.Kind == SubagentToolCall && last.ToolID == toolID {
			return false
		}
	}
	e.Transcript = append(e.Transcript, SubagentEntry{Kind: SubagentToolCall, Text: name, Args: args, ToolID: toolID})
	r.capTranscript(e)
	return true
}

// capTranscript trims an entry's transcript to the most recent
// subagentTranscriptCap entries, dropping the oldest so the registry stays
// bounded under a long-running sub-agent.
func (r *Subagents) capTranscript(e *SubagentView) {
	if over := len(e.Transcript) - subagentTranscriptCap; over > 0 {
		e.Transcript = append(e.Transcript[:0], e.Transcript[over:]...)
	}
}

// MarkInputRequired flips a still-running entry (matched by either identity key,
// instance or spawn) to the input-required attention state — where a registered
// pause parks it (S4/S5). A settled (done/failed) entry is left alone, and an
// already-parked one is a no-op. Reports whether the displayed status changed.
func (r *Subagents) MarkInputRequired(id string) bool {
	e := r.byAnyID(id)
	if e == nil || e.Status != SubagentWorking {
		return false
	}
	e.Status = SubagentInputRequired
	return true
}

// ClearInputRequired returns an input-required entry (matched by either key) to
// working when its pause resolves. A no-op for any other status. Reports whether
// the status changed.
func (r *Subagents) ClearInputRequired(id string) bool {
	e := r.byAnyID(id)
	if e == nil || e.Status != SubagentInputRequired {
		return false
	}
	e.Status = SubagentWorking
	return true
}

// byAnyID resolves an entry by either identity key — the instance AgentID a pause
// is tagged with, or the spawn ToolCallID the overlay/list row carries. Instance
// first (the more specific key), then spawn.
func (r *Subagents) byAnyID(id string) *SubagentView {
	if e := r.byInstance(id); e != nil {
		return e
	}
	return r.bySpawn(id)
}

// byInstance returns the entry currently bound to the instance AgentID key, or
// nil — the read-only instance lookup shared by join, byAnyID, and ViewByInstance.
func (r *Subagents) byInstance(instanceID string) *SubagentView {
	if instanceID == "" {
		return nil
	}
	for i := range r.entries {
		if r.entries[i].InstanceID == instanceID {
			return &r.entries[i]
		}
	}
	return nil
}

// RecordSteer annotates the spawn's transcript with a human-authored steer
// message (the overlay knows the spawn id, so this is spawn-keyed). It makes the
// human's mid-run intervention visible in the drill-down. Empty text or an
// unknown spawn is a no-op; reports whether the transcript changed.
func (r *Subagents) RecordSteer(spawnID, text string) bool {
	if text == "" {
		return false
	}
	e := r.bySpawn(spawnID)
	if e == nil {
		return false
	}
	e.Transcript = append(e.Transcript, SubagentEntry{Kind: SubagentSteer, Text: text})
	r.capTranscript(e)
	return true
}

// SetLaneSession marks the spawn's entry lane-backed, recording the backing
// session the overlay steers into (issue 0074 S5). An unknown spawn is ignored.
func (r *Subagents) SetLaneSession(spawnID, session string) {
	if e := r.bySpawn(spawnID); e != nil {
		e.LaneSession = session
	}
}

// ByID returns a deep copy of the spawn's entry (transcript included) for the
// overlay drill-down, and whether it was found. The copy is independent of the
// registry, so a render-side mutation can't corrupt live state.
func (r *Subagents) ByID(spawnID string) (SubagentView, bool) {
	if e := r.bySpawn(spawnID); e != nil {
		return copyView(e), true
	}
	return SubagentView{}, false
}

// ViewByInstance returns a deep copy of the entry currently bound to instanceID
// — a read-only lookup that does NOT join (unlike join's binding) — and whether
// one exists. The server uses it to address a just-updated sub-agent's overlay
// fragment by its spawn id after folding a tagged event.
func (r *Subagents) ViewByInstance(instanceID string) (SubagentView, bool) {
	if e := r.byInstance(instanceID); e != nil {
		return copyView(e), true
	}
	return SubagentView{}, false
}

// copyView returns a deep copy of an entry with its transcript slice cloned, so a
// handed-out view shares no mutable state with the registry.
func copyView(e *SubagentView) SubagentView {
	v := *e
	v.Transcript = append([]SubagentEntry(nil), e.Transcript...)
	return v
}

// NameFor returns the display label of the instance's entry, joining it like
// Observe if needed (so a usage event that is the first tag still binds). Empty
// when no entry can be joined — the caller still meters the spend (it is real),
// the label just falls back to the raw instance id. The S3 caller's seam for
// labelling a tagged ledger record.
func (r *Subagents) NameFor(instanceID string) string {
	e := r.join(instanceID)
	if e == nil {
		return ""
	}
	if e.DisplayName != "" {
		return e.DisplayName
	}
	return e.Name
}

// End settles the spawn's entry as done or failed, keeping it listed. A
// successful completion that reported zero tokens AND whose stream we never
// observed (no instance ever joined the entry) is marked Unverified —
// sub-agents are known to die early yet report completed (claude-code#47936),
// so "done" is only trusted when the tokens or the watched stream corroborate
// it. Unknown spawns are ignored gracefully.
func (r *Subagents) End(spawnID string, success bool, detail string, totalTokens int64) bool {
	e := r.bySpawn(spawnID)
	if e == nil {
		return false
	}
	if success {
		e.Status = SubagentDone
		e.Unverified = totalTokens == 0 && e.InstanceID == ""
	} else {
		e.Status = SubagentFailed
	}
	e.Detail = detail
	e.Activity = ""
	return true
}

// Entries returns a copy of the registry in start order.
func (r *Subagents) Entries() []SubagentView {
	out := make([]SubagentView, len(r.entries))
	copy(out, r.entries)
	return out
}

// Empty reports whether nothing has been registered.
func (r *Subagents) Empty() bool { return len(r.entries) == 0 }

// Reset empties the registry (a /clear).
func (r *Subagents) Reset() { r.entries = nil }

// bySpawn returns the entry with the given spawn id, or nil.
func (r *Subagents) bySpawn(spawnID string) *SubagentView {
	for i := range r.entries {
		if r.entries[i].SpawnID == spawnID {
			return &r.entries[i]
		}
	}
	return nil
}

// join resolves an instance id to its entry: the already-joined entry, else the
// oldest still-working entry with no instance yet (first-tag-after-start,
// ADR-0040), else nil.
func (r *Subagents) join(instanceID string) *SubagentView {
	if instanceID == "" {
		return nil
	}
	if e := r.byInstance(instanceID); e != nil {
		return e
	}
	for i := range r.entries {
		if r.entries[i].InstanceID == "" && r.entries[i].Status == SubagentWorking {
			r.entries[i].InstanceID = instanceID
			return &r.entries[i]
		}
	}
	return nil
}
