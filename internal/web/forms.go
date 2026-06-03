package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
)

// This file renders and processes the add/edit forms for the forge entities
// (skills, instructions, agents). Creation and edits reuse the validated
// ctxforge builders (AddSkill/UpdateSkill/…), which roll back on an invalid
// result, so the web layer never persists a malformed forge. On a validation
// error the form is re-rendered in place with the message instead of the list.

// --- field helpers ---

func textField(label, name, value string, required bool) string {
	req := ""
	if required {
		req = " required"
	}
	return fmt.Sprintf(
		`<label class="field"><span>%s</span><input type="text" name="%s" value="%s" autocomplete="off"%s></label>`,
		esc(label), name, attr(value), req)
}

func textArea(label, name, value string, required bool) string {
	req := ""
	if required {
		req = " required"
	}
	return fmt.Sprintf(
		`<label class="field"><span>%s</span><textarea name="%s" rows="4"%s>%s</textarea></label>`,
		esc(label), name, req, esc(value))
}

func numberField(label, name string, value int) string {
	return fmt.Sprintf(
		`<label class="field"><span>%s</span><input type="number" name="%s" value="%d"></label>`,
		esc(label), name, value)
}

func checkboxField(label, name string, on bool) string {
	ck := ""
	if on {
		ck = " checked"
	}
	return fmt.Sprintf(
		`<label class="field check"><input type="checkbox" name="%s"%s><span>%s</span></label>`,
		name, ck, esc(label))
}

func selectField(label, name, value string, opts []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<label class="field"><span>%s</span><select name="%s">`, esc(label), name)
	for _, o := range opts {
		sel := ""
		if o == value {
			sel = " selected"
		}
		fmt.Fprintf(&b, `<option value="%s"%s>%s</option>`, attr(o), sel, esc(o))
	}
	b.WriteString(`</select></label>`)
	return b.String()
}

// idField renders the slug input: editable when creating, read-only when editing
// (renames would orphan references; toggle/delete cover lifecycle instead).
func idField(value string, isNew bool) string {
	if isNew {
		return textField("ID (slug)", "id", value, true)
	}
	return fmt.Sprintf(
		`<label class="field"><span>ID (slug)</span><input type="text" name="id" value="%s" readonly></label>`,
		attr(value))
}

// attr escapes a value for an HTML attribute (no <br> conversion, unlike esc).
func attr(s string) string {
	return strings.NewReplacer(`&`, "&amp;", `"`, "&quot;", `<`, "&lt;", `>`, "&gt;").Replace(s)
}

// formShell wraps fields in a <form> posting to action, plus a save/cancel row.
// kind is the list slug used to cancel back to the list page.
func formShell(title, action, kind, errMsg string, fields ...string) string {
	var b strings.Builder
	b.WriteString(`<section class="page"><h2>` + esc(title) + `</h2>`)
	if errMsg != "" {
		b.WriteString(`<p class="error">⚠ ` + esc(errMsg) + `</p>`)
	}
	fmt.Fprintf(&b, `<form class="forge-form" hx-post="%s" hx-target="#main" hx-swap="innerHTML">`, action)
	for _, f := range fields {
		b.WriteString(f)
	}
	b.WriteString(`<div class="form-actions"><button type="submit">Save</button>`)
	fmt.Fprintf(&b, `<button type="button" class="cancel" hx-get="/page/%s" hx-target="#main" hx-swap="innerHTML">Cancel</button>`, kind)
	b.WriteString(`</div></form></section>`)
	return b.String()
}

// --- skill form ---

func renderSkillForm(s ctxforge.Skill, isNew bool, errMsg string) string {
	title, action := "Edit skill", "/skills/"+s.ID
	if isNew {
		title, action = "New skill", "/skills"
	}
	return formShell(title, action, "skills", errMsg,
		idField(s.ID, isNew),
		textField("Name", "name", s.Name, true),
		textField("Description", "description", s.Description, false),
		textArea("Prompt", "prompt", s.Prompt, true),
		textField("Slash command", "command", s.Command, false),
		checkboxField("Enabled", "enabled", s.Enabled),
	)
}

func (s *Server) handleSkillNew(w http.ResponseWriter, r *http.Request) {
	s.writePartial(w, renderSkillForm(ctxforge.Skill{Enabled: true}, true, ""))
}

func (s *Server) handleSkillEdit(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	sk := s.forge.Skill(r.PathValue("id"))
	var form string
	if sk != nil {
		form = renderSkillForm(*sk, false, "")
	}
	s.mu.Unlock()
	if form == "" {
		s.writePartial(w, s.skillsPartial())
		return
	}
	s.writePartial(w, form)
}

func skillFromForm(r *http.Request, id string) ctxforge.Skill {
	return ctxforge.Skill{
		ID:          id,
		Name:        strings.TrimSpace(r.FormValue("name")),
		Description: strings.TrimSpace(r.FormValue("description")),
		Prompt:      strings.TrimSpace(r.FormValue("prompt")),
		Command:     strings.TrimSpace(r.FormValue("command")),
		Enabled:     r.FormValue("enabled") != "",
	}
}

func (s *Server) handleSkillCreate(w http.ResponseWriter, r *http.Request) {
	sk := skillFromForm(r, strings.TrimSpace(r.FormValue("id")))
	if err := s.editForge(func() error { return s.forge.AddSkill(sk) }); err != nil {
		s.writePartial(w, renderSkillForm(sk, true, err.Error()))
		return
	}
	s.writePartial(w, s.skillsPartial())
}

func (s *Server) handleSkillUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sk := skillFromForm(r, id)
	if err := s.editForge(func() error { return s.forge.UpdateSkill(id, sk) }); err != nil {
		s.writePartial(w, renderSkillForm(sk, false, err.Error()))
		return
	}
	s.writePartial(w, s.skillsPartial())
}

// --- instruction form ---

func renderInstructionForm(in ctxforge.Instruction, isNew bool, errMsg string) string {
	title, action := "Edit instruction", "/instructions/"+in.ID
	if isNew {
		title, action = "New instruction", "/instructions"
	}
	return formShell(title, action, "instructions", errMsg,
		idField(in.ID, isNew),
		textField("Title", "title", in.Title, false),
		textArea("Body", "body", in.Body, true),
		numberField("Priority", "priority", in.Priority),
		checkboxField("Enabled", "enabled", in.Enabled),
	)
}

func (s *Server) handleInstructionNew(w http.ResponseWriter, r *http.Request) {
	s.writePartial(w, renderInstructionForm(ctxforge.Instruction{Enabled: true}, true, ""))
}

func (s *Server) handleInstructionEdit(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	in := s.forge.Instruction(r.PathValue("id"))
	var form string
	if in != nil {
		form = renderInstructionForm(*in, false, "")
	}
	s.mu.Unlock()
	if form == "" {
		s.writePartial(w, s.instructionsPartial())
		return
	}
	s.writePartial(w, form)
}

func instructionFromForm(r *http.Request, id string) ctxforge.Instruction {
	prio, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("priority")))
	return ctxforge.Instruction{
		ID:       id,
		Title:    strings.TrimSpace(r.FormValue("title")),
		Body:     strings.TrimSpace(r.FormValue("body")),
		Priority: prio,
		Enabled:  r.FormValue("enabled") != "",
	}
}

func (s *Server) handleInstructionCreate(w http.ResponseWriter, r *http.Request) {
	in := instructionFromForm(r, strings.TrimSpace(r.FormValue("id")))
	if err := s.editForge(func() error { return s.forge.AddInstruction(in) }); err != nil {
		s.writePartial(w, renderInstructionForm(in, true, err.Error()))
		return
	}
	s.writePartial(w, s.instructionsPartial())
}

func (s *Server) handleInstructionUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	in := instructionFromForm(r, id)
	if err := s.editForge(func() error { return s.forge.UpdateInstruction(id, in) }); err != nil {
		s.writePartial(w, renderInstructionForm(in, false, err.Error()))
		return
	}
	s.writePartial(w, s.instructionsPartial())
}

// --- agent form ---

var reasoningOpts = []string{"low", "medium", "high", "xhigh"}

func renderAgentForm(a ctxforge.Agent, isNew bool, errMsg string) string {
	title, action := "Edit agent", "/agents/"+a.ID
	if isNew {
		title, action = "New agent", "/agents"
	}
	effort := a.ReasoningEffort
	if effort == "" {
		effort = "medium"
	}
	return formShell(title, action, "agents", errMsg,
		idField(a.ID, isNew),
		textField("Name", "name", a.Name, false),
		textField("Description", "description", a.Description, false),
		textField("Model", "model", a.Model, true),
		selectField("Reasoning effort", "reasoningEffort", effort, reasoningOpts),
		textArea("System message", "systemMessage", a.SystemMessage, false),
		textField("Skills (comma-separated IDs)", "skills", strings.Join(a.Skills, ", "), false),
	)
}

func (s *Server) handleAgentNew(w http.ResponseWriter, r *http.Request) {
	s.writePartial(w, renderAgentForm(ctxforge.Agent{ReasoningEffort: "medium"}, true, ""))
}

func (s *Server) handleAgentEdit(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	a := s.forge.Agent(r.PathValue("id"))
	var form string
	if a != nil {
		form = renderAgentForm(*a, false, "")
	}
	s.mu.Unlock()
	if form == "" {
		s.writePartial(w, s.agentsPartial())
		return
	}
	s.writePartial(w, form)
}

func agentFromForm(r *http.Request, id string) ctxforge.Agent {
	return ctxforge.Agent{
		ID:              id,
		Name:            strings.TrimSpace(r.FormValue("name")),
		Description:     strings.TrimSpace(r.FormValue("description")),
		Model:           strings.TrimSpace(r.FormValue("model")),
		ReasoningEffort: strings.TrimSpace(r.FormValue("reasoningEffort")),
		SystemMessage:   strings.TrimSpace(r.FormValue("systemMessage")),
		Skills:          parseCSV(r.FormValue("skills")),
	}
}

func (s *Server) handleAgentCreate(w http.ResponseWriter, r *http.Request) {
	a := agentFromForm(r, strings.TrimSpace(r.FormValue("id")))
	if err := s.editForge(func() error { return s.forge.AddAgent(a) }); err != nil {
		s.writePartial(w, renderAgentForm(a, true, err.Error()))
		return
	}
	s.writePartial(w, s.agentsPartial())
}

func (s *Server) handleAgentUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a := agentFromForm(r, id)
	if err := s.editForge(func() error { return s.forge.UpdateAgent(id, a) }); err != nil {
		s.writePartial(w, renderAgentForm(a, false, err.Error()))
		return
	}
	s.writePartial(w, s.agentsPartial())
}

// parseCSV splits a comma-separated list into trimmed, non-empty entries.
func parseCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
