package web

import (
	"net/http"
	"strings"

	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
)

// This file is the Snippets library: saved, reusable composer prompts (item 3.4,
// ADR-0015). It mirrors the forge-CRUD pattern (validated builders,
// rollback-on-invalid save, re-render the form in place on error) used for
// skills/instructions/agents, minus the enable/toggle — a snippet is never
// compiled into a session, only inserted into the composer. Insertion is surfaced
// through the slash autocomplete (autocomplete.go): picking a snippet inserts its
// body client-side (fillSnippet), and submitting a bare "/trigger" expands and
// sends it (snippetExpansion, used by handleSend).

// snippetsPartial renders the Snippets library list.
func (s *Server) snippetsPartial() string {
	s.hub.forgeMu.Lock()
	defer s.hub.forgeMu.Unlock()
	rows := make([]map[string]any, 0, len(s.forge.Snippets))
	for _, sn := range s.forge.Snippets {
		rows = append(rows, map[string]any{
			"ID": sn.ID, "Name": sn.Name, "Desc": truncate(sn.Body, 80),
		})
	}
	return frag("snippetsPage", map[string]any{"Add": addData("snippets", "snippet"), "Rows": rows})
}

// snippetExpansion resolves a bare "/trigger" composer submission to the snippet
// body to send. It returns ok=false when args are present (the leading token is
// being used as a command with arguments), when the name is a reserved command/
// page slug (the command always wins), or when no snippet matches — so handleSend
// falls back to the normal command path in those cases.
func (s *Server) snippetExpansion(name, args string) (string, bool) {
	if args != "" || isReservedCommand(name) || s.forge == nil {
		return "", false
	}
	s.hub.forgeMu.Lock()
	defer s.hub.forgeMu.Unlock()
	if sn := s.forge.Snippet(name); sn != nil {
		return sn.Body, true
	}
	return "", false
}

// --- snippet form ---

func renderSnippetForm(sn ctxforge.Snippet, isNew bool, errMsg string) string {
	title, action := "Edit snippet", "/snippets/"+sn.ID
	if isNew {
		title, action = "New snippet", "/snippets"
	}
	return formShell(title, action, "snippets", errMsg,
		idField(sn.ID, isNew),
		textField("Name", "name", sn.Name, true),
		textArea("Body (the prompt inserted into the composer)", "body", sn.Body, true),
	)
}

func (s *Server) handleSnippetNew(w http.ResponseWriter, r *http.Request) {
	s.writePartial(w, renderSnippetForm(ctxforge.Snippet{}, true, ""))
}

func (s *Server) handleSnippetEdit(w http.ResponseWriter, r *http.Request) {
	s.hub.forgeMu.Lock()
	sn := s.forge.Snippet(r.PathValue("id"))
	var form string
	if sn != nil {
		form = renderSnippetForm(*sn, false, "")
	}
	s.hub.forgeMu.Unlock()
	if form == "" {
		s.writePartial(w, s.snippetsPartial())
		return
	}
	s.writePartial(w, form)
}

func snippetFromForm(r *http.Request, id string) ctxforge.Snippet {
	return ctxforge.Snippet{
		ID:   id,
		Name: strings.TrimSpace(r.FormValue("name")),
		Body: strings.TrimSpace(r.FormValue("body")),
	}
}

func (s *Server) handleSnippetCreate(w http.ResponseWriter, r *http.Request) {
	sn := snippetFromForm(r, strings.TrimSpace(r.FormValue("id")))
	if err := s.editForge(func() error { return s.forge.AddSnippet(sn) }); err != nil {
		s.writePartial(w, renderSnippetForm(sn, true, err.Error()))
		return
	}
	s.writePartial(w, s.snippetsPartial())
}

func (s *Server) handleSnippetUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sn := snippetFromForm(r, id)
	if err := s.editForge(func() error { return s.forge.UpdateSnippet(id, sn) }); err != nil {
		s.writePartial(w, renderSnippetForm(sn, false, err.Error()))
		return
	}
	s.writePartial(w, s.snippetsPartial())
}

func (s *Server) handleSnippetDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.editForge(func() error { return s.forge.RemoveSnippet(r.PathValue("id")) }); err != nil {
		s.logger.Printf("remove snippet: %v", err)
	}
	s.writePartial(w, s.snippetsPartial())
}
