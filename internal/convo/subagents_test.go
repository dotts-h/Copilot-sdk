package convo

import "testing"

// The registry's transition table (issue 0071, epic 0069 S2): one entry per
// sub-agent instance, joined across the two ADR-0040 keys — the spawn ToolCallID
// (lifecycle events) and the instance AgentID (tagged stream events) — by
// chronological first-tag-after-start. Unknown-instance events are ignored
// gracefully, and a zero-token completion with no observed stream renders
// done-unverified (the claude-code#47936 lesson).
func TestSubagentsTransitions(t *testing.T) {
	t.Run("start adds a working entry", func(t *testing.T) {
		var r Subagents
		r.Start("tc-1", "explorer", "Explore", "search the repo", "sonnet")
		got := r.Entries()
		if len(got) != 1 {
			t.Fatalf("entries = %d, want 1", len(got))
		}
		e := got[0]
		if e.SpawnID != "tc-1" || e.Status != SubagentWorking || e.InstanceID != "" {
			t.Fatalf("unexpected entry after start: %+v", e)
		}
		if e.Activity == "" {
			t.Errorf("a fresh entry should show a non-empty activity, got %+v", e)
		}
	})

	t.Run("duplicate start is ignored", func(t *testing.T) {
		var r Subagents
		r.Start("tc-1", "explorer", "Explore", "", "sonnet")
		r.Start("tc-1", "explorer", "Explore", "", "sonnet")
		if got := len(r.Entries()); got != 1 {
			t.Fatalf("duplicate start duplicated the entry: %d", got)
		}
	})

	t.Run("first tagged event joins the oldest unjoined working entry", func(t *testing.T) {
		var r Subagents
		r.Start("tc-1", "a", "A", "", "")
		r.Start("tc-2", "b", "B", "", "")
		if !r.Observe("sub-1", "grep") {
			t.Fatal("tool activity on a joined entry should report a change")
		}
		got := r.Entries()
		if got[0].InstanceID != "sub-1" || got[0].Activity != "grep" {
			t.Fatalf("first tag should join + update the oldest unjoined entry: %+v", got[0])
		}
		if got[1].InstanceID != "" {
			t.Fatalf("second entry must stay unjoined: %+v", got[1])
		}
		// The next distinct instance joins the next unjoined entry.
		r.Observe("sub-2", "bash")
		if got := r.Entries(); got[1].InstanceID != "sub-2" || got[1].Activity != "bash" {
			t.Fatalf("second tag should join the second entry: %+v", got[1])
		}
	})

	t.Run("a known instance updates its own entry, not the join order", func(t *testing.T) {
		var r Subagents
		r.Start("tc-1", "a", "A", "", "")
		r.Start("tc-2", "b", "B", "", "")
		r.Observe("sub-1", "grep")
		r.Observe("sub-1", "read")
		got := r.Entries()
		if got[0].Activity != "read" {
			t.Fatalf("repeat tag should update the joined entry in place: %+v", got[0])
		}
		if got[1].InstanceID != "" {
			t.Fatalf("repeat tag must not consume the next unjoined entry: %+v", got[1])
		}
	})

	t.Run("unchanged activity reports no change", func(t *testing.T) {
		var r Subagents
		r.Start("tc-1", "a", "A", "", "")
		r.Observe("sub-1", "grep")
		if r.Observe("sub-1", "grep") {
			t.Error("same activity twice should not report a change (delta-storm guard)")
		}
	})

	t.Run("unknown instance with nothing to join is ignored", func(t *testing.T) {
		var r Subagents
		if r.Observe("ghost", "grep") {
			t.Error("an unjoinable tag must be ignored gracefully")
		}
		if got := len(r.Entries()); got != 0 {
			t.Fatalf("ignored tag created an entry: %d", got)
		}
	})

	t.Run("end success marks done and keeps the entry listed", func(t *testing.T) {
		var r Subagents
		r.Start("tc-1", "a", "A", "", "")
		r.Observe("sub-1", "grep")
		if !r.End("tc-1", true, "1.2s · 3.4k tok", 3400) {
			t.Fatal("end on a known spawn should report a change")
		}
		got := r.Entries()
		if len(got) != 1 {
			t.Fatalf("a finished entry must stay listed, got %d", len(got))
		}
		e := got[0]
		if e.Status != SubagentDone || e.Unverified || e.Detail != "1.2s · 3.4k tok" {
			t.Fatalf("unexpected entry after verified completion: %+v", e)
		}
		if e.Activity != "" {
			t.Errorf("a finished entry should clear its activity: %+v", e)
		}
	})

	t.Run("end failure marks failed with the error detail", func(t *testing.T) {
		var r Subagents
		r.Start("tc-1", "a", "A", "", "")
		r.End("tc-1", false, "boom", 0)
		if e := r.Entries()[0]; e.Status != SubagentFailed || e.Detail != "boom" || e.Unverified {
			t.Fatalf("unexpected entry after failure: %+v", e)
		}
	})

	t.Run("zero-token completion with no observed stream is unverified", func(t *testing.T) {
		var r Subagents
		r.Start("tc-1", "a", "A", "", "")
		r.End("tc-1", true, "", 0)
		if e := r.Entries()[0]; e.Status != SubagentDone || !e.Unverified {
			t.Fatalf("zero-token silent completion must render done (unverified): %+v", e)
		}
	})

	t.Run("an observed stream verifies a zero-token completion", func(t *testing.T) {
		var r Subagents
		r.Start("tc-1", "a", "A", "", "")
		r.Observe("sub-1", "grep")
		r.End("tc-1", true, "", 0)
		if e := r.Entries()[0]; e.Unverified {
			t.Fatalf("a completion whose stream we watched do work is verified: %+v", e)
		}
	})

	t.Run("end on an unknown spawn is ignored", func(t *testing.T) {
		var r Subagents
		if r.End("ghost", true, "", 0) {
			t.Error("end on an unknown spawn must be ignored gracefully")
		}
	})

	t.Run("credits accumulate per instance", func(t *testing.T) {
		var r Subagents
		r.Start("tc-1", "a", "A", "", "")
		if !r.AddCredits("sub-1", 0.08) {
			t.Fatal("credits on a joinable instance should report a change")
		}
		r.AddCredits("sub-1", 0.02)
		if e := r.Entries()[0]; e.Credits < 0.099 || e.Credits > 0.101 {
			t.Fatalf("credits should accumulate: %+v", e)
		}
		if r.AddCredits("ghost", 1) {
			t.Error("credits for an unjoinable instance must be ignored gracefully")
		}
	})

	t.Run("reset empties the registry", func(t *testing.T) {
		var r Subagents
		r.Start("tc-1", "a", "A", "", "")
		r.Reset()
		if !r.Empty() || len(r.Entries()) != 0 {
			t.Fatal("reset should empty the registry")
		}
	})
}

// A registered pause flips a working sub-agent to input-required (the attention
// state, S4/S5), matched by either identity key, and resolution returns it to
// working — but a settled (done/failed) entry is never reopened.
func TestSubagentInputRequiredTransition(t *testing.T) {
	t.Run("a working entry flips to input-required and back, by instance or spawn id", func(t *testing.T) {
		var r Subagents
		r.Start("tc-1", "a", "A", "", "")
		r.Observe("sub-1", "grep") // join
		if !r.MarkInputRequired("sub-1") {
			t.Fatal("marking a working entry should report a change")
		}
		if v, _ := r.ByID("tc-1"); v.Status != SubagentInputRequired {
			t.Fatalf("entry should be input-required: %+v", v)
		}
		// Re-marking is a no-op; clearing by the SPAWN key returns it to working.
		if r.MarkInputRequired("sub-1") {
			t.Error("re-marking an already-parked entry should report no change")
		}
		if !r.ClearInputRequired("tc-1") {
			t.Fatal("clearing a parked entry should report a change")
		}
		if v, _ := r.ByID("tc-1"); v.Status != SubagentWorking {
			t.Fatalf("entry should be back to working: %+v", v)
		}
	})

	t.Run("a settled entry is never reopened", func(t *testing.T) {
		var r Subagents
		r.Start("tc-1", "a", "A", "", "")
		r.End("tc-1", true, "done", 100)
		if r.MarkInputRequired("tc-1") {
			t.Error("a done entry must not be flipped to input-required")
		}
		if v, _ := r.ByID("tc-1"); v.Status != SubagentDone {
			t.Fatalf("done entry must stay done: %+v", v)
		}
	})

	t.Run("an unknown id is a no-op", func(t *testing.T) {
		var r Subagents
		if r.MarkInputRequired("nope") || r.ClearInputRequired("nope") {
			t.Error("an unknown id should report no change")
		}
	})
}

// The status vocabulary is the field's de-facto 4-state standard; each state has
// a stable text label (status is conveyed by text, not color alone — a11y) and a
// stable CSS class hook.
func TestSubagentStatusVocabulary(t *testing.T) {
	cases := []struct {
		st    SubagentStatus
		label string
		class string
	}{
		{SubagentWorking, "working", "sa-working"},
		{SubagentInputRequired, "input required", "sa-input-required"},
		{SubagentDone, "done", "sa-done"},
		{SubagentFailed, "failed", "sa-failed"},
	}
	for _, c := range cases {
		if got := c.st.Label(); got != c.label {
			t.Errorf("Label(%d) = %q, want %q", c.st, got, c.label)
		}
		if got := c.st.Class(); got != c.class {
			t.Errorf("Class(%d) = %q, want %q", c.st, got, c.class)
		}
	}
}

// The per-sub-agent transcript (issue 0074, epic 0069 S5): the overlay's drill-down
// reads a bounded, coalesced record of each instance's own stream — message and
// reasoning deltas folded into a trailing run, tool calls as discrete one-line
// entries — keyed by the spawn id the list row carries. Identity joins exactly like
// Observe (the first tagged text binds an unjoined instance), and the record is
// capped so a long-running sub-agent can't grow the registry without bound.
func TestSubagentTranscript(t *testing.T) {
	t.Run("text deltas coalesce into a trailing run of the same kind", func(t *testing.T) {
		var r Subagents
		r.Start("tc-1", "explorer", "Explore", "", "sonnet")
		if !r.AppendText("sub-1", false, "Hello, ") {
			t.Fatal("first text should report a change")
		}
		r.AppendText("sub-1", false, "world")
		v, ok := r.ByID("tc-1")
		if !ok {
			t.Fatal("ByID(spawn) should find the started entry")
		}
		if len(v.Transcript) != 1 || v.Transcript[0].Kind != SubagentMessage || v.Transcript[0].Text != "Hello, world" {
			t.Fatalf("message deltas should coalesce: %+v", v.Transcript)
		}
		// The first tagged text also performs the ADR-0040 join.
		if v.InstanceID != "sub-1" {
			t.Fatalf("first tagged text should join the instance: %+v", v)
		}
	})

	t.Run("a tool call breaks the run; reasoning and message are distinct kinds", func(t *testing.T) {
		var r Subagents
		r.Start("tc-1", "a", "A", "", "")
		r.AppendText("sub-1", true, "thinking about it")
		r.AppendText("sub-1", false, "answer part 1")
		r.RecordTool("sub-1", "tc-g1", "grep", "pattern=foo")
		r.AppendText("sub-1", false, "answer part 2")
		v, _ := r.ByID("tc-1")
		if len(v.Transcript) != 4 {
			t.Fatalf("want 4 distinct entries (reasoning, message, tool, message): %+v", v.Transcript)
		}
		if v.Transcript[0].Kind != SubagentReasoning || v.Transcript[2].Kind != SubagentToolCall {
			t.Fatalf("kinds out of order: %+v", v.Transcript)
		}
		if v.Transcript[2].Text != "grep" || v.Transcript[2].Args != "pattern=foo" {
			t.Fatalf("tool entry should carry name+args: %+v", v.Transcript[2])
		}
		// A post-tool message starts a fresh run rather than merging across the tool.
		if v.Transcript[3].Text != "answer part 2" {
			t.Fatalf("post-tool message should be its own run: %+v", v.Transcript[3])
		}
	})

	t.Run("empty text is a no-op; an unknown instance is ignored", func(t *testing.T) {
		var r Subagents
		r.Start("tc-1", "a", "A", "", "")
		if r.AppendText("sub-1", false, "") {
			t.Error("empty text should not report a change")
		}
		// No started-but-unjoined entry is available once joined+the only slot is taken:
		r.AppendText("sub-1", false, "x")
		if r.AppendText("sub-2", false, "orphan") {
			t.Error("text from an unjoinable instance should be ignored")
		}
		if v, _ := r.ByID("tc-1"); len(v.Transcript) != 1 {
			t.Fatalf("orphan text must not land anywhere: %+v", v.Transcript)
		}
	})

	t.Run("the transcript is bounded; oldest entries drop", func(t *testing.T) {
		var r Subagents
		r.Start("tc-1", "a", "A", "", "")
		// Each tool call is its own entry, so this is the simplest way to overflow.
		for i := 0; i < subagentTranscriptCap+10; i++ {
			r.RecordTool("sub-1", "", "t", "")
		}
		v, _ := r.ByID("tc-1")
		if len(v.Transcript) != subagentTranscriptCap {
			t.Fatalf("transcript should cap at %d, got %d", subagentTranscriptCap, len(v.Transcript))
		}
	})

	t.Run("ByID returns a deep copy; mutating it can't corrupt the registry", func(t *testing.T) {
		var r Subagents
		r.Start("tc-1", "a", "A", "", "")
		r.AppendText("sub-1", false, "live")
		v, _ := r.ByID("tc-1")
		v.Transcript[0].Text = "tampered"
		if again, _ := r.ByID("tc-1"); again.Transcript[0].Text != "live" {
			t.Fatalf("ByID must hand back a copy, registry was mutated: %q", again.Transcript[0].Text)
		}
	})

	t.Run("ByID misses an unknown spawn", func(t *testing.T) {
		var r Subagents
		if _, ok := r.ByID("nope"); ok {
			t.Fatal("ByID should miss an unknown spawn id")
		}
	})

	t.Run("CommitText replaces the trailing streamed run rather than doubling it", func(t *testing.T) {
		var r Subagents
		r.Start("tc-1", "a", "A", "", "")
		r.AppendText("sub-1", false, "scanning ")
		r.AppendText("sub-1", false, "the repo…")
		// The canonical full message arrives after the deltas — it replaces, not appends.
		if !r.CommitText("sub-1", false, "scanning the repo… done") {
			t.Fatal("commit that changes the text should report a change")
		}
		v, _ := r.ByID("tc-1")
		if len(v.Transcript) != 1 || v.Transcript[0].Text != "scanning the repo… done" {
			t.Fatalf("commit should replace the trailing message run: %+v", v.Transcript)
		}
		// An identical re-commit is a no-op.
		if r.CommitText("sub-1", false, "scanning the repo… done") {
			t.Error("an identical commit should not report a change")
		}
		// With a tool between, the commit starts a fresh run (can't reach back past it).
		r.RecordTool("sub-1", "tc-g2", "grep", "")
		r.CommitText("sub-1", false, "final word")
		if v, _ := r.ByID("tc-1"); v.Transcript[len(v.Transcript)-1].Text != "final word" {
			t.Fatalf("post-tool commit should append a fresh run: %+v", v.Transcript)
		}
	})

	t.Run("two back-to-back committed messages are both kept, not overwritten", func(t *testing.T) {
		var r Subagents
		r.Start("tc-1", "a", "A", "", "")
		// Two non-streaming full messages with nothing between them: the second must
		// NOT clobber the first (a committed run belongs to a prior message).
		r.CommitText("sub-1", false, "first message")
		r.CommitText("sub-1", false, "second message")
		v, _ := r.ByID("tc-1")
		if len(v.Transcript) != 2 || v.Transcript[0].Text != "first message" || v.Transcript[1].Text != "second message" {
			t.Fatalf("both committed messages should survive: %+v", v.Transcript)
		}
		// Deltas after a commit start a fresh run rather than appending to the sealed one.
		r.AppendText("sub-1", false, "third ")
		r.AppendText("sub-1", false, "streamed")
		if v, _ := r.ByID("tc-1"); len(v.Transcript) != 3 || v.Transcript[2].Text != "third streamed" {
			t.Fatalf("post-commit deltas should open a new run: %+v", v.Transcript)
		}
	})

	t.Run("a sub-agent can be marked lane-backed for steering", func(t *testing.T) {
		var r Subagents
		r.Start("tc-1", "a", "A", "", "")
		if v, _ := r.ByID("tc-1"); v.LaneSession != "" {
			t.Fatal("a fresh sub-agent is not lane-backed")
		}
		r.SetLaneSession("tc-1", "sess-9")
		if v, _ := r.ByID("tc-1"); v.LaneSession != "sess-9" {
			t.Fatalf("SetLaneSession should record the backing session: %+v", v)
		}
		// Unknown spawn: a harmless no-op.
		r.SetLaneSession("nope", "x")
	})
}
