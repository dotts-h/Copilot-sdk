package ctxforge

import (
	"fmt"
	"strings"
)

// Workflow is a named, ordered orchestration of agents: a multi-agent run that
// either hands off from one agent to the next in sequence (each step receiving
// the previous step's output) or fans the steps out to run in parallel. It is the
// product's "orchestra" surface — the forge already compiles distinct agent
// personas deterministically, so a workflow is just an ordered list of (agent,
// task) steps plus a run mode.
type Workflow struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Mode        string         `json:"mode"` // "sequential" | "parallel" ("" = sequential)
	Steps       []WorkflowStep `json:"steps"`
}

// WorkflowStep is one stage of a workflow: which agent runs it and the task
// prompt it is handed. In a sequential workflow the previous step's output is
// appended to this prompt as a handoff; in a parallel workflow every step starts
// from its own prompt with no handoff.
type WorkflowStep struct {
	// AgentID names the forge agent (or the built-in "chat" agent) the step runs
	// as. Referential integrity (the id resolving to a real agent) is enforced by
	// the whole-forge Validate, exactly like an agent's pinned-skill references.
	AgentID string `json:"agentId"`
	// Prompt is the task handed to the step's agent. In a sequential run the prior
	// step's output is appended as a handoff section before the step runs.
	Prompt string `json:"prompt"`
}

// Workflow run modes.
const (
	WorkflowSequential = "sequential"
	WorkflowParallel   = "parallel"
)

// validWorkflowModes is the accepted set; "" reads as sequential so older
// forge.json files (and a blank form select) stay valid.
var validWorkflowModes = map[string]bool{
	"": true, WorkflowSequential: true, WorkflowParallel: true,
}

// EffectiveMode returns the concrete run mode, defaulting an empty Mode to
// sequential (the lowest-risk handoff).
func (w Workflow) EffectiveMode() string {
	if w.Mode == "" {
		return WorkflowSequential
	}
	return w.Mode
}

// Validate reports whether the workflow is well-formed. Referential integrity of
// each step's AgentID is checked by Forge.Validate (it needs the agent set).
func (w Workflow) Validate() error {
	if err := validateID("workflow", w.ID); err != nil {
		return err
	}
	if strings.TrimSpace(w.Name) == "" {
		return fmt.Errorf("workflow %q: name is required", w.ID)
	}
	if !validWorkflowModes[w.Mode] {
		return fmt.Errorf("workflow %q: invalid mode %q (want sequential or parallel)", w.ID, w.Mode)
	}
	if len(w.Steps) == 0 {
		return fmt.Errorf("workflow %q: at least one step is required", w.ID)
	}
	for i, st := range w.Steps {
		if strings.TrimSpace(st.AgentID) == "" {
			return fmt.Errorf("workflow %q step %d: agent is required", w.ID, i+1)
		}
		if strings.TrimSpace(st.Prompt) == "" {
			return fmt.Errorf("workflow %q step %d: prompt is required", w.ID, i+1)
		}
	}
	return nil
}

// CompiledStep is one step of a workflow ready to run: the resolved agent's
// display name, the step's task prompt, and the compiled SessionSpec that drives
// the sub-run (the agent persona — system message, model/effort, tools, MCP
// servers — produced by the same deterministic Compile that powers agent select).
type CompiledStep struct {
	AgentID   string
	AgentName string
	Prompt    string
	Spec      SessionSpec
}

// CompileWorkflow resolves a workflow by id and compiles each step's agent into a
// SessionSpec, deterministically (each step reuses Compile, the same path agent
// selection uses). It returns the workflow and the per-step compiled inputs, so
// the runner can map each lane onto the seam's session lifecycle.
func (f *Forge) CompileWorkflow(id string) (Workflow, []CompiledStep, error) {
	w := f.Workflow(id)
	if w == nil {
		return Workflow{}, nil, fmt.Errorf("unknown workflow %q", id)
	}
	steps := make([]CompiledStep, 0, len(w.Steps))
	for i, st := range w.Steps {
		spec, err := f.Compile(st.AgentID)
		if err != nil {
			return Workflow{}, nil, fmt.Errorf("workflow %q step %d: %w", id, i+1, err)
		}
		name := st.AgentID
		if a := f.Agent(st.AgentID); a != nil && a.Name != "" {
			name = a.Name
		}
		steps = append(steps, CompiledStep{
			AgentID: st.AgentID, AgentName: name, Prompt: st.Prompt, Spec: spec,
		})
	}
	return *w, steps, nil
}

// Workflow returns the workflow with the given ID, or nil.
func (f *Forge) Workflow(id string) *Workflow {
	for i := range f.Workflows {
		if f.Workflows[i].ID == id {
			return &f.Workflows[i]
		}
	}
	return nil
}

// AddWorkflow validates and appends a workflow, rejecting duplicate IDs.
// Referential integrity (steps → agents) is enforced by a whole-forge validate,
// with the append rolled back if it leaves the forge invalid — mirroring AddAgent.
func (f *Forge) AddWorkflow(w Workflow) error {
	return f.mutate(func() error {
		if err := w.Validate(); err != nil {
			return err
		}
		if f.Workflow(w.ID) != nil {
			return fmt.Errorf("workflow %q already exists", w.ID)
		}
		f.Workflows = append(f.Workflows, w)
		return nil
	})
}

// UpdateWorkflow replaces the workflow identified by id with w, then validates the
// whole forge and rolls back to the prior value if the result is invalid (e.g. a
// step now references an unknown agent).
func (f *Forge) UpdateWorkflow(id string, w Workflow) error {
	return f.mutate(func() error {
		cur := f.Workflow(id)
		if cur == nil {
			return fmt.Errorf("unknown workflow %q", id)
		}
		*cur = w
		return nil
	})
}

// RemoveWorkflow deletes the workflow with the given id. Nothing within the forge
// references a workflow; it routes through mutate for uniform discipline.
func (f *Forge) RemoveWorkflow(id string) error {
	return f.mutate(func() error {
		for i := range f.Workflows {
			if f.Workflows[i].ID == id {
				f.Workflows = append(f.Workflows[:i], f.Workflows[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("unknown workflow %q", id)
	})
}
