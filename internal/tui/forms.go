package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"

	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
)

// formKind selects which forge entity a form edits.
type formKind int

const (
	formSkill formKind = iota
	formInstruction
	formAgent
)

// formField is one labeled text input in a form.
type formField struct {
	key      string
	label    string
	help     string
	input    textinput.Model
	required bool
}

// forgeForm is a modal editor for a single forge entity. editIndex is -1 for a
// new entity, or the slice index being edited.
type forgeForm struct {
	kind      formKind
	editIndex int
	fields    []formField
	focus     int
	// enabled carries the prior Enabled state when editing (forms don't expose
	// a checkbox; toggling stays on the list page).
	enabled bool
}

func newField(key, label, help string, required bool, value string) formField {
	ti := textinput.New()
	ti.SetValue(value)
	ti.Prompt = ""
	ti.CharLimit = 4096
	return formField{key: key, label: label, help: help, input: ti, required: required}
}

// newSkillForm builds a form for adding (s == nil) or editing a skill.
func newSkillForm(s *ctxforge.Skill, index int) *forgeForm {
	var id, name, desc, prompt, cmd string
	enabled := true
	if s != nil {
		id, name, desc, prompt, cmd, enabled = s.ID, s.Name, s.Description, s.Prompt, s.Command, s.Enabled
	}
	f := &forgeForm{kind: formSkill, editIndex: index, enabled: enabled, fields: []formField{
		newField("id", "ID", "lowercase slug, e.g. tdd", true, id),
		newField("name", "Name", "", true, name),
		newField("description", "Description", "", false, desc),
		newField("prompt", "Prompt", "injected into the system message", true, prompt),
		newField("command", "Slash command", "optional, no leading /", false, cmd),
	}}
	f.fields[0].input.Focus()
	return f
}

// newInstructionForm builds a form for an instruction.
func newInstructionForm(in *ctxforge.Instruction, index int) *forgeForm {
	var id, title, body, prio string
	enabled := true
	if in != nil {
		id, title, body, prio, enabled = in.ID, in.Title, in.Body, strconv.Itoa(in.Priority), in.Enabled
	}
	f := &forgeForm{kind: formInstruction, editIndex: index, enabled: enabled, fields: []formField{
		newField("id", "ID", "lowercase slug", true, id),
		newField("title", "Title", "", false, title),
		newField("body", "Body", "the rule text", true, body),
		newField("priority", "Priority", "integer; lower compiles first", false, prio),
	}}
	f.fields[0].input.Focus()
	return f
}

// newAgentForm builds a form for an agent.
func newAgentForm(a *ctxforge.Agent, index int) *forgeForm {
	var id, name, desc, model, effort, system, skills string
	if a != nil {
		id, name, desc, model, effort, system = a.ID, a.Name, a.Description, a.Model, a.ReasoningEffort, a.SystemMessage
		skills = strings.Join(a.Skills, ", ")
	}
	f := &forgeForm{kind: formAgent, editIndex: index, fields: []formField{
		newField("id", "ID", "lowercase slug", true, id),
		newField("name", "Name", "", true, name),
		newField("description", "Description", "", false, desc),
		newField("model", "Model", "e.g. gpt-5, claude-opus-4.7", true, model),
		newField("effort", "Reasoning effort", "low|medium|high|xhigh", false, effort),
		newField("system", "System message", "optional persona", false, system),
		newField("skills", "Pinned skills", "comma-separated skill IDs", false, skills),
	}}
	f.fields[0].input.Focus()
	return f
}

// value returns the trimmed value of the field with the given key.
func (f *forgeForm) value(key string) string {
	for i := range f.fields {
		if f.fields[i].key == key {
			return strings.TrimSpace(f.fields[i].input.Value())
		}
	}
	return ""
}

// moveFocus advances focus by delta with wraparound and updates input focus.
func (f *forgeForm) moveFocus(delta int) {
	f.fields[f.focus].input.Blur()
	n := len(f.fields)
	f.focus = (f.focus + delta%n + n) % n
	f.fields[f.focus].input.Focus()
}

// title renders the form heading.
func (f *forgeForm) title() string {
	verb := "New"
	if f.editIndex >= 0 {
		verb = "Edit"
	}
	switch f.kind {
	case formSkill:
		return verb + " skill"
	case formInstruction:
		return verb + " instruction"
	case formAgent:
		return verb + " agent"
	}
	return verb
}

// --- pure builders (validated) -------------------------------------------

func buildSkill(id, name, desc, prompt, command string, enabled bool) (ctxforge.Skill, error) {
	s := ctxforge.Skill{ID: id, Name: name, Description: desc, Prompt: prompt, Command: command, Enabled: enabled}
	return s, s.Validate()
}

func buildInstruction(id, title, body, priority string, enabled bool) (ctxforge.Instruction, error) {
	p := 0
	if priority != "" {
		v, err := strconv.Atoi(priority)
		if err != nil {
			return ctxforge.Instruction{}, fmt.Errorf("priority must be an integer: %q", priority)
		}
		p = v
	}
	in := ctxforge.Instruction{ID: id, Title: title, Body: body, Priority: p, Enabled: enabled}
	return in, in.Validate()
}

func buildAgent(id, name, desc, model, effort, system, skillsCSV string) (ctxforge.Agent, error) {
	var skills []string
	for _, s := range strings.Split(skillsCSV, ",") {
		if s = strings.TrimSpace(s); s != "" {
			skills = append(skills, s)
		}
	}
	a := ctxforge.Agent{
		ID: id, Name: name, Description: desc, Model: model,
		ReasoningEffort: effort, SystemMessage: system, Skills: skills,
	}
	return a, a.Validate()
}

// build constructs the entity from the form's current field values.
func (f *forgeForm) build() (any, error) {
	switch f.kind {
	case formSkill:
		return buildSkill(f.value("id"), f.value("name"), f.value("description"),
			f.value("prompt"), f.value("command"), f.enabled)
	case formInstruction:
		return buildInstruction(f.value("id"), f.value("title"), f.value("body"),
			f.value("priority"), f.enabled)
	case formAgent:
		return buildAgent(f.value("id"), f.value("name"), f.value("description"),
			f.value("model"), f.value("effort"), f.value("system"), f.value("skills"))
	}
	return nil, fmt.Errorf("unknown form kind")
}
