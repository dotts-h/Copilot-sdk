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
