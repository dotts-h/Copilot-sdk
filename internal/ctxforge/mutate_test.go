package ctxforge

import "testing"

func sampleInstruction() Instruction {
	return Instruction{ID: "tests-first", Title: "Tests first", Body: "Always write a failing test first.", Priority: 10, Enabled: true}
}

func sampleAgent() Agent {
	return Agent{ID: "builder", Name: "Builder", Description: "ships features", Model: "gpt-5", ReasoningEffort: "high"}
}

func TestAddInstruction(t *testing.T) {
	f := New(t.TempDir())
	if err := f.AddInstruction(sampleInstruction()); err != nil {
		t.Fatalf("valid instruction rejected: %v", err)
	}
	if err := f.AddInstruction(sampleInstruction()); err == nil {
		t.Fatal("expected duplicate instruction error")
	}
	if err := f.AddInstruction(Instruction{ID: "no-body"}); err == nil {
		t.Fatal("expected validation error for empty body")
	}
	if len(f.Instructions) != 1 {
		t.Fatalf("expected 1 instruction after rejected adds, got %d", len(f.Instructions))
	}
}

func TestAddAgent(t *testing.T) {
	f := New(t.TempDir())
	if err := f.AddAgent(sampleAgent()); err != nil {
		t.Fatalf("valid agent rejected: %v", err)
	}
	if err := f.AddAgent(sampleAgent()); err == nil {
		t.Fatal("expected duplicate agent error")
	}
	// Agent referencing an unknown skill must be rejected and not appended.
	bad := sampleAgent()
	bad.ID = "bad"
	bad.Skills = []string{"ghost"}
	if err := f.AddAgent(bad); err == nil {
		t.Fatal("expected unknown-skill reference error")
	}
	if len(f.Agents) != 1 {
		t.Fatalf("expected 1 agent after rejected adds, got %d", len(f.Agents))
	}
}

func TestUpdateSkill(t *testing.T) {
	f := New(t.TempDir())
	if err := f.AddSkill(sampleSkill()); err != nil {
		t.Fatal(err)
	}
	edited := sampleSkill()
	edited.Name = "Test-Driven Dev"
	if err := f.UpdateSkill("tdd", edited); err != nil {
		t.Fatalf("valid update rejected: %v", err)
	}
	if f.Skill("tdd").Name != "Test-Driven Dev" {
		t.Fatal("update did not apply")
	}
	if err := f.UpdateSkill("missing", edited); err == nil {
		t.Fatal("expected error updating unknown skill")
	}
	// Invalid edit (empty prompt) must roll back, leaving the prior value intact.
	broken := sampleSkill()
	broken.Prompt = ""
	if err := f.UpdateSkill("tdd", broken); err == nil {
		t.Fatal("expected validation error for empty prompt")
	}
	if got := f.Skill("tdd").Prompt; got == "" {
		t.Fatal("failed update should have rolled back the prompt")
	}
}

func TestUpdateSkillRenameCollisionRollsBack(t *testing.T) {
	f := New(t.TempDir())
	_ = f.AddSkill(sampleSkill())
	other := Skill{ID: "review", Name: "Review", Prompt: "Review carefully.", Enabled: true}
	_ = f.AddSkill(other)
	// Rename "review" onto the existing "tdd" id → duplicate, must roll back.
	clash := other
	clash.ID = "tdd"
	if err := f.UpdateSkill("review", clash); err == nil {
		t.Fatal("expected duplicate-id error on rename collision")
	}
	if f.Skill("review") == nil {
		t.Fatal("rollback should have restored the original id")
	}
}

func TestUpdateAgentUnknownSkillRollsBack(t *testing.T) {
	f := New(t.TempDir())
	_ = f.AddAgent(sampleAgent())
	broken := sampleAgent()
	broken.Skills = []string{"ghost"}
	if err := f.UpdateAgent("builder", broken); err == nil {
		t.Fatal("expected unknown-skill reference error")
	}
	if len(f.Agent("builder").Skills) != 0 {
		t.Fatal("failed update should have rolled back skill refs")
	}
}

func TestUpdateInstruction(t *testing.T) {
	f := New(t.TempDir())
	_ = f.AddInstruction(sampleInstruction())
	edited := sampleInstruction()
	edited.Priority = 5
	if err := f.UpdateInstruction("tests-first", edited); err != nil {
		t.Fatalf("valid update rejected: %v", err)
	}
	if f.Instruction("tests-first").Priority != 5 {
		t.Fatal("update did not apply")
	}
	if err := f.UpdateInstruction("missing", edited); err == nil {
		t.Fatal("expected error updating unknown instruction")
	}
}
