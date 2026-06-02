package tui

import (
	"strings"
)

// Role identifies who authored a turn.
type Role int

const (
	RoleUser Role = iota
	RoleAgent
	RoleSystem
	RoleTool
)

// Turn is a single entry in the conversation transcript.
type Turn struct {
	Role Role
	Text string
}

// chatState is the testable conversation model: it owns the transcript and the
// in-flight streaming buffer, independent of any rendering concern.
type chatState struct {
	turns     []Turn
	streaming bool
	buf       strings.Builder // current assistant delta accumulation
	tools     []string        // active tool names (for the status line)
}

// addUser appends a user turn.
func (c *chatState) addUser(text string) {
	c.turns = append(c.turns, Turn{Role: RoleUser, Text: text})
}

// addSystem appends a system/notice turn.
func (c *chatState) addSystem(text string) {
	c.turns = append(c.turns, Turn{Role: RoleSystem, Text: text})
}

// beginStream starts accumulating an assistant response.
func (c *chatState) beginStream() {
	c.streaming = true
	c.buf.Reset()
}

// appendDelta adds a streamed chunk, starting a stream if needed.
func (c *chatState) appendDelta(text string) {
	if !c.streaming {
		c.beginStream()
	}
	c.buf.WriteString(text)
}

// finish completes the assistant turn. If finalContent is non-empty it replaces
// the streamed buffer (the SDK's authoritative final message); otherwise the
// accumulated buffer is used. A no-op when there is nothing to commit.
func (c *chatState) finish(finalContent string) {
	text := finalContent
	if text == "" {
		text = strings.TrimRight(c.buf.String(), " \n")
	}
	if text != "" {
		c.turns = append(c.turns, Turn{Role: RoleAgent, Text: text})
	}
	c.streaming = false
	c.buf.Reset()
	c.tools = nil
}

// toolStart records an active tool.
func (c *chatState) toolStart(name string) {
	if name == "" {
		return
	}
	c.tools = append(c.tools, name)
}

// toolEnd removes a finished tool from the active set.
func (c *chatState) toolEnd(name string) {
	for i, n := range c.tools {
		if n == name {
			c.tools = append(c.tools[:i], c.tools[i+1:]...)
			return
		}
	}
}

// transcript returns the committed turns plus the live streaming buffer (as a
// provisional agent turn) for rendering.
func (c *chatState) transcript() []Turn {
	out := make([]Turn, len(c.turns))
	copy(out, c.turns)
	if c.streaming && c.buf.Len() > 0 {
		out = append(out, Turn{Role: RoleAgent, Text: c.buf.String()})
	}
	return out
}
