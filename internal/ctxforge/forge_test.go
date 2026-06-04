package ctxforge

import (
	"path/filepath"
	"strings"
	"testing"
)

func sampleSkill() Skill {
	return Skill{ID: "tdd", Name: "TDD", Description: "test first", Prompt: "Write a failing test before code.", Command: "tdd", Enabled: true}
}

func TestSkillValidate(t *testing.T) {
	if err := sampleSkill().Validate(); err != nil {
		t.Fatalf("valid skill rejected: %v", err)
	}
	bad := []Skill{
		{ID: "UPPER", Name: "x", Prompt: "p"},
		{ID: "ok", Name: "", Prompt: "p"},
		{ID: "ok", Name: "x", Prompt: ""},
		{ID: "", Name: "x", Prompt: "p"},
		{ID: "has space", Name: "x", Prompt: "p"},
	}
	for i, s := range bad {
		if err := s.Validate(); err == nil {
			t.Fatalf("case %d: expected validation error for %+v", i, s)
		}
	}
}

func TestAgentValidate(t *testing.T) {
	good := Agent{ID: "builder", Name: "Builder", Model: "gpt-5", ReasoningEffort: "high"}
	if err := good.Validate(); err != nil {
		t.Fatalf("valid agent rejected: %v", err)
	}
	if err := (Agent{ID: "x", Model: "", ReasoningEffort: "high"}).Validate(); err == nil {
		t.Fatal("expected error for missing model")
	}
	if err := (Agent{ID: "x", Model: "gpt-5", ReasoningEffort: "ludicrous"}).Validate(); err == nil {
		t.Fatal("expected error for bad reasoning effort")
	}
}

func TestForgeAddAndToggleSkill(t *testing.T) {
	f := New(t.TempDir())
	if err := f.AddSkill(sampleSkill()); err != nil {
		t.Fatal(err)
	}
	if err := f.AddSkill(sampleSkill()); err == nil {
		t.Fatal("expected duplicate skill error")
	}
	state, err := f.ToggleSkill("tdd")
	if err != nil {
		t.Fatal(err)
	}
	if state {
		t.Fatal("toggle from enabled should be disabled")
	}
	if _, err := f.ToggleSkill("missing"); err == nil {
		t.Fatal("expected error toggling unknown skill")
	}
}

func TestForgeValidateDuplicateIDs(t *testing.T) {
	f := New(t.TempDir())
	f.Skills = []Skill{sampleSkill(), sampleSkill()}
	if err := f.Validate(); err == nil {
		t.Fatal("expected duplicate id error")
	}
}

func TestForgeValidateAgentUnknownSkill(t *testing.T) {
	f := New(t.TempDir())
	f.Agents = []Agent{{ID: "a", Name: "A", Model: "gpt-5", ReasoningEffort: "high", Skills: []string{"nope"}}}
	if err := f.Validate(); err == nil {
		t.Fatal("expected unknown-skill reference error")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	f := New(dir)
	_ = f.AddSkill(sampleSkill())
	f.Instructions = []Instruction{{ID: "no-secrets", Title: "Secrets", Body: "Never print secrets.", Priority: 1, Enabled: true}}
	f.Agents = []Agent{{ID: "builder", Name: "Builder", Model: "gpt-5", ReasoningEffort: "high", Skills: []string{"tdd"}}}
	f.MCPServers = []MCPServer{{ID: "fs", Name: "fs", Command: "mcp-fs", Enabled: true}}
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Skills) != 1 || loaded.Skills[0].ID != "tdd" {
		t.Fatalf("skills not round-tripped: %+v", loaded.Skills)
	}
	if loaded.Agent("builder") == nil {
		t.Fatal("agent not round-tripped")
	}
	if loaded.Dir != dir {
		t.Fatalf("Dir not restored: %q", loaded.Dir)
	}
}

func TestLoadMissingDirIsEmpty(t *testing.T) {
	f, err := Load(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("missing forge should not error: %v", err)
	}
	if len(f.Skills) != 0 {
		t.Fatal("expected empty forge")
	}
}

func TestCompileMergesContext(t *testing.T) {
	f := New(t.TempDir())
	_ = f.AddSkill(Skill{ID: "tdd", Name: "TDD", Prompt: "Test first.", Command: "tdd", Enabled: false})
	_ = f.AddSkill(Skill{ID: "lint", Name: "Lint", Prompt: "Keep it clean.", Enabled: true})
	f.Instructions = []Instruction{
		{ID: "b", Title: "Second", Body: "second", Priority: 10, Enabled: true},
		{ID: "a", Title: "First", Body: "first", Priority: 1, Enabled: true},
		{ID: "off", Title: "Off", Body: "nope", Priority: 0, Enabled: false},
	}
	f.Agents = []Agent{{ID: "builder", Name: "Builder", Model: "claude-opus-4.7", ReasoningEffort: "xhigh", SystemMessage: "You are Builder.", Skills: []string{"tdd"}}}

	spec, err := f.Compile("builder")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Model != "claude-opus-4.7" || spec.ReasoningEffort != "xhigh" {
		t.Fatalf("agent model/effort not applied: %+v", spec)
	}
	// Agent system message comes first.
	if !strings.HasPrefix(spec.SystemMessage, "You are Builder.") {
		t.Fatalf("agent system message not first:\n%s", spec.SystemMessage)
	}
	// Instructions ordered by priority: "first" before "second".
	if strings.Index(spec.SystemMessage, "first") > strings.Index(spec.SystemMessage, "second") {
		t.Fatalf("instructions not priority-ordered:\n%s", spec.SystemMessage)
	}
	// Disabled instruction excluded.
	if strings.Contains(spec.SystemMessage, "nope") {
		t.Fatal("disabled instruction leaked into system message")
	}
	// Agent pins "tdd" even though it's globally disabled; "lint" is global.
	if len(spec.EnabledSkillIDs) != 2 {
		t.Fatalf("expected 2 active skills (tdd pinned + lint global), got %v", spec.EnabledSkillIDs)
	}
	if spec.EnabledSkillIDs[0] != "tdd" {
		t.Fatalf("agent-pinned skill should be first, got %v", spec.EnabledSkillIDs)
	}
	// Slash command from tdd skill present.
	if len(spec.SlashCommands) != 1 || spec.SlashCommands[0] != "tdd" {
		t.Fatalf("slash command not collected: %v", spec.SlashCommands)
	}
}

func TestCompileNoAgent(t *testing.T) {
	f := New(t.TempDir())
	_ = f.AddSkill(Skill{ID: "lint", Name: "Lint", Prompt: "Clean.", Enabled: true})
	spec, err := f.Compile("")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Model != "" {
		t.Fatalf("no agent should leave model empty, got %q", spec.Model)
	}
	if len(spec.EnabledSkillIDs) != 1 {
		t.Fatalf("expected global skill active, got %v", spec.EnabledSkillIDs)
	}
}

func TestCompileUnknownAgent(t *testing.T) {
	f := New(t.TempDir())
	if _, err := f.Compile("ghost"); err == nil {
		t.Fatal("expected error compiling unknown agent")
	}
}

func TestCompileExcludesDisabledMCP(t *testing.T) {
	f := New(t.TempDir())
	f.MCPServers = []MCPServer{
		{ID: "on", Name: "on", Command: "a", Enabled: true},
		{ID: "off", Name: "off", Command: "b", Enabled: false},
	}
	spec, _ := f.Compile("")
	if len(spec.MCPServers) != 1 || spec.MCPServers[0].ID != "on" {
		t.Fatalf("disabled MCP server not excluded: %+v", spec.MCPServers)
	}
}

func TestForgeRemoveSkillRollsBackOnDanglingRef(t *testing.T) {
	f := New(t.TempDir())
	if err := f.AddSkill(sampleSkill()); err != nil {
		t.Fatal(err)
	}
	if err := f.AddAgent(Agent{ID: "b", Name: "B", Model: "gpt-5", ReasoningEffort: "high", Skills: []string{"tdd"}}); err != nil {
		t.Fatal(err)
	}
	// Removing a skill an agent pins must fail and leave the skill in place.
	if err := f.RemoveSkill("tdd"); err == nil {
		t.Fatal("expected dangling-reference error removing a pinned skill")
	}
	if f.Skill("tdd") == nil {
		t.Fatal("rolled-back skill should still be present")
	}
	if err := f.Validate(); err != nil {
		t.Fatalf("forge should remain valid after rollback: %v", err)
	}
}

func TestForgeRemoveSkillOK(t *testing.T) {
	f := New(t.TempDir())
	_ = f.AddSkill(sampleSkill())
	if err := f.RemoveSkill("tdd"); err != nil {
		t.Fatalf("remove unreferenced skill: %v", err)
	}
	if f.Skill("tdd") != nil {
		t.Fatal("skill should be gone")
	}
	if err := f.RemoveSkill("tdd"); err == nil {
		t.Fatal("removing a missing skill should error")
	}
}

func TestForgeRemoveInstructionAndAgent(t *testing.T) {
	f := New(t.TempDir())
	_ = f.AddInstruction(Instruction{ID: "i1", Title: "T", Body: "b", Enabled: true})
	_ = f.AddAgent(Agent{ID: "a1", Name: "A", Model: "gpt-5", ReasoningEffort: "high"})
	if err := f.RemoveInstruction("i1"); err != nil {
		t.Fatal(err)
	}
	if f.Instruction("i1") != nil {
		t.Fatal("instruction should be gone")
	}
	if err := f.RemoveAgent("a1"); err != nil {
		t.Fatal(err)
	}
	if f.Agent("a1") != nil {
		t.Fatal("agent should be gone")
	}
	if err := f.RemoveAgent("a1"); err == nil {
		t.Fatal("removing a missing agent should error")
	}
}

func TestForgeToggleInstruction(t *testing.T) {
	f := New(t.TempDir())
	_ = f.AddInstruction(Instruction{ID: "i1", Title: "T", Body: "b", Enabled: true})
	state, err := f.ToggleInstruction("i1")
	if err != nil || state {
		t.Fatalf("toggle from enabled should be disabled: state=%v err=%v", state, err)
	}
	if _, err := f.ToggleInstruction("missing"); err == nil {
		t.Fatal("expected error toggling unknown instruction")
	}
}

func TestDefaultChatAgentResolvable(t *testing.T) {
	f := &Forge{}
	// The built-in chat agent is resolvable on an empty forge.
	if a := f.Agent(DefaultChatAgentID); a == nil || a.Name != "Chat" {
		t.Fatalf("built-in chat agent not resolvable: %+v", a)
	}
	if f.HasOwnChatAgent() {
		t.Errorf("empty forge should not report its own chat agent")
	}
	// A forge-defined chat agent overrides the built-in.
	f.Agents = []Agent{{ID: "chat", Name: "Custom Chat", Model: "gpt-5"}}
	if a := f.Agent("chat"); a == nil || a.Name != "Custom Chat" {
		t.Errorf("forge chat agent should override built-in: %+v", a)
	}
	if !f.HasOwnChatAgent() {
		t.Errorf("forge with its own chat agent should report it")
	}
}

func TestCompileCarriesAllowedTools(t *testing.T) {
	f := &Forge{Agents: []Agent{{
		ID: "scoped", Name: "Scoped", Model: "gpt-5", ReasoningEffort: "medium",
		AllowedTools: []string{"bash", "read"},
	}}}
	spec, err := f.Compile("scoped")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(spec.AllowedTools, ",") != "bash,read" {
		t.Errorf("Compile did not carry AllowedTools: %v", spec.AllowedTools)
	}
}
