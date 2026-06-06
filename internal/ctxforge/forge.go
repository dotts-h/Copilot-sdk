package ctxforge

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Forge is the in-memory, file-backed registry of context building blocks.
type Forge struct {
	// Dir is the directory the forge persists to (forge.json). It is not
	// serialized — Load sets it from the load path.
	Dir string `json:"-"`

	Skills       []Skill       `json:"skills"`
	Instructions []Instruction `json:"instructions"`
	Agents       []Agent       `json:"agents"`
	MCPServers   []MCPServer   `json:"mcpServers"`
	Workflows    []Workflow    `json:"workflows,omitempty"`
	Snippets     []Snippet     `json:"snippets,omitempty"`
}

const forgeFile = "forge.json"

// New returns an empty forge bound to dir.
func New(dir string) *Forge {
	return &Forge{Dir: dir}
}

// Load reads the forge from dir/forge.json. A missing file yields an empty
// forge (not an error), so first runs just work.
func Load(dir string) (*Forge, error) {
	f := New(dir)
	path := filepath.Join(dir, forgeFile)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return f, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read forge: %w", err)
	}
	if err := json.Unmarshal(data, f); err != nil {
		return nil, fmt.Errorf("parse forge %s: %w", path, err)
	}
	f.Dir = dir
	if err := f.Validate(); err != nil {
		return nil, err
	}
	return f, nil
}

// Save validates and writes the forge atomically to dir/forge.json.
func (f *Forge) Save() error {
	if err := f.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(f.Dir, 0o755); err != nil {
		return fmt.Errorf("create forge dir: %w", err)
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("encode forge: %w", err)
	}
	path := filepath.Join(f.Dir, forgeFile)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write forge: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("commit forge: %w", err)
	}
	return nil
}

// Validate checks every element and enforces unique IDs within each kind.
func (f *Forge) Validate() error {
	if err := uniqueIDs("skill", len(f.Skills), func(i int) string { return f.Skills[i].ID }); err != nil {
		return err
	}
	if err := uniqueIDs("instruction", len(f.Instructions), func(i int) string { return f.Instructions[i].ID }); err != nil {
		return err
	}
	if err := uniqueIDs("agent", len(f.Agents), func(i int) string { return f.Agents[i].ID }); err != nil {
		return err
	}
	if err := uniqueIDs("mcpServer", len(f.MCPServers), func(i int) string { return f.MCPServers[i].ID }); err != nil {
		return err
	}
	if err := uniqueIDs("workflow", len(f.Workflows), func(i int) string { return f.Workflows[i].ID }); err != nil {
		return err
	}
	if err := uniqueIDs("snippet", len(f.Snippets), func(i int) string { return f.Snippets[i].ID }); err != nil {
		return err
	}
	for _, s := range f.Skills {
		if err := s.Validate(); err != nil {
			return err
		}
	}
	for _, i := range f.Instructions {
		if err := i.Validate(); err != nil {
			return err
		}
	}
	for _, a := range f.Agents {
		if err := a.Validate(); err != nil {
			return err
		}
		for _, sid := range a.Skills {
			if f.Skill(sid) == nil {
				return fmt.Errorf("agent %q references unknown skill %q", a.ID, sid)
			}
		}
	}
	for _, m := range f.MCPServers {
		if err := m.Validate(); err != nil {
			return err
		}
	}
	for _, w := range f.Workflows {
		if err := w.Validate(); err != nil {
			return err
		}
		for i, st := range w.Steps {
			if f.Agent(st.AgentID) == nil {
				return fmt.Errorf("workflow %q step %d references unknown agent %q", w.ID, i+1, st.AgentID)
			}
		}
	}
	for _, s := range f.Snippets {
		if err := s.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func uniqueIDs(kind string, n int, id func(int) string) error {
	seen := make(map[string]bool, n)
	for i := 0; i < n; i++ {
		if seen[id(i)] {
			return fmt.Errorf("duplicate %s id %q", kind, id(i))
		}
		seen[id(i)] = true
	}
	return nil
}

// Skill returns the skill with the given ID, or nil.
func (f *Forge) Skill(id string) *Skill {
	for i := range f.Skills {
		if f.Skills[i].ID == id {
			return &f.Skills[i]
		}
	}
	return nil
}

// Agent returns the agent with the given ID, or nil.
func (f *Forge) Agent(id string) *Agent {
	for i := range f.Agents {
		if f.Agents[i].ID == id {
			return &f.Agents[i]
		}
	}
	// The built-in chat agent is always resolvable so chat works with no forge
	// config; a forge-defined "chat" agent above overrides it.
	if id == DefaultChatAgentID {
		a := DefaultChatAgent()
		return &a
	}
	return nil
}

// HasOwnChatAgent reports whether the forge defines its own "chat" agent (i.e.
// the built-in is overridden). The web layer uses this to decide whether to
// list the built-in.
func (f *Forge) HasOwnChatAgent() bool {
	for i := range f.Agents {
		if f.Agents[i].ID == DefaultChatAgentID {
			return true
		}
	}
	return false
}

// forgeState is a shallow snapshot of every entity slice, used to roll a failed
// mutation back to the exact pre-mutation forge.
type forgeState struct {
	skills       []Skill
	instructions []Instruction
	agents       []Agent
	mcpServers   []MCPServer
	workflows    []Workflow
	snippets     []Snippet
}

func (f *Forge) snapshot() forgeState {
	return forgeState{
		skills:       append([]Skill(nil), f.Skills...),
		instructions: append([]Instruction(nil), f.Instructions...),
		agents:       append([]Agent(nil), f.Agents...),
		mcpServers:   append([]MCPServer(nil), f.MCPServers...),
		workflows:    append([]Workflow(nil), f.Workflows...),
		snippets:     append([]Snippet(nil), f.Snippets...),
	}
}

func (f *Forge) restore(s forgeState) {
	f.Skills, f.Instructions, f.Agents = s.skills, s.instructions, s.agents
	f.MCPServers, f.Workflows, f.Snippets = s.mcpServers, s.workflows, s.snippets
}

// mutate applies a forge mutation, then validates the whole forge and rolls the
// forge back to its pre-mutation state if apply errored or the result is invalid.
// This is the single discipline every Add/Update/Remove builder shares: an
// element edit can never leave the in-memory forge (or the next Save) broken, and
// referential checks (agent→skill, workflow→agent) are enforced uniformly rather
// than per-kind. Validate is O(n) over a handful of entities, so the whole-forge
// check on every mutation is cheap.
func (f *Forge) mutate(apply func() error) error {
	snap := f.snapshot()
	if err := apply(); err != nil {
		f.restore(snap)
		return err
	}
	if err := f.Validate(); err != nil {
		f.restore(snap)
		return err
	}
	return nil
}

// AddSkill validates and appends a skill, rejecting duplicate IDs.
func (f *Forge) AddSkill(s Skill) error {
	return f.mutate(func() error {
		if err := s.Validate(); err != nil {
			return err
		}
		if f.Skill(s.ID) != nil {
			return fmt.Errorf("skill %q already exists", s.ID)
		}
		f.Skills = append(f.Skills, s)
		return nil
	})
}

// Instruction returns the instruction with the given ID, or nil.
func (f *Forge) Instruction(id string) *Instruction {
	for i := range f.Instructions {
		if f.Instructions[i].ID == id {
			return &f.Instructions[i]
		}
	}
	return nil
}

// AddInstruction validates and appends an instruction, rejecting duplicate IDs.
func (f *Forge) AddInstruction(in Instruction) error {
	return f.mutate(func() error {
		if err := in.Validate(); err != nil {
			return err
		}
		if f.Instruction(in.ID) != nil {
			return fmt.Errorf("instruction %q already exists", in.ID)
		}
		f.Instructions = append(f.Instructions, in)
		return nil
	})
}

// AddAgent validates and appends an agent, rejecting duplicate IDs. Referential
// integrity (skill references) is enforced by mutate's whole-forge validate.
func (f *Forge) AddAgent(a Agent) error {
	return f.mutate(func() error {
		if err := a.Validate(); err != nil {
			return err
		}
		if f.Agent(a.ID) != nil {
			return fmt.Errorf("agent %q already exists", a.ID)
		}
		f.Agents = append(f.Agents, a)
		return nil
	})
}

// UpdateSkill replaces the skill identified by id with s, then validates the
// whole forge and rolls back to the prior value if the result is invalid (e.g. a
// rename collides with another id, or a referenced field becomes invalid).
func (f *Forge) UpdateSkill(id string, s Skill) error {
	return f.mutate(func() error {
		cur := f.Skill(id)
		if cur == nil {
			return fmt.Errorf("unknown skill %q", id)
		}
		*cur = s
		return nil
	})
}

// UpdateInstruction replaces the instruction identified by id, rolling back on
// an invalid result.
func (f *Forge) UpdateInstruction(id string, in Instruction) error {
	return f.mutate(func() error {
		cur := f.Instruction(id)
		if cur == nil {
			return fmt.Errorf("unknown instruction %q", id)
		}
		*cur = in
		return nil
	})
}

// UpdateAgent replaces the agent identified by id, rolling back on an invalid
// result (e.g. a skill reference that does not resolve).
func (f *Forge) UpdateAgent(id string, a Agent) error {
	return f.mutate(func() error {
		cur := f.Agent(id)
		if cur == nil {
			return fmt.Errorf("unknown agent %q", id)
		}
		*cur = a
		return nil
	})
}

// ToggleSkill flips a skill's Enabled flag, returning the new state.
func (f *Forge) ToggleSkill(id string) (bool, error) {
	s := f.Skill(id)
	if s == nil {
		return false, fmt.Errorf("unknown skill %q", id)
	}
	s.Enabled = !s.Enabled
	return s.Enabled, nil
}

// ToggleInstruction flips an instruction's Enabled flag, returning the new state.
func (f *Forge) ToggleInstruction(id string) (bool, error) {
	in := f.Instruction(id)
	if in == nil {
		return false, fmt.Errorf("unknown instruction %q", id)
	}
	in.Enabled = !in.Enabled
	return in.Enabled, nil
}

// RemoveSkill deletes the skill with the given id. mutate re-validates the whole
// forge and rolls back if the removal would orphan a reference (an agent that
// pins the skill), so the caller gets a clear error instead of a forge that
// later fails to save.
func (f *Forge) RemoveSkill(id string) error {
	return f.mutate(func() error {
		for i := range f.Skills {
			if f.Skills[i].ID == id {
				f.Skills = append(f.Skills[:i], f.Skills[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("unknown skill %q", id)
	})
}

// RemoveInstruction deletes the instruction with the given id.
func (f *Forge) RemoveInstruction(id string) error {
	return f.mutate(func() error {
		for i := range f.Instructions {
			if f.Instructions[i].ID == id {
				f.Instructions = append(f.Instructions[:i], f.Instructions[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("unknown instruction %q", id)
	})
}

// RemoveAgent deletes the agent with the given id. Nothing references an agent
// within the forge (the active-agent pointer lives in config), but it routes
// through mutate for uniform rollback discipline like every other builder.
func (f *Forge) RemoveAgent(id string) error {
	return f.mutate(func() error {
		for i := range f.Agents {
			if f.Agents[i].ID == id {
				f.Agents = append(f.Agents[:i], f.Agents[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("unknown agent %q", id)
	})
}

// MCPServer returns the MCP server with the given ID, or nil.
func (f *Forge) MCPServer(id string) *MCPServer {
	for i := range f.MCPServers {
		if f.MCPServers[i].ID == id {
			return &f.MCPServers[i]
		}
	}
	return nil
}

// AddMCPServer validates and appends an MCP server, rejecting duplicate IDs.
func (f *Forge) AddMCPServer(m MCPServer) error {
	return f.mutate(func() error {
		if err := m.Validate(); err != nil {
			return err
		}
		if f.MCPServer(m.ID) != nil {
			return fmt.Errorf("mcpServer %q already exists", m.ID)
		}
		f.MCPServers = append(f.MCPServers, m)
		return nil
	})
}

// UpdateMCPServer replaces the server identified by id with m, then validates the
// whole forge and rolls back to the prior value if the result is invalid (e.g. a
// rename collides with another id).
func (f *Forge) UpdateMCPServer(id string, m MCPServer) error {
	return f.mutate(func() error {
		cur := f.MCPServer(id)
		if cur == nil {
			return fmt.Errorf("unknown mcpServer %q", id)
		}
		*cur = m
		return nil
	})
}

// ToggleMCPServer flips a server's Enabled flag, returning the new state.
func (f *Forge) ToggleMCPServer(id string) (bool, error) {
	m := f.MCPServer(id)
	if m == nil {
		return false, fmt.Errorf("unknown mcpServer %q", id)
	}
	m.Enabled = !m.Enabled
	return m.Enabled, nil
}

// RemoveMCPServer deletes the server with the given id. Nothing within the forge
// references an MCP server; it routes through mutate for uniform discipline.
func (f *Forge) RemoveMCPServer(id string) error {
	return f.mutate(func() error {
		for i := range f.MCPServers {
			if f.MCPServers[i].ID == id {
				f.MCPServers = append(f.MCPServers[:i], f.MCPServers[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("unknown mcpServer %q", id)
	})
}

// SessionSpec is the compiled, ready-to-use context for a Copilot SDK session.
type SessionSpec struct {
	Model           string
	ReasoningEffort string
	SystemMessage   string
	EnabledSkillIDs []string
	SlashCommands   []string
	MCPServers      []MCPServer
	AgentID         string
	// AllowedTools restricts the session's tools (maps to the SDK's
	// AvailableTools). Set from the active agent; empty = all tools.
	AllowedTools []string
}

// Compile produces a SessionSpec for the given agent ID (empty = no agent
// persona). It merges, in order: the agent's system message, enabled global
// instructions (by Priority), and the prompts of every active skill (the
// agent's pinned skills plus globally-enabled skills). Disabled MCP servers are
// excluded.
func (f *Forge) Compile(agentID string) (SessionSpec, error) {
	spec := SessionSpec{AgentID: agentID}

	var agent *Agent
	if agentID != "" {
		agent = f.Agent(agentID)
		if agent == nil {
			return spec, fmt.Errorf("unknown agent %q", agentID)
		}
		spec.Model = agent.Model
		spec.ReasoningEffort = agent.ReasoningEffort
		spec.AllowedTools = agent.AllowedTools
	}

	var b strings.Builder
	if agent != nil && strings.TrimSpace(agent.SystemMessage) != "" {
		b.WriteString(strings.TrimSpace(agent.SystemMessage))
		b.WriteString("\n\n")
	}

	// Instructions, sorted by priority then ID for determinism.
	instrs := make([]Instruction, 0, len(f.Instructions))
	for _, i := range f.Instructions {
		if i.Enabled {
			instrs = append(instrs, i)
		}
	}
	sort.SliceStable(instrs, func(a, c int) bool {
		if instrs[a].Priority != instrs[c].Priority {
			return instrs[a].Priority < instrs[c].Priority
		}
		return instrs[a].ID < instrs[c].ID
	})
	for _, i := range instrs {
		if t := strings.TrimSpace(i.Title); t != "" {
			b.WriteString("## " + t + "\n")
		}
		b.WriteString(strings.TrimSpace(i.Body))
		b.WriteString("\n\n")
	}

	// Active skills: agent-pinned (in declared order) then globally enabled,
	// de-duplicated.
	active := f.activeSkills(agent)
	for _, s := range active {
		spec.EnabledSkillIDs = append(spec.EnabledSkillIDs, s.ID)
		if s.Command != "" {
			spec.SlashCommands = append(spec.SlashCommands, s.Command)
		}
		b.WriteString("### Skill: " + s.Name + "\n")
		b.WriteString(strings.TrimSpace(s.Prompt))
		b.WriteString("\n\n")
	}

	spec.SystemMessage = strings.TrimSpace(b.String())

	for _, m := range f.MCPServers {
		if m.Enabled {
			spec.MCPServers = append(spec.MCPServers, m)
		}
	}
	return spec, nil
}

// activeSkills resolves the ordered, de-duplicated set of skills that should be
// active for a compilation: the agent's pinned skills first, then any
// globally-enabled skills.
func (f *Forge) activeSkills(agent *Agent) []Skill {
	seen := make(map[string]bool)
	var out []Skill
	add := func(s *Skill) {
		if s != nil && !seen[s.ID] {
			seen[s.ID] = true
			out = append(out, *s)
		}
	}
	if agent != nil {
		for _, sid := range agent.Skills {
			add(f.Skill(sid))
		}
	}
	for i := range f.Skills {
		if f.Skills[i].Enabled {
			add(&f.Skills[i])
		}
	}
	return out
}
