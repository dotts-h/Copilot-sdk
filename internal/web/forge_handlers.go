package web

import (
	"net/http"

	"github.com/dotts-h/copilot-sdk/internal/config"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// This file holds the forge/config-mutation list-page handlers lifted out of the
// god file (0088): the skill/instruction toggle+delete, agent select+delete, and
// model/effort select handlers. All forge/config mutation goes through editForge
// or editConfig (snapshot → mutate → validating Save → roll back on failure) so it
// is serialized against the readers in pages.go and never half-applied. The Server
// struct + send/budget core stay in server.go.

func (s *Server) handleSkillToggle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_ = s.editForge(func() error { _, err := s.forge.ToggleSkill(id); return err })
	s.writePartial(w, s.skillsPartial())
}

func (s *Server) handleSkillDelete(w http.ResponseWriter, r *http.Request) {
	skillCRUD.Delete(s, w, r) // a failure (e.g. an agent still pins it) is logged
}

func (s *Server) handleInstructionToggle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_ = s.editForge(func() error { _, err := s.forge.ToggleInstruction(id); return err })
	s.writePartial(w, s.instructionsPartial())
}

func (s *Server) handleInstructionDelete(w http.ResponseWriter, r *http.Request) {
	instructionCRUD.Delete(s, w, r)
}

func (s *Server) handleAgentSelect(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// Persist the selection through editConfig (snapshot → mutate → validating
	// Save → roll back on failure) so a disk error can't leave the live config
	// pointing at an agent that was never written — the same discipline every
	// other config mutation uses (REGRESSIONS: "edit through Server.editConfig").
	var compileID string
	if err := s.editConfig(func(c *config.Config) {
		if c.DefaultAgent == id {
			c.DefaultAgent = "" // toggle off → no agent persona
		} else {
			c.DefaultAgent = id
		}
		compileID = c.DefaultAgent
	}); err != nil {
		s.logger.Printf("save config: %v", err)
		s.writePartial(w, s.agentsPartial()) // selection didn't persist; leave the session as-is
		return
	}
	s.hub.forgeMu.Lock()
	c := s.compiledSpec(compileID)
	// Capture the selected persona's leash + label under forgeMu so applyAgentSpec can
	// snapshot it under s.mu (issue 0072); toggle-off (compileID "") leaves it inert.
	var leash telemetry.Leash
	var leashLabel string
	if ag := s.forge.Agent(compileID); ag != nil {
		leash = telemetry.Leash{MaxCredits: ag.MaxCredits, MaxTurns: ag.MaxTurns}
		leashLabel = ag.Name
	}
	s.hub.forgeMu.Unlock()
	// Compile the agent's full persona (system message + instructions + skill
	// prompts) plus model/effort/tool-allowlist + enabled MCP servers into the live
	// spec and restart the session so the selection takes effect on the next prompt.
	s.applyAgentSpec(c, compileID, leash, leashLabel)
	s.writePartial(w, s.agentsPartial())
}

// handleModelSelect switches the active model from the model picker and
// re-renders the page with the new current marked.
func (s *Server) handleModelSelect(w http.ResponseWriter, r *http.Request) {
	s.setModel(r.PathValue("id")) // self-locks per-session + shared state
	s.writePartial(w, s.modelsPartial())
}

// handleEffortSelect sets the reasoning effort from the Models page and
// re-renders it with the new effort marked. "default" clears it.
func (s *Server) handleEffortSelect(w http.ResponseWriter, r *http.Request) {
	v := r.PathValue("value")
	if v == "default" {
		v = ""
	}
	s.setEffort(v) // self-locks per-session + shared state
	s.writePartial(w, s.modelsPartial())
}

func (s *Server) handleAgentDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.editForge(func() error { return s.forge.RemoveAgent(id) }); err != nil {
		s.logger.Printf("remove agent: %v", err)
		s.writePartial(w, s.agentsPartial())
		return
	}
	// Deleting the active agent clears the config pointer to it — through
	// editConfig so a Save failure rolls back rather than leaving the live config
	// dangling at a now-deleted agent (matches every other config mutation).
	s.hub.forgeMu.Lock()
	wasActive := s.config.DefaultAgent == id
	s.hub.forgeMu.Unlock()
	if wasActive {
		if err := s.editConfig(func(c *config.Config) { c.DefaultAgent = "" }); err != nil {
			s.logger.Printf("save config: %v", err)
		}
	}
	s.writePartial(w, s.agentsPartial())
}
