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
	if c.streaming {
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
	c.toolStart("bash")
	c.toolStart("read")
	if len(c.tools) != 2 {
		t.Fatalf("expected 2 active tools, got %v", c.tools)
	}
	c.toolEnd("bash")
	if len(c.tools) != 1 || c.tools[0] != "read" {
		t.Fatalf("toolEnd wrong: %v", c.tools)
	}
	c.toolEnd("missing") // no-op
	c.toolStart("")      // ignored
	if len(c.tools) != 1 {
		t.Fatalf("unexpected tool set: %v", c.tools)
	}
}
