// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package web

import (
	"net/http"
	"strings"

	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
)

// forgeCRUD generically drives the add/edit form lifecycle shared by the forge
// entities (skills, instructions, agents, snippets): render an empty form (New),
// render a populated form or fall back to the list (Edit), and save-or-re-render
// on a validation error (Create/Update/Delete). Each entity supplies only its
// per-type hooks; the lock/lookup/fallback and error-re-render logic — the
// fiddly, easy-to-skew parts — live here once. Saves go through the validated
// ctxforge builders via editForge, so an invalid edit is rolled back, never
// persisted. Routes stay (*Server).handle… (see hub.go); the thin per-entity
// methods just delegate here.
type forgeCRUD[T any] struct {
	kind     string                                 // list slug, for the delete log line
	lookup   func(*ctxforge.Forge, string) *T       // find by id
	render   func(T, bool, string) string           // render form: (entity, isNew, errMsg)
	blank    func() T                               // the empty entity a "New" form starts from
	fromForm func(*http.Request, string) T          // build an entity from the posted form + id
	add      func(*ctxforge.Forge, T) error         // validated create builder
	update   func(*ctxforge.Forge, string, T) error // validated update builder
	remove   func(*ctxforge.Forge, string) error    // validated delete builder
	list     func(*Server) string                   // re-render the entity's list partial
}

// New renders an empty create form.
func (c forgeCRUD[T]) New(s *Server, w http.ResponseWriter, r *http.Request) {
	s.writePartial(w, c.render(c.blank(), true, ""))
}

// Edit renders a populated edit form, falling back to the list when the id is
// unknown (e.g. a stale link after a delete).
func (c forgeCRUD[T]) Edit(s *Server, w http.ResponseWriter, r *http.Request) {
	s.hub.forgeMu.Lock()
	e := c.lookup(s.forge, r.PathValue("id"))
	var form string
	if e != nil {
		form = c.render(*e, false, "")
	}
	s.hub.forgeMu.Unlock()
	if form == "" {
		s.writePartial(w, c.list(s))
		return
	}
	s.writePartial(w, form)
}

// Create adds a new entity, re-rendering the form in place with the error on a
// failed validation, else the updated list.
func (c forgeCRUD[T]) Create(s *Server, w http.ResponseWriter, r *http.Request) {
	e := c.fromForm(r, strings.TrimSpace(r.FormValue("id")))
	if err := s.editForge(func() error { return c.add(s.forge, e) }); err != nil {
		s.writePartial(w, c.render(e, true, err.Error()))
		return
	}
	s.writePartial(w, c.list(s))
}

// Update replaces an entity by id, re-rendering the form with the error on a
// failed validation, else the updated list.
func (c forgeCRUD[T]) Update(s *Server, w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	e := c.fromForm(r, id)
	if err := s.editForge(func() error { return c.update(s.forge, id, e) }); err != nil {
		s.writePartial(w, c.render(e, false, err.Error()))
		return
	}
	s.writePartial(w, c.list(s))
}

// Delete removes an entity by id (a failure — e.g. a still-referenced skill — is
// logged, not surfaced) and re-renders the list.
func (c forgeCRUD[T]) Delete(s *Server, w http.ResponseWriter, r *http.Request) {
	if err := s.editForge(func() error { return c.remove(s.forge, r.PathValue("id")) }); err != nil {
		s.logger.Printf("remove %s: %v", c.kind, err)
	}
	s.writePartial(w, c.list(s))
}

// The per-entity CRUD descriptors. Method expressions (e.g. (*ctxforge.Forge).Skill)
// bind the validated builders without per-entity wrapper closures.
var (
	skillCRUD = forgeCRUD[ctxforge.Skill]{
		kind:   "skill",
		lookup: (*ctxforge.Forge).Skill, render: renderSkillForm,
		blank:    func() ctxforge.Skill { return ctxforge.Skill{Enabled: true} },
		fromForm: skillFromForm,
		add:      (*ctxforge.Forge).AddSkill, update: (*ctxforge.Forge).UpdateSkill,
		remove: (*ctxforge.Forge).RemoveSkill, list: (*Server).skillsPartial,
	}
	instructionCRUD = forgeCRUD[ctxforge.Instruction]{
		kind:   "instruction",
		lookup: (*ctxforge.Forge).Instruction, render: renderInstructionForm,
		blank:    func() ctxforge.Instruction { return ctxforge.Instruction{Enabled: true} },
		fromForm: instructionFromForm,
		add:      (*ctxforge.Forge).AddInstruction, update: (*ctxforge.Forge).UpdateInstruction,
		remove: (*ctxforge.Forge).RemoveInstruction, list: (*Server).instructionsPartial,
	}
	agentCRUD = forgeCRUD[ctxforge.Agent]{
		kind:   "agent",
		lookup: (*ctxforge.Forge).Agent, render: renderAgentForm,
		blank:    func() ctxforge.Agent { return ctxforge.Agent{ReasoningEffort: "medium"} },
		fromForm: agentFromForm,
		add:      (*ctxforge.Forge).AddAgent, update: (*ctxforge.Forge).UpdateAgent,
		remove: (*ctxforge.Forge).RemoveAgent, list: (*Server).agentsPartial,
	}
	snippetCRUD = forgeCRUD[ctxforge.Snippet]{
		kind:   "snippet",
		lookup: (*ctxforge.Forge).Snippet, render: renderSnippetForm,
		blank:    func() ctxforge.Snippet { return ctxforge.Snippet{} },
		fromForm: snippetFromForm,
		add:      (*ctxforge.Forge).AddSnippet, update: (*ctxforge.Forge).UpdateSnippet,
		remove: (*ctxforge.Forge).RemoveSnippet, list: (*Server).snippetsPartial,
	}
	workflowCRUD = forgeCRUD[ctxforge.Workflow]{
		kind:   "workflow",
		lookup: (*ctxforge.Forge).Workflow, render: renderWorkflowForm,
		blank:    func() ctxforge.Workflow { return ctxforge.Workflow{Mode: ctxforge.WorkflowSequential} },
		fromForm: workflowFromForm,
		add:      (*ctxforge.Forge).AddWorkflow, update: (*ctxforge.Forge).UpdateWorkflow,
		remove: (*ctxforge.Forge).RemoveWorkflow, list: (*Server).workflowsPartial,
	}
)
