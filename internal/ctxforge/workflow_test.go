// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package ctxforge

import (
	"reflect"
	"strings"
	"testing"
)

// a workflow's own Validate covers shape: id, name, mode, ≥1 step, each step's
// agent + prompt — independent of whether the agent exists (that is the forge's job).
func TestWorkflowValidate(t *testing.T) {
	tests := []struct {
		name    string
		wf      Workflow
		wantErr string
	}{
		{
			name: "valid sequential",
			wf: Workflow{ID: "ship", Name: "Ship it", Mode: WorkflowSequential,
				Steps: []WorkflowStep{{AgentID: "builder", Prompt: "build it"}}},
		},
		{
			name: "valid parallel",
			wf: Workflow{ID: "fan", Name: "Fan out", Mode: WorkflowParallel,
				Steps: []WorkflowStep{{AgentID: "a", Prompt: "p"}, {AgentID: "b", Prompt: "q"}}},
		},
		{
			name: "blank mode is valid (defaults sequential)",
			wf: Workflow{ID: "ship", Name: "Ship", Mode: "",
				Steps: []WorkflowStep{{AgentID: "builder", Prompt: "build"}}},
		},
		{
			name:    "bad id",
			wf:      Workflow{ID: "Bad ID", Name: "x", Steps: []WorkflowStep{{AgentID: "a", Prompt: "p"}}},
			wantErr: "slug",
		},
		{
			name:    "missing name",
			wf:      Workflow{ID: "ship", Steps: []WorkflowStep{{AgentID: "a", Prompt: "p"}}},
			wantErr: "name is required",
		},
		{
			name:    "invalid mode",
			wf:      Workflow{ID: "ship", Name: "x", Mode: "diagonal", Steps: []WorkflowStep{{AgentID: "a", Prompt: "p"}}},
			wantErr: "invalid mode",
		},
		{
			name:    "no steps",
			wf:      Workflow{ID: "ship", Name: "x"},
			wantErr: "at least one step",
		},
		{
			name:    "step missing agent",
			wf:      Workflow{ID: "ship", Name: "x", Steps: []WorkflowStep{{Prompt: "p"}}},
			wantErr: "agent is required",
		},
		{
			name:    "step missing prompt",
			wf:      Workflow{ID: "ship", Name: "x", Steps: []WorkflowStep{{AgentID: "a"}}},
			wantErr: "prompt is required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.wf.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

// a step's When predicate is validated for a known condition, a strictly-prior
// step reference (no self/forward → no cycle), and a value for output-contains.
func TestWorkflowValidateWhen(t *testing.T) {
	base := func(steps ...WorkflowStep) Workflow {
		return Workflow{ID: "wf", Name: "WF", Mode: WorkflowSequential, Steps: steps}
	}
	tests := []struct {
		name    string
		wf      Workflow
		wantErr string
	}{
		{
			name: "valid output-contains on a prior step",
			wf: base(
				WorkflowStep{AgentID: "a", Prompt: "review"},
				WorkflowStep{AgentID: "b", Prompt: "fix", When: &StepCondition{
					Step: 1, Condition: CondOutputContains, Value: "issues"}},
			),
		},
		{
			name: "valid succeeded predicate",
			wf: base(
				WorkflowStep{AgentID: "a", Prompt: "p"},
				WorkflowStep{AgentID: "b", Prompt: "q", When: &StepCondition{Step: 1, Condition: CondSucceeded}},
			),
		},
		{
			name: "valid always needs no step",
			wf: base(
				WorkflowStep{AgentID: "a", Prompt: "p", When: &StepCondition{Condition: CondAlways}},
			),
		},
		{
			name: "unknown condition",
			wf: base(
				WorkflowStep{AgentID: "a", Prompt: "p"},
				WorkflowStep{AgentID: "b", Prompt: "q", When: &StepCondition{Step: 1, Condition: "maybe"}},
			),
			wantErr: "invalid condition",
		},
		{
			name: "self reference is forward",
			wf: base(
				WorkflowStep{AgentID: "a", Prompt: "p"},
				WorkflowStep{AgentID: "b", Prompt: "q", When: &StepCondition{Step: 2, Condition: CondSucceeded}},
			),
			wantErr: "prior step",
		},
		{
			name: "forward reference",
			wf: base(
				WorkflowStep{AgentID: "a", Prompt: "p", When: &StepCondition{Step: 2, Condition: CondSucceeded}},
				WorkflowStep{AgentID: "b", Prompt: "q"},
			),
			wantErr: "prior step",
		},
		{
			name: "first step cannot gate on a prior",
			wf: base(
				WorkflowStep{AgentID: "a", Prompt: "p", When: &StepCondition{Step: 1, Condition: CondSucceeded}},
			),
			wantErr: "prior step",
		},
		{
			name: "output-contains needs a value",
			wf: base(
				WorkflowStep{AgentID: "a", Prompt: "p"},
				WorkflowStep{AgentID: "b", Prompt: "q", When: &StepCondition{Step: 1, Condition: CondOutputContains}},
			),
			wantErr: "value is required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.wf.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

// CompileWorkflow carries each step's predicate through to the compiled step so the
// run engine can evaluate branching without re-reading the forge.
func TestCompileWorkflowCarriesWhen(t *testing.T) {
	f := New("")
	_ = f.AddAgent(Agent{ID: "a", Name: "A", Model: "gpt-5"})
	_ = f.AddAgent(Agent{ID: "b", Name: "B", Model: "gpt-5"})
	when := &StepCondition{Step: 1, Condition: CondOutputContains, Value: "issues"}
	_ = f.AddWorkflow(Workflow{ID: "br", Name: "Br", Mode: WorkflowSequential,
		Steps: []WorkflowStep{
			{AgentID: "a", Prompt: "review"},
			{AgentID: "b", Prompt: "fix", When: when},
		}})

	_, steps, err := f.CompileWorkflow("br")
	if err != nil {
		t.Fatalf("CompileWorkflow: %v", err)
	}
	if steps[0].When != nil {
		t.Errorf("step 0 should have no predicate, got %+v", steps[0].When)
	}
	if steps[1].When == nil || steps[1].When.Condition != CondOutputContains || steps[1].When.Step != 1 {
		t.Errorf("step 1 predicate not carried: %+v", steps[1].When)
	}
}

func TestWorkflowEffectiveMode(t *testing.T) {
	if got := (Workflow{}).EffectiveMode(); got != WorkflowSequential {
		t.Errorf("empty mode = %q, want sequential", got)
	}
	if got := (Workflow{Mode: WorkflowParallel}).EffectiveMode(); got != WorkflowParallel {
		t.Errorf("parallel mode = %q, want parallel", got)
	}
}

// forge-level Validate enforces step→agent referential integrity, like agent→skill.
func TestForgeValidateWorkflowAgentReference(t *testing.T) {
	f := New("")
	if err := f.AddAgent(Agent{ID: "builder", Name: "Builder", Model: "gpt-5"}); err != nil {
		t.Fatalf("AddAgent: %v", err)
	}
	// A workflow referencing a real agent is accepted.
	if err := f.AddWorkflow(Workflow{ID: "ship", Name: "Ship", Mode: WorkflowSequential,
		Steps: []WorkflowStep{{AgentID: "builder", Prompt: "go"}}}); err != nil {
		t.Fatalf("AddWorkflow valid: %v", err)
	}
	// A workflow referencing a missing agent is rejected, and the add is rolled back.
	err := f.AddWorkflow(Workflow{ID: "bad", Name: "Bad", Mode: WorkflowSequential,
		Steps: []WorkflowStep{{AgentID: "ghost", Prompt: "go"}}})
	if err == nil || !strings.Contains(err.Error(), "unknown agent") {
		t.Fatalf("AddWorkflow missing agent = %v, want unknown-agent error", err)
	}
	if f.Workflow("bad") != nil {
		t.Error("invalid workflow was not rolled back")
	}
}

// the built-in chat agent resolves, so a workflow step may target it with no
// forge-defined agent.
func TestForgeValidateWorkflowBuiltinChatAgent(t *testing.T) {
	f := New("")
	if err := f.AddWorkflow(Workflow{ID: "ask", Name: "Ask", Mode: WorkflowSequential,
		Steps: []WorkflowStep{{AgentID: DefaultChatAgentID, Prompt: "hi"}}}); err != nil {
		t.Fatalf("AddWorkflow with built-in chat agent: %v", err)
	}
}

func TestForgeWorkflowCRUD(t *testing.T) {
	f := New("")
	_ = f.AddAgent(Agent{ID: "a", Name: "A", Model: "gpt-5"})
	_ = f.AddAgent(Agent{ID: "b", Name: "B", Model: "gpt-5"})

	wf := Workflow{ID: "wf", Name: "WF", Mode: WorkflowSequential,
		Steps: []WorkflowStep{{AgentID: "a", Prompt: "first"}}}
	if err := f.AddWorkflow(wf); err != nil {
		t.Fatalf("AddWorkflow: %v", err)
	}
	if err := f.AddWorkflow(wf); err == nil {
		t.Error("AddWorkflow duplicate id should fail")
	}

	// Update to a two-step workflow.
	upd := wf
	upd.Steps = append(upd.Steps, WorkflowStep{AgentID: "b", Prompt: "second"})
	if err := f.UpdateWorkflow("wf", upd); err != nil {
		t.Fatalf("UpdateWorkflow: %v", err)
	}
	if got := f.Workflow("wf"); got == nil || len(got.Steps) != 2 {
		t.Fatalf("UpdateWorkflow did not apply: %+v", got)
	}

	// Update that breaks referential integrity rolls back.
	bad := upd
	bad.Steps = []WorkflowStep{{AgentID: "ghost", Prompt: "x"}}
	if err := f.UpdateWorkflow("wf", bad); err == nil {
		t.Error("UpdateWorkflow with unknown agent should fail")
	}
	if got := f.Workflow("wf"); got == nil || len(got.Steps) != 2 {
		t.Error("failed update was not rolled back")
	}

	if err := f.RemoveWorkflow("wf"); err != nil {
		t.Fatalf("RemoveWorkflow: %v", err)
	}
	if f.Workflow("wf") != nil {
		t.Error("RemoveWorkflow did not remove")
	}
	if err := f.RemoveWorkflow("wf"); err == nil {
		t.Error("RemoveWorkflow unknown id should fail")
	}
}

// CompileWorkflow compiles each step's agent into a SessionSpec deterministically,
// carrying the resolved agent display name and the step prompt for the lanes.
func TestCompileWorkflow(t *testing.T) {
	f := New("")
	_ = f.AddSkill(Skill{ID: "tdd", Name: "TDD", Prompt: "test first", Enabled: false})
	_ = f.AddAgent(Agent{ID: "builder", Name: "Builder", Model: "gpt-5",
		ReasoningEffort: "high", SystemMessage: "You build.", Skills: []string{"tdd"}})
	_ = f.AddAgent(Agent{ID: "sdet", Name: "SDET", Model: "claude-sonnet-4.6"})
	_ = f.AddWorkflow(Workflow{ID: "ship", Name: "Ship", Mode: WorkflowSequential,
		Steps: []WorkflowStep{
			{AgentID: "builder", Prompt: "implement the feature"},
			{AgentID: "sdet", Prompt: "harden it with tests"},
		}})

	wf, steps, err := f.CompileWorkflow("ship")
	if err != nil {
		t.Fatalf("CompileWorkflow: %v", err)
	}
	if wf.ID != "ship" || len(steps) != 2 {
		t.Fatalf("got wf=%q steps=%d", wf.ID, len(steps))
	}
	if steps[0].AgentName != "Builder" || steps[0].Spec.Model != "gpt-5" {
		t.Errorf("step 0 = %+v", steps[0])
	}
	if !strings.Contains(steps[0].Spec.SystemMessage, "You build.") {
		t.Errorf("step 0 system message missing agent persona: %q", steps[0].Spec.SystemMessage)
	}
	if steps[1].AgentName != "SDET" || steps[1].Spec.Model != "claude-sonnet-4.6" {
		t.Errorf("step 1 = %+v", steps[1])
	}
	if steps[1].Prompt != "harden it with tests" {
		t.Errorf("step 1 prompt = %q", steps[1].Prompt)
	}

	// Determinism: same forge → identical compile.
	_, steps2, _ := f.CompileWorkflow("ship")
	if !reflect.DeepEqual(steps, steps2) {
		t.Error("CompileWorkflow not deterministic")
	}

	if _, _, err := f.CompileWorkflow("nope"); err == nil {
		t.Error("CompileWorkflow unknown id should fail")
	}
}
