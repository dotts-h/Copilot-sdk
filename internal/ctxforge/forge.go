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
	return nil
}

// AddSkill validates and appends a skill, rejecting duplicate IDs.
func (f *Forge) AddSkill(s Skill) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if f.Skill(s.ID) != nil {
		return fmt.Errorf("skill %q already exists", s.ID)
	}
	f.Skills = append(f.Skills, s)
	return nil
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

// SessionSpec is the compiled, ready-to-use context for a Copilot SDK session.
type SessionSpec struct {
	Model           string
	ReasoningEffort string
	SystemMessage   string
	EnabledSkillIDs []string
	SlashCommands   []string
	MCPServers      []MCPServer
	AgentID         string
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
