// Package convo is the UI-agnostic conversation transcript model for
// my-orchestra. It was lifted out of the Bubble Tea TUI (internal/tui/chat.go)
// so both a terminal and the htmx web frontend render from the same reducer
// (see docs/WEB_UI_PLAN.md). It owns the transcript and the two independent
// in-flight buffers — assistant message text and reasoning — and exposes tool
// executions as first-class, interleaved timeline entries.
package convo

import "strings"

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

// State is the testable conversation model. It owns the transcript and two
// independent in-flight buffers — one for assistant message text and one for
// reasoning ("thinking") — so the two are never concatenated. Tool executions
// are first-class timeline entries, interleaved in the order they occur.
type State struct {
	turns   []Turn
	msgBuf  strings.Builder // current assistant message accumulation
	reaBuf  strings.Builder // current reasoning accumulation
	toolIdx map[string]int  // tool-call ID -> index into turns
}

// AddUser appends a user turn.
func (c *State) AddUser(text string) {
	c.turns = append(c.turns, Turn{Role: RoleUser, Text: text})
}

// AddSystem appends a system/notice turn.
func (c *State) AddSystem(text string) {
	c.turns = append(c.turns, Turn{Role: RoleSystem, Text: text})
}

// AppendDelta adds a streamed assistant-message chunk. Any pending reasoning is
// committed first so it renders above the answer, preserving chronology.
func (c *State) AppendDelta(text string) {
	c.commitReasoning()
	c.msgBuf.WriteString(text)
}

// AppendReasoning adds a reasoning chunk (streamed delta or a full block). Any
// pending message text is committed first so order is preserved.
func (c *State) AppendReasoning(text string) {
	c.commitMessage("")
	c.reaBuf.WriteString(text)
}

// commitReasoning flushes the reasoning buffer into a committed turn.
func (c *State) commitReasoning() {
	if c.reaBuf.Len() == 0 {
		return
	}
	c.turns = append(c.turns, Turn{Role: RoleReasoning, Text: strings.TrimRight(c.reaBuf.String(), " \n")})
	c.reaBuf.Reset()
}

// commitMessage flushes the message buffer (or final, when non-empty) into a
// committed agent turn. A no-op when there is nothing to commit.
func (c *State) commitMessage(final string) {
	text := final
	if text == "" {
		text = strings.TrimRight(c.msgBuf.String(), " \n")
	}
	c.msgBuf.Reset()
	if text != "" {
		c.turns = append(c.turns, Turn{Role: RoleAgent, Text: text})
	}
}

// Finish completes the in-flight turn: it commits any pending reasoning and the
// message (preferring finalContent when provided).
func (c *State) Finish(finalContent string) {
	c.commitReasoning()
	c.commitMessage(finalContent)
}

// ToolStart records a new tool execution as a timeline entry and remembers its
// position so progress/completion update it in place. Pending assistant text is
// committed first so the tool appears after the text that prompted it.
func (c *State) ToolStart(id, name, args string) {
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

// ToolProgress updates a running tool's latest progress message.
func (c *State) ToolProgress(id, msg string) {
	if tv := c.toolByID(id); tv != nil {
		tv.Progress = msg
	}
}

// ToolEnd marks a tool finished, recording its result and success.
func (c *State) ToolEnd(id, result string, success bool) {
	if tv := c.toolByID(id); tv != nil {
		tv.Done = true
		tv.Failed = !success
		tv.Progress = ""
		if result != "" {
			tv.Result = result
		}
	}
}

func (c *State) toolByID(id string) *ToolView {
	if id == "" || c.toolIdx == nil {
		return nil
	}
	if i, ok := c.toolIdx[id]; ok && i < len(c.turns) {
		return c.turns[i].Tool
	}
	return nil
}

// ActiveTools returns the names of tools still running, for the status line.
func (c *State) ActiveTools() []string {
	var out []string
	for _, t := range c.turns {
		if t.Role == RoleTool && t.Tool != nil && !t.Tool.Done {
			out = append(out, t.Tool.Name)
		}
	}
	return out
}

// Committed returns the committed turns only (excluding any in-flight buffer).
// Renderers pair this with Pending to show the live tail separately.
func (c *State) Committed() []Turn {
	out := make([]Turn, len(c.turns))
	copy(out, c.turns)
	return out
}

// Pending returns the in-flight buffer as a role + text. At most one buffer is
// non-empty at a time (switching modes commits the other). When nothing is in
// flight it returns (RoleAgent, "").
func (c *State) Pending() (Role, string) {
	if c.reaBuf.Len() > 0 {
		return RoleReasoning, c.reaBuf.String()
	}
	return RoleAgent, c.msgBuf.String()
}
