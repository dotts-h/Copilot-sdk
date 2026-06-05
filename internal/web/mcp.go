package web

import (
	"net/http"
	"strings"

	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
)

// MCPServerSpecs converts the forge's compiled (enabled) MCP servers into the
// seam type the session spec carries, preserving the unique id so the runtime
// keys them safely. It is the single forge→seam translation for MCP, shared by
// the session-startup path (bootstrap) and the agent-restart path (compiledSpec).
func MCPServerSpecs(servers []ctxforge.MCPServer) []copilot.MCPServer {
	if len(servers) == 0 {
		return nil
	}
	out := make([]copilot.MCPServer, 0, len(servers))
	for _, m := range servers {
		out = append(out, copilot.MCPServer{
			ID: m.ID, Name: m.Name, Command: m.Command, Args: m.Args, Env: m.Env,
		})
	}
	return out
}

// This file is the MCP-server management page: the one forge entity that had no
// UI. It mirrors the forge-CRUD pattern (validated builders, rollback-on-invalid
// save, re-render the form in place on error) used for skills/instructions/agents,
// and adds a PATH preflight (exec.LookPath, behind the s.lookPath seam) so a
// curated stdio server whose binary is absent is flagged in the UI rather than
// surprise-failing when the session starts. Secrets/Env are intentionally not
// edited here (see ADR-0010); an existing Env is preserved across edits.

// mcpServersPartial renders the MCP-server list with each row's preflight state.
// It snapshots the servers under forgeMu and releases the lock *before* the
// per-server PATH preflight, so the LookPath filesystem probes never run while
// holding the shared forge/config mutation lock.
func (s *Server) mcpServersPartial() string {
	s.hub.forgeMu.Lock()
	servers := make([]ctxforge.MCPServer, len(s.forge.MCPServers))
	copy(servers, s.forge.MCPServers)
	s.hub.forgeMu.Unlock()

	rows := make([]map[string]any, 0, len(servers))
	for _, m := range servers {
		cmd := m.Command
		if len(m.Args) > 0 {
			cmd += " " + strings.Join(m.Args, " ")
		}
		rows = append(rows, map[string]any{
			"ID": m.ID, "On": m.Enabled, "Name": m.Name,
			"Desc": truncate(cmd, 80), "Available": s.commandAvailable(m.Command),
		})
	}
	return frag("mcpPage", map[string]any{"Add": addData("mcp", "MCP server"), "Rows": rows})
}

// commandAvailable reports whether an MCP server's command resolves on PATH. It
// is the page preflight: a blank command (already rejected by Validate) and a
// nil seam both read as available so the page never blocks on the impurity.
func (s *Server) commandAvailable(command string) bool {
	command = strings.TrimSpace(command)
	if command == "" || s.lookPath == nil {
		return true
	}
	_, err := s.lookPath(command)
	return err == nil
}

// --- MCP server form ---

func renderMCPServerForm(m ctxforge.MCPServer, isNew bool, errMsg string) string {
	title, action := "Edit MCP server", "/mcp/"+m.ID
	if isNew {
		title, action = "New MCP server", "/mcp"
	}
	return formShell(title, action, "mcp", errMsg,
		idField(m.ID, isNew),
		textField("Name", "name", m.Name, false),
		textField("Command", "command", m.Command, true),
		textField("Args (comma-separated)", "args", strings.Join(m.Args, ", "), false),
		checkboxField("Enabled", "enabled", m.Enabled),
	)
}

func (s *Server) handleMCPServerNew(w http.ResponseWriter, r *http.Request) {
	s.writePartial(w, renderMCPServerForm(ctxforge.MCPServer{}, true, ""))
}

func (s *Server) handleMCPServerEdit(w http.ResponseWriter, r *http.Request) {
	s.hub.forgeMu.Lock()
	m := s.forge.MCPServer(r.PathValue("id"))
	var form string
	if m != nil {
		form = renderMCPServerForm(*m, false, "")
	}
	s.hub.forgeMu.Unlock()
	if form == "" {
		s.writePartial(w, s.mcpServersPartial())
		return
	}
	s.writePartial(w, form)
}

func mcpServerFromForm(r *http.Request, id string) ctxforge.MCPServer {
	return ctxforge.MCPServer{
		ID:      id,
		Name:    strings.TrimSpace(r.FormValue("name")),
		Command: strings.TrimSpace(r.FormValue("command")),
		Args:    parseCSV(r.FormValue("args")),
		Enabled: r.FormValue("enabled") != "",
	}
}

func (s *Server) handleMCPServerCreate(w http.ResponseWriter, r *http.Request) {
	m := mcpServerFromForm(r, strings.TrimSpace(r.FormValue("id")))
	if err := s.editForge(func() error { return s.forge.AddMCPServer(m) }); err != nil {
		s.writePartial(w, renderMCPServerForm(m, true, err.Error()))
		return
	}
	s.writePartial(w, s.mcpServersPartial())
}

func (s *Server) handleMCPServerUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	m := mcpServerFromForm(r, id)
	// Env is not edited through the form (no secrets UI — ADR-0010); preserve any
	// existing value so an edit doesn't silently wipe a manually-configured key.
	s.hub.forgeMu.Lock()
	if cur := s.forge.MCPServer(id); cur != nil {
		m.Env = cur.Env
	}
	s.hub.forgeMu.Unlock()
	if err := s.editForge(func() error { return s.forge.UpdateMCPServer(id, m) }); err != nil {
		s.writePartial(w, renderMCPServerForm(m, false, err.Error()))
		return
	}
	s.writePartial(w, s.mcpServersPartial())
}

func (s *Server) handleMCPServerToggle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_ = s.editForge(func() error { _, err := s.forge.ToggleMCPServer(id); return err })
	s.writePartial(w, s.mcpServersPartial())
}

func (s *Server) handleMCPServerDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.editForge(func() error { return s.forge.RemoveMCPServer(r.PathValue("id")) }); err != nil {
		s.logger.Printf("remove mcp server: %v", err)
	}
	s.writePartial(w, s.mcpServersPartial())
}
