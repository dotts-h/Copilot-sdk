package web

import (
	"context"
	"fmt"
	"strings"

	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
)

func (s *Server) skillsPartial() string {
	s.hub.forgeMu.Lock()
	defer s.hub.forgeMu.Unlock()
	rows := make([]map[string]any, 0, len(s.forge.Skills))
	for _, sk := range s.forge.Skills {
		rows = append(rows, map[string]any{
			"Kind": "skills", "ID": sk.ID, "On": sk.Enabled, "Name": sk.Name, "Desc": truncate(sk.Description, 80),
		})
	}
	return frag("skillsPage", map[string]any{"Add": addData("skills", "skill"), "Rows": rows})
}

func (s *Server) instructionsPartial() string {
	s.hub.forgeMu.Lock()
	defer s.hub.forgeMu.Unlock()
	rows := make([]map[string]any, 0, len(s.forge.Instructions))
	for _, ins := range s.forge.Instructions {
		rows = append(rows, map[string]any{
			"Kind": "instructions", "ID": ins.ID, "On": ins.Enabled,
			"Name": fmt.Sprintf("%s (p%d)", ins.Title, ins.Priority), "Desc": truncate(ins.Body, 80),
		})
	}
	return frag("instructionsPage", map[string]any{"Add": addData("instructions", "instruction"), "Rows": rows})
}

func (s *Server) agentsPartial() string {
	s.hub.forgeMu.Lock()
	defer s.hub.forgeMu.Unlock()
	rows := make([]map[string]any, 0, len(s.forge.Agents)+1)
	for _, a := range s.forge.Agents {
		desc := fmt.Sprintf("%s · %s · %s", a.Model, def(a.ReasoningEffort, "medium"), a.Description)
		rows = append(rows, map[string]any{
			"ID": a.ID, "Active": a.ID == s.config.DefaultAgent, "Name": a.Name, "Desc": desc,
		})
	}
	// Always offer the built-in chat agent (unless the forge defines its own), so
	// chat has a baseline persona with no config. It is virtual: selectable but
	// not editable/deletable (ADR 0003).
	if !s.forge.HasOwnChatAgent() {
		b := ctxforge.DefaultChatAgent()
		rows = append(rows, map[string]any{
			"ID": b.ID, "Active": b.ID == s.config.DefaultAgent, "Name": b.Name + " (built-in)",
			"Desc": b.Description, "Builtin": true,
		})
	}
	return frag("agentsPage", map[string]any{"Add": addData("agents", "agent"), "Rows": rows})
}

// modelsPartial renders the model picker: every model the account can use, with
// the current one marked and the rest offering a one-click switch (POST
// /models/{id}/select, which restarts the session — workstream 3). It degrades
// to a notice when the runtime can't list models.
func (s *Server) modelsPartial() string {
	models, err := s.client.ListModels(context.Background())
	if err != nil {
		return frag("modelsPage", map[string]any{"Err": err.Error()})
	}

	s.mu.Lock()
	current := s.spec.Model
	curEffort := s.spec.ReasoningEffort
	s.mu.Unlock()

	rows := make([]map[string]any, 0, len(models))
	var efforts []map[string]any
	for _, m := range models {
		desc := m.ID
		if e := strings.Join(m.SupportedReasoningEfforts, ", "); e != "" {
			desc += " · reasoning: " + e
		}
		rows = append(rows, map[string]any{
			"ID": m.ID, "Active": m.ID == current, "Name": def(m.Name, m.ID), "Desc": desc,
		})
		if m.ID == current {
			for _, e := range m.SupportedReasoningEfforts {
				efforts = append(efforts, map[string]any{"Value": e, "Active": e == curEffort})
			}
		}
	}
	return frag("modelsPage", map[string]any{"Rows": rows, "Efforts": efforts})
}

func (s *Server) settingsPartial() string {
	return s.renderSettings("", "")
}
