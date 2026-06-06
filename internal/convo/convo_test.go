package convo

import "testing"

func TestStreamThenFinish(t *testing.T) {
	var c State
	c.AddUser("hello")
	c.AppendDelta("Hi ")
	c.AppendDelta("there")
	// Mid-stream the provisional agent text lives in the pending buffer, above the
	// single committed user turn.
	if len(c.Committed()) != 1 {
		t.Fatalf("only the user turn should be committed mid-stream: %+v", c.Committed())
	}
	if r, txt := c.Pending(); r != RoleAgent || txt != "Hi there" {
		t.Fatalf("provisional stream not shown: %v %q", r, txt)
	}
	c.Finish("")
	committed := c.Committed()
	if len(committed) != 2 || committed[1].Role != RoleAgent || committed[1].Text != "Hi there" {
		t.Fatalf("finished transcript wrong: %+v", committed)
	}
	if _, txt := c.Pending(); txt != "" {
		t.Fatalf("nothing should be pending after finish, got %q", txt)
	}
}

func TestAddSystemAppendsNoticeTurn(t *testing.T) {
	var c State
	c.AddUser("hi")
	c.AddSystem("conversation cleared")
	committed := c.Committed()
	last := committed[len(committed)-1]
	if last.Role != RoleSystem || last.Text != "conversation cleared" {
		t.Fatalf("AddSystem should append a system turn, got %+v", last)
	}
}

func TestFinishPrefersFinalContent(t *testing.T) {
	var c State
	c.AppendDelta("partial")
	c.Finish("authoritative final")
	committed := c.Committed()
	if committed[len(committed)-1].Text != "authoritative final" {
		t.Fatalf("final content should win: %+v", committed)
	}
}

func TestFinishEmptyIsNoop(t *testing.T) {
	var c State
	c.AddUser("hi")
	c.Finish("") // nothing streamed
	if len(c.Committed()) != 1 {
		t.Fatalf("empty finish should not add an agent turn: %+v", c.Committed())
	}
}

func TestTools(t *testing.T) {
	var c State
	c.ToolStart("t1", "bash", "go test ./...")
	c.ToolStart("t2", "read", "main.go")
	if got := c.ActiveTools(); len(got) != 2 {
		t.Fatalf("expected 2 active tools, got %v", got)
	}
	c.ToolProgress("t1", "running tests…")
	if tv := c.toolByID("t1"); tv == nil || tv.Progress != "running tests…" {
		t.Fatalf("progress not recorded: %+v", tv)
	}
	c.ToolEnd("t1", "ok\nPASS", true)
	active := c.ActiveTools()
	if len(active) != 1 || active[0] != "read" {
		t.Fatalf("toolEnd wrong: %v", active)
	}
	if tv := c.toolByID("t1"); tv == nil || !tv.Done || tv.Failed || tv.Result != "ok\nPASS" || tv.Progress != "" {
		t.Fatalf("completed tool state wrong: %+v", tv)
	}
	c.ToolEnd("missing", "", true) // no-op
	c.ToolStart("t3", "", "x")     // ignored (no name)
	if got := c.ActiveTools(); len(got) != 1 {
		t.Fatalf("unexpected active set: %v", got)
	}
}

func TestToolFailure(t *testing.T) {
	var c State
	c.ToolStart("t1", "bash", "false")
	c.ToolEnd("t1", "exit status 1", false)
	tv := c.toolByID("t1")
	if tv == nil || !tv.Done || !tv.Failed {
		t.Fatalf("expected failed tool, got %+v", tv)
	}
}

// Reasoning must never be concatenated into the assistant's answer; each renders
// as its own turn, with reasoning committed ahead of the message it precedes.
func TestReasoningIsSeparate(t *testing.T) {
	var c State
	c.AppendReasoning("let me think")
	c.AppendDelta("the answer")
	c.Finish("")
	got := c.Committed()
	if len(got) != 2 {
		t.Fatalf("expected reasoning + agent turns, got %d: %+v", len(got), got)
	}
	if got[0].Role != RoleReasoning || got[0].Text != "let me think" {
		t.Fatalf("first turn should be reasoning: %+v", got[0])
	}
	if got[1].Role != RoleAgent || got[1].Text != "the answer" {
		t.Fatalf("second turn should be the answer: %+v", got[1])
	}
}

// A tool invoked mid-stream commits the preceding assistant text first, so the
// timeline preserves chronological order: text, then tool, then more text.
func TestToolInterleavesWithText(t *testing.T) {
	var c State
	c.AppendDelta("before")
	c.ToolStart("t1", "bash", "ls")
	c.AppendDelta("after")
	c.Finish("")
	roles := make([]Role, len(c.Committed()))
	for i, t := range c.Committed() {
		roles[i] = t.Role
	}
	want := []Role{RoleAgent, RoleTool, RoleAgent}
	if len(roles) != 3 || roles[0] != want[0] || roles[1] != want[1] || roles[2] != want[2] {
		t.Fatalf("timeline order wrong: %+v", c.Committed())
	}
}

func TestPendingReflectsBuffer(t *testing.T) {
	var c State
	if r, txt := c.Pending(); txt != "" {
		t.Fatalf("expected empty pending, got %v %q", r, txt)
	}
	c.AppendReasoning("thinking")
	if r, txt := c.Pending(); r != RoleReasoning || txt != "thinking" {
		t.Fatalf("pending reasoning wrong: %v %q", r, txt)
	}
	c.AppendDelta("answer") // commits reasoning, opens message
	if r, txt := c.Pending(); r != RoleAgent || txt != "answer" {
		t.Fatalf("pending message wrong: %v %q", r, txt)
	}
}
