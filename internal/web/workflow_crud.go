// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// This file is the Workflows CRUD page (mirrors the agents page): the forms and
// handlers that author the workflow definitions the run engine (run_engine.go)
// later executes. It is the definition surface; the run surface lives in the
// run_* files.

func (s *Server) workflowsPartial() string {
	s.hub.forgeMu.Lock()
	defer s.hub.forgeMu.Unlock()
	// Join the two pure readers keyed by workflow id, each guarded for an absent store:
	// RunAggregates carries the last-run signal + run count (RunStore), WorkflowShares
	// the authoritative per-workflow spend (SpendStore). The row already knows its own
	// id/name, so nothing new is resolved — the join only badges each row (V4). With no
	// store wired (or no record for a workflow), the maps stay empty and the row renders
	// its prior shape, no badges. Read under forgeMu like runsPartial; the stores' own
	// mutex is a leaf lock, so no lock-order risk.
	runAggs := map[string]telemetry.RunAggregate{}
	if s.runs != nil {
		for _, a := range telemetry.RunAggregates(s.runs.Records()) {
			runAggs[a.WorkflowID] = a
		}
	}
	spendShares := map[string]telemetry.WorkflowShare{}
	if s.spend != nil {
		for _, sh := range telemetry.WorkflowShares(s.spend.Records()) {
			spendShares[sh.WorkflowID] = sh
		}
	}
	rows := make([]map[string]any, 0, len(s.forge.Workflows))
	for _, w := range s.forge.Workflows {
		agents := make([]string, len(w.Steps))
		for i, st := range w.Steps {
			agents[i] = st.AgentID
		}
		desc := w.EffectiveMode() + " · " + strings.Join(agents, " → ")
		if w.EffectiveMode() == ctxforge.WorkflowParallel {
			desc = w.EffectiveMode() + " · " + strings.Join(agents, " ‖ ")
		}
		row := map[string]any{"ID": w.ID, "Name": w.Name, "Desc": truncate(desc, 90)}
		// Last-run + run-count badges (RunStore). Guard on Runs>0 so a workflow with no
		// run history shows no run badge, just navigation.
		if a, ok := runAggs[w.ID]; ok && a.Runs > 0 {
			glyph, state := runOutcomeGlyph(a.LastOutcome)
			row["HasRuns"] = true
			row["Runs"] = a.Runs
			row["LastGlyph"] = glyph
			row["LastState"] = state
			row["LastOutcome"] = a.LastOutcome
			row["LastWhen"] = humanWhen(a.LastStartedAt)
		}
		// Spend badge (SpendStore): the workflow's total metered credits.
		if sh, ok := spendShares[w.ID]; ok {
			row["HasSpend"] = true
			row["Spend"] = telemetry.FormatCredits(sh.Credits)
		}
		rows = append(rows, row)
	}
	return frag("workflowsPage", map[string]any{"Add": addData("workflows", "workflow"), "Rows": rows})
}

var workflowModeOpts = []string{ctxforge.WorkflowSequential, ctxforge.WorkflowParallel}

func renderWorkflowForm(w ctxforge.Workflow, isNew bool, errMsg string) string {
	title, action := "Edit workflow", "/workflows/"+w.ID
	if isNew {
		title, action = "New workflow", "/workflows"
	}
	mode := w.Mode
	if mode == "" {
		mode = ctxforge.WorkflowSequential
	}
	return formShell(title, action, "workflows", errMsg,
		idField(w.ID, isNew),
		textField("Name", "name", w.Name, true),
		textField("Description", "description", w.Description, false),
		selectField("Mode", "mode", mode, workflowModeOpts),
		textArea("Steps (one per line: agentID [step N condition value]: prompt)", "steps", stepsToText(w.Steps), true),
	)
}

// stepsToText renders a workflow's steps as the textarea's "agentID: prompt" lines
// (with an optional "[step N condition value]" predicate prefix on a gated step),
// the inverse of stepsFromText.
func stepsToText(steps []ctxforge.WorkflowStep) string {
	var b strings.Builder
	for _, st := range steps {
		b.WriteString(st.AgentID)
		if cond := formatStepCondition(st.When); cond != "" {
			b.WriteString(" [")
			b.WriteString(cond)
			b.WriteString("]")
		}
		b.WriteString(": ")
		b.WriteString(st.Prompt)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// stepsFromText parses the steps textarea: one "agentID [predicate]: prompt" per
// non-blank line, where the bracketed predicate is optional. A line with no colon is
// treated as an agent id with an empty prompt, so Validate surfaces a clear "prompt
// is required" rather than silently dropping it.
func stepsFromText(raw string) []ctxforge.WorkflowStep {
	var steps []ctxforge.WorkflowStep
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		head, prompt := splitStepLine(line)
		agentPart := strings.TrimSpace(head)
		var when *ctxforge.StepCondition
		if i := strings.IndexByte(agentPart, '['); i >= 0 {
			if j := strings.IndexByte(agentPart, ']'); j > i {
				when = parseStepCondition(agentPart[i+1 : j])
			}
			agentPart = strings.TrimSpace(agentPart[:i])
		}
		steps = append(steps, ctxforge.WorkflowStep{
			AgentID: agentPart, Prompt: strings.TrimSpace(prompt), When: when,
		})
	}
	return steps
}

// splitStepLine splits a step line into its "agentID [predicate]" head and prompt
// at the separator colon. The colon is the first one AFTER any closing "]", so an
// output-contains value inside the bracket may itself contain a colon without
// derailing the split; with no bracket it is the first colon (as before). A line
// with no qualifying colon is all head (empty prompt), so Validate surfaces a clear
// "prompt is required".
func splitStepLine(line string) (head, prompt string) {
	sep := strings.IndexByte(line, ':')
	if b := strings.IndexByte(line, '['); b >= 0 {
		if c := strings.IndexByte(line, ']'); c > b {
			if rel := strings.IndexByte(line[c:], ':'); rel >= 0 {
				sep = c + rel
			} else {
				sep = -1
			}
		}
	}
	if sep < 0 {
		return line, ""
	}
	return line[:sep], line[sep+1:]
}

// formatStepCondition renders a predicate as the bracket body for stepsToText: ""
// for a nil predicate (ungated), "always", or "step N condition [value]".
func formatStepCondition(c *ctxforge.StepCondition) string {
	if c == nil {
		return ""
	}
	if c.Condition == ctxforge.CondAlways {
		return ctxforge.CondAlways
	}
	s := fmt.Sprintf("step %d %s", c.Step, c.Condition)
	if c.Condition == ctxforge.CondOutputContains && c.Value != "" {
		s += " " + c.Value
	}
	return s
}

// parseStepCondition parses the bracket body from a step line back into a predicate
// ("always", or "step N condition [value...]"). A malformed body is kept as a raw
// condition so the whole-forge Validate surfaces the error rather than silently
// dropping the predicate. An empty body yields nil (ungated).
func parseStepCondition(spec string) *ctxforge.StepCondition {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil
	}
	if spec == ctxforge.CondAlways {
		return &ctxforge.StepCondition{Condition: ctxforge.CondAlways}
	}
	fields := strings.Fields(spec)
	if len(fields) >= 3 && fields[0] == "step" {
		n, _ := strconv.Atoi(fields[1])
		return &ctxforge.StepCondition{
			Step:      n,
			Condition: fields[2],
			Value:     strings.TrimSpace(strings.Join(fields[3:], " ")),
		}
	}
	return &ctxforge.StepCondition{Condition: spec}
}

func workflowFromForm(r *http.Request, id string) ctxforge.Workflow {
	return ctxforge.Workflow{
		ID:          id,
		Name:        strings.TrimSpace(r.FormValue("name")),
		Description: strings.TrimSpace(r.FormValue("description")),
		Mode:        strings.TrimSpace(r.FormValue("mode")),
		Steps:       stepsFromText(r.FormValue("steps")),
	}
}

func (s *Server) handleWorkflowNew(w http.ResponseWriter, r *http.Request) {
	s.writePartial(w, renderWorkflowForm(ctxforge.Workflow{Mode: ctxforge.WorkflowSequential}, true, ""))
}

func (s *Server) handleWorkflowEdit(w http.ResponseWriter, r *http.Request) {
	s.hub.forgeMu.Lock()
	wf := s.forge.Workflow(r.PathValue("id"))
	var form string
	if wf != nil {
		form = renderWorkflowForm(*wf, false, "")
	}
	s.hub.forgeMu.Unlock()
	if form == "" {
		s.writePartial(w, s.workflowsPartial())
		return
	}
	s.writePartial(w, form)
}

func (s *Server) handleWorkflowCreate(w http.ResponseWriter, r *http.Request) {
	wf := workflowFromForm(r, strings.TrimSpace(r.FormValue("id")))
	if err := s.editForge(func() error { return s.forge.AddWorkflow(wf) }); err != nil {
		s.writePartial(w, renderWorkflowForm(wf, true, err.Error()))
		return
	}
	s.writePartial(w, s.workflowsPartial())
}

func (s *Server) handleWorkflowUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wf := workflowFromForm(r, id)
	if err := s.editForge(func() error { return s.forge.UpdateWorkflow(id, wf) }); err != nil {
		s.writePartial(w, renderWorkflowForm(wf, false, err.Error()))
		return
	}
	s.writePartial(w, s.workflowsPartial())
}

func (s *Server) handleWorkflowDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.editForge(func() error { return s.forge.RemoveWorkflow(r.PathValue("id")) }); err != nil {
		s.logger.Printf("remove workflow: %v", err)
	}
	s.writePartial(w, s.workflowsPartial())
}
