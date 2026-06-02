package tui

import (
	"testing"

	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
)

var forgeSkillFixture = ctxforge.Skill{ID: "fix", Name: "Fix", Description: "d", Prompt: "p", Enabled: true}

func TestBuildSkillValidates(t *testing.T) {
	s, err := buildSkill("tdd", "TDD", "desc", "write tests first", "tdd", true)
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "tdd" || !s.Enabled || s.Command != "tdd" {
		t.Fatalf("unexpected skill: %+v", s)
	}
	if _, err := buildSkill("BAD ID", "n", "", "p", "", true); err == nil {
		t.Fatal("expected validation error for bad id")
	}
	if _, err := buildSkill("ok", "n", "", "", "", true); err == nil {
		t.Fatal("expected error for empty prompt")
	}
}

func TestBuildInstructionParsesPriority(t *testing.T) {
	in, err := buildInstruction("rule", "Title", "body", "5", true)
	if err != nil {
		t.Fatal(err)
	}
	if in.Priority != 5 {
		t.Fatalf("priority not parsed: %d", in.Priority)
	}
	// Empty priority defaults to 0.
	in, _ = buildInstruction("rule", "Title", "body", "", true)
	if in.Priority != 0 {
		t.Fatalf("empty priority should be 0, got %d", in.Priority)
	}
	if _, err := buildInstruction("rule", "t", "b", "notanumber", true); err == nil {
		t.Fatal("expected error for non-integer priority")
	}
}

func TestBuildAgentSplitsSkills(t *testing.T) {
	a, err := buildAgent("builder", "Builder", "d", "gpt-5", "high", "be helpful", "tdd, lint ,  ")
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Skills) != 2 || a.Skills[0] != "tdd" || a.Skills[1] != "lint" {
		t.Fatalf("skills CSV not parsed/trimmed: %v", a.Skills)
	}
	if _, err := buildAgent("a", "n", "", "gpt-5", "ludicrous", "", ""); err == nil {
		t.Fatal("expected error for bad reasoning effort")
	}
	if _, err := buildAgent("a", "n", "", "", "high", "", ""); err == nil {
		t.Fatal("expected error for missing model")
	}
}

func TestFormFocusWraps(t *testing.T) {
	f := newSkillForm(nil, -1)
	if f.focus != 0 {
		t.Fatal("focus should start at 0")
	}
	f.moveFocus(-1)
	if f.focus != len(f.fields)-1 {
		t.Fatalf("moveFocus(-1) should wrap to last, got %d", f.focus)
	}
	f.moveFocus(1)
	if f.focus != 0 {
		t.Fatalf("moveFocus(1) should wrap to first, got %d", f.focus)
	}
}

func TestEditFormPrefillsAndPreservesEnabled(t *testing.T) {
	f := newSkillForm(&forgeSkillFixture, 2)
	if f.editIndex != 2 {
		t.Fatalf("editIndex = %d", f.editIndex)
	}
	if f.value("id") != "fix" || f.value("prompt") != "p" {
		t.Fatalf("form not prefilled: id=%q prompt=%q", f.value("id"), f.value("prompt"))
	}
	if !f.enabled {
		t.Fatal("edit form should preserve Enabled=true")
	}
	built, err := f.build()
	if err != nil {
		t.Fatal(err)
	}
	if s, ok := built.(interface{ Validate() error }); !ok || s == nil {
		t.Fatal("build should return a validatable skill")
	}
}
