package tui

import "testing"

func TestChatStreamThenFinish(t *testing.T) {
	var c chatState
	c.addUser("hello")
	c.appendDelta("Hi ")
	c.appendDelta("there")
	// Mid-stream, transcript should include the provisional agent text.
	tr := c.transcript()
	if len(tr) != 2 || tr[1].Role != RoleAgent || tr[1].Text != "Hi there" {
		t.Fatalf("provisional stream not shown: %+v", tr)
	}
	c.finish("")
	if c.stream {
		t.Fatal("should not be streaming after finish")
	}
	tr = c.transcript()
	if len(tr) != 2 || tr[1].Text != "Hi there" {
		t.Fatalf("finished transcript wrong: %+v", tr)
	}
}

func TestChatFinishPrefersFinalContent(t *testing.T) {
	var c chatState
	c.appendDelta("partial")
	c.finish("authoritative final")
	tr := c.transcript()
	if tr[len(tr)-1].Text != "authoritative final" {
		t.Fatalf("final content should win: %+v", tr)
	}
}

func TestChatFinishEmptyIsNoop(t *testing.T) {
	var c chatState
	c.addUser("hi")
	c.finish("") // nothing streamed
	if len(c.turns) != 1 {
		t.Fatalf("empty finish should not add an agent turn: %+v", c.turns)
	}
}

func TestChatTools(t *testing.T) {
	var c chatState
	c.toolStart("t1", "bash", "go test ./...")
	c.toolStart("t2", "read", "main.go")
	if got := c.activeTools(); len(got) != 2 {
		t.Fatalf("expected 2 active tools, got %v", got)
	}
	c.toolProgress("t1", "running tests…")
	if tv := c.toolByID("t1"); tv == nil || tv.Progress != "running tests…" {
		t.Fatalf("progress not recorded: %+v", tv)
	}
	c.toolEnd("t1", "ok\nPASS", true)
	active := c.activeTools()
	if len(active) != 1 || active[0] != "read" {
		t.Fatalf("toolEnd wrong: %v", active)
	}
	if tv := c.toolByID("t1"); tv == nil || !tv.Done || tv.Failed || tv.Result != "ok\nPASS" || tv.Progress != "" {
		t.Fatalf("completed tool state wrong: %+v", tv)
	}
	c.toolEnd("missing", "", true) // no-op
	c.toolStart("t3", "", "x")     // ignored (no name)
	if got := c.activeTools(); len(got) != 1 {
		t.Fatalf("unexpected active set: %v", got)
	}
}

func TestChatToolFailure(t *testing.T) {
	var c chatState
	c.toolStart("t1", "bash", "false")
	c.toolEnd("t1", "exit status 1", false)
	tv := c.toolByID("t1")
	if tv == nil || !tv.Done || !tv.Failed {
		t.Fatalf("expected failed tool, got %+v", tv)
	}
}

// Reasoning must never be concatenated into the assistant's answer; each renders
// as its own turn, with reasoning committed ahead of the message it precedes.
func TestChatReasoningIsSeparate(t *testing.T) {
	var c chatState
	c.appendReasoning("let me think")
	c.appendDelta("the answer")
	c.finish("")
	got := c.turns
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
func TestChatToolInterleavesWithText(t *testing.T) {
	var c chatState
	c.appendDelta("before")
	c.toolStart("t1", "bash", "ls")
	c.appendDelta("after")
	c.finish("")
	roles := make([]Role, len(c.turns))
	for i, t := range c.turns {
		roles[i] = t.Role
	}
	want := []Role{RoleAgent, RoleTool, RoleAgent}
	if len(roles) != 3 || roles[0] != want[0] || roles[1] != want[1] || roles[2] != want[2] {
		t.Fatalf("timeline order wrong: %+v", c.turns)
	}
}
