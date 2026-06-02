// Package ctxforge is "my-ctx forge": the context-engineering subsystem of
// my-orchestra. It owns the user's reusable building blocks — skills,
// instructions, agents, and MCP servers — and compiles a selected subset into a
// concrete SessionSpec that drives a Copilot SDK session.
//
// The forge is file-backed (JSON, stdlib-only) so a session's context is
// reproducible, diffable, and shareable across machines.
package ctxforge

import (
	"fmt"
	"regexp"
	"strings"
)

// idPattern constrains the identifiers used for skills/agents/etc. to
// filesystem- and URL-safe slugs.
var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// Skill is a reusable capability: a named prompt fragment (and optional command
// trigger) the user can toggle into a session's system message.
type Skill struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// Prompt is injected into the system message when the skill is enabled.
	Prompt string `json:"prompt"`
	// Command, if set, registers a slash-command name that surfaces the skill.
	Command string `json:"command,omitempty"`
	Enabled bool   `json:"enabled"`
}

// Instruction is a standalone system-message rule (e.g. "always write tests
// first"). Instructions are ordered by Priority ascending when compiled.
type Instruction struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	Priority int    `json:"priority"`
	Enabled  bool   `json:"enabled"`
}

// Agent is a named persona: a model + reasoning effort + extra system guidance,
// optionally scoped to a set of skills.
type Agent struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoningEffort"` // low|medium|high|xhigh
	SystemMessage   string `json:"systemMessage,omitempty"`
	// Skills lists skill IDs this agent always activates, beyond globally
	// enabled skills.
	Skills []string `json:"skills,omitempty"`
}

// MCPServer describes a Model Context Protocol server the forge can expose to a
// session.
type MCPServer struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Enabled bool              `json:"enabled"`
}

// validReasoningEfforts is the set the Copilot SDK accepts.
var validReasoningEfforts = map[string]bool{
	"": true, "low": true, "medium": true, "high": true, "xhigh": true,
}

// Validate reports whether the skill is well-formed.
func (s Skill) Validate() error {
	if err := validateID("skill", s.ID); err != nil {
		return err
	}
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("skill %q: name is required", s.ID)
	}
	if strings.TrimSpace(s.Prompt) == "" {
		return fmt.Errorf("skill %q: prompt is required", s.ID)
	}
	return nil
}

// Validate reports whether the instruction is well-formed.
func (i Instruction) Validate() error {
	if err := validateID("instruction", i.ID); err != nil {
		return err
	}
	if strings.TrimSpace(i.Body) == "" {
		return fmt.Errorf("instruction %q: body is required", i.ID)
	}
	return nil
}

// Validate reports whether the agent is well-formed.
func (a Agent) Validate() error {
	if err := validateID("agent", a.ID); err != nil {
		return err
	}
	if strings.TrimSpace(a.Model) == "" {
		return fmt.Errorf("agent %q: model is required", a.ID)
	}
	if !validReasoningEfforts[a.ReasoningEffort] {
		return fmt.Errorf("agent %q: invalid reasoningEffort %q", a.ID, a.ReasoningEffort)
	}
	return nil
}

// Validate reports whether the MCP server is well-formed.
func (m MCPServer) Validate() error {
	if err := validateID("mcpServer", m.ID); err != nil {
		return err
	}
	if strings.TrimSpace(m.Command) == "" {
		return fmt.Errorf("mcpServer %q: command is required", m.ID)
	}
	return nil
}

func validateID(kind, id string) error {
	if !idPattern.MatchString(id) {
		return fmt.Errorf("%s id %q must be a lowercase slug ([a-z0-9-], 1-64 chars)", kind, id)
	}
	return nil
}
