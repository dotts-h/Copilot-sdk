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

// TestMutateRollbackPreservesOrder guards the uniform mutate() rollback: a
// failed RemoveSkill (the skill is pinned by an agent) must restore every skill
// in its original position, not just keep the count. This covers the snapshot/
// restore path that replaced the per-method manual reinsertion.
func TestMutateRollbackPreservesOrder(t *testing.T) {
	f := New(t.TempDir())
	for _, id := range []string{"a", "b", "c"} {
		if err := f.AddSkill(Skill{ID: id, Name: id, Prompt: "p", Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	// Pin the middle skill so removing it must roll back.
	if err := f.AddAgent(Agent{ID: "ag", Name: "Ag", Model: "gpt-5", ReasoningEffort: "high", Skills: []string{"b"}}); err != nil {
		t.Fatal(err)
	}
	if err := f.RemoveSkill("b"); err == nil {
		t.Fatal("expected dangling-reference error removing a pinned skill")
	}
	got := []string{f.Skills[0].ID, f.Skills[1].ID, f.Skills[2].ID}
	if got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("rollback should preserve skill order, got %v", got)
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
