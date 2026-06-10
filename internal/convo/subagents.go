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

// SubagentView is one registry entry, displayable as a list row: identity
// (both keys), naming, live status + current activity, the completion detail,
// and accumulated credits (0.00 until S3 feeds it).
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
}

// Subagents is the registry. Its zero value is ready to use. Entries are kept
// in start order; finished entries stay listed (the list is the session's
// sub-agent roster, not a transient busy indicator).
type Subagents struct {
	entries []SubagentView
	seen    map[string]bool // spawn id -> a tagged stream event was observed
}

// Start registers a spawned sub-agent as a working entry. A duplicate spawn id
// is ignored (the entry already exists).
func (r *Subagents) Start(spawnID, name, displayName, description, model string) {
	if r.bySpawn(spawnID) != nil {
		return
	}
	r.entries = append(r.entries, SubagentView{
		SpawnID: spawnID, Name: name, DisplayName: displayName,
		Description: description, Model: model,
		Status: SubagentWorking, Activity: "thinking…",
	})
}

// Observe folds one tagged stream event into the instance's entry, joining the
// instance to its spawn on first sight, and records that the instance's stream
// was actually seen doing work (the completion cross-check). activity is the
// tool now running, "thinking…" between tools, or "" to record the observation
// without touching the display. It reports whether anything displayable changed.
func (r *Subagents) Observe(instanceID, activity string) bool {
	e := r.join(instanceID)
	if e == nil {
		return false
	}
	if r.seen == nil {
		r.seen = map[string]bool{}
	}
	r.seen[e.SpawnID] = true
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
	if e == nil {
		return false
	}
	if r.seen == nil {
		r.seen = map[string]bool{}
	}
	r.seen[e.SpawnID] = true
	if credits == 0 {
		return false
	}
	e.Credits += credits
	return true
}

// End settles the spawn's entry as done or failed, keeping it listed. A
// successful completion that reported zero tokens AND whose stream we never
// observed is marked Unverified — sub-agents are known to die early yet report
// completed (claude-code#47936), so "done" is only trusted when the tokens or
// the watched stream corroborate it. Unknown spawns are ignored gracefully.
func (r *Subagents) End(spawnID string, success bool, detail string, totalTokens int64) bool {
	e := r.bySpawn(spawnID)
	if e == nil {
		return false
	}
	if success {
		e.Status = SubagentDone
		e.Unverified = totalTokens == 0 && !r.seen[spawnID]
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
func (r *Subagents) Reset() { r.entries, r.seen = nil, nil }

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
	for i := range r.entries {
		if r.entries[i].InstanceID == instanceID {
			return &r.entries[i]
		}
	}
	for i := range r.entries {
		if r.entries[i].InstanceID == "" && r.entries[i].Status == SubagentWorking {
			r.entries[i].InstanceID = instanceID
			return &r.entries[i]
		}
	}
	return nil
}
