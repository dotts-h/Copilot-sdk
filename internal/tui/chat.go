package tui

import (
	"strings"
)

// Role identifies who authored a turn.
type Role int

const (
	RoleUser Role = iota
	RoleAgent
	RoleReasoning
	RoleSystem
	RoleTool
)

// ToolView is the displayable state of a single tool execution in the timeline.
// It is updated in place across start → progress → completion, keyed by ID.
type ToolView struct {
	ID       string
	Name     string
	Args     string // one-line summary of the arguments
	Progress string // latest progress message while running
	Result   string // detailed result/diff on completion
	Done     bool
	Failed   bool // completed unsuccessfully
}

// Turn is a single entry in the conversation transcript. For RoleTool turns the
// Tool field carries the live execution state and Text is unused.
type Turn struct {
	Role Role
	Text string
	Tool *ToolView
}

// chatState is the testable conversation model. It owns the transcript and two
// independent in-flight buffers — one for assistant message text and one for
// reasoning ("thinking") — so the two are never concatenated. Tool executions
// are first-class timeline entries, interleaved in the order they occur.
type chatState struct {
	turns   []Turn
	msgBuf  strings.Builder // current assistant message accumulation
	reaBuf  strings.Builder // current reasoning accumulation
	stream  bool
	toolIdx map[string]int // tool-call ID -> index into turns
}

// addUser appends a user turn.
func (c *chatState) addUser(text string) {
	c.turns = append(c.turns, Turn{Role: RoleUser, Text: text})
}

// addSystem appends a system/notice turn.
func (c *chatState) addSystem(text string) {
	c.turns = append(c.turns, Turn{Role: RoleSystem, Text: text})
}

// appendDelta adds a streamed assistant-message chunk. Any pending reasoning is
// committed first so it renders above the answer, preserving chronology.
func (c *chatState) appendDelta(text string) {
	c.commitReasoning()
	c.msgBuf.WriteString(text)
	c.stream = true
}

// appendReasoning adds a reasoning chunk (streamed delta or a full block). Any
// pending message text is committed first so order is preserved.
func (c *chatState) appendReasoning(text string) {
	c.commitMessage("")
	c.reaBuf.WriteString(text)
	c.stream = true
}

// commitReasoning flushes the reasoning buffer into a committed turn.
func (c *chatState) commitReasoning() {
	if c.reaBuf.Len() == 0 {
		return
	}
	c.turns = append(c.turns, Turn{Role: RoleReasoning, Text: strings.TrimRight(c.reaBuf.String(), " \n")})
	c.reaBuf.Reset()
}

// commitMessage flushes the message buffer (or final, when non-empty) into a
// committed agent turn. A no-op when there is nothing to commit.
func (c *chatState) commitMessage(final string) {
	text := final
	if text == "" {
		text = strings.TrimRight(c.msgBuf.String(), " \n")
	}
	c.msgBuf.Reset()
	if text != "" {
		c.turns = append(c.turns, Turn{Role: RoleAgent, Text: text})
	}
}

// finish completes the in-flight turn: it commits any pending reasoning and the
// message (preferring finalContent when provided) and clears the streaming flag.
func (c *chatState) finish(finalContent string) {
	c.commitReasoning()
	c.commitMessage(finalContent)
	c.stream = false
}

// toolStart records a new tool execution as a timeline entry and remembers its
// position so progress/completion update it in place. Pending assistant text is
// committed first so the tool appears after the text that prompted it.
func (c *chatState) toolStart(id, name, args string) {
	if name == "" {
		return
	}
	c.commitReasoning()
	c.commitMessage("")
	if c.toolIdx == nil {
		c.toolIdx = map[string]int{}
	}
	c.turns = append(c.turns, Turn{Role: RoleTool, Tool: &ToolView{ID: id, Name: name, Args: args}})
	if id != "" {
		c.toolIdx[id] = len(c.turns) - 1
	}
}

// toolProgress updates a running tool's latest progress message.
func (c *chatState) toolProgress(id, msg string) {
	if tv := c.toolByID(id); tv != nil {
		tv.Progress = msg
	}
}

// toolEnd marks a tool finished, recording its result and success.
func (c *chatState) toolEnd(id, result string, success bool) {
	if tv := c.toolByID(id); tv != nil {
		tv.Done = true
		tv.Failed = !success
		tv.Progress = ""
		if result != "" {
			tv.Result = result
		}
	}
}

func (c *chatState) toolByID(id string) *ToolView {
	if id == "" || c.toolIdx == nil {
		return nil
	}
	if i, ok := c.toolIdx[id]; ok && i < len(c.turns) {
		return c.turns[i].Tool
	}
	return nil
}

// activeTools returns the names of tools still running, for the status line.
func (c *chatState) activeTools() []string {
	var out []string
	for _, t := range c.turns {
		if t.Role == RoleTool && t.Tool != nil && !t.Tool.Done {
			out = append(out, t.Tool.Name)
		}
	}
	return out
}

// transcript returns the committed turns plus any live buffer (reasoning or
// message) as a provisional trailing turn for rendering. At most one buffer is
// non-empty at a time, because switching modes commits the other.
func (c *chatState) transcript() []Turn {
	out := make([]Turn, len(c.turns))
	copy(out, c.turns)
	if c.reaBuf.Len() > 0 {
		out = append(out, Turn{Role: RoleReasoning, Text: c.reaBuf.String()})
	} else if c.msgBuf.Len() > 0 {
		out = append(out, Turn{Role: RoleAgent, Text: c.msgBuf.String()})
	}
	return out
}
