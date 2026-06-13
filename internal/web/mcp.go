// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package web

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
)

// MCPServerSpecs converts the forge's compiled (enabled) MCP servers into the
// seam type the session spec carries, preserving the unique id so the runtime
// keys them safely. It is the single forge→seam translation for MCP, shared by
// the session-startup path (bootstrap) and the agent-restart path (compiledSpec).
//
// This boundary is also where Env secrets are resolved (ADR-0020): an Env value
// of the reference shape ${VAR_NAME} is expanded via lookupEnv (default
// os.Getenv); a reference that resolves empty is left UNSET on the spec — never
// forwarded as the literal "${VAR_NAME}". Literals pass through unchanged. The
// reference shape is given meaning only here, so ctxforge stores it as opaque
// data and stays dependency-free.
func MCPServerSpecs(servers []ctxforge.MCPServer, lookupEnv func(string) string) []copilot.MCPServer {
	if len(servers) == 0 {
		return nil
	}
	out := make([]copilot.MCPServer, 0, len(servers))
	for _, m := range servers {
		out = append(out, copilot.MCPServer{
			ID: m.ID, Name: m.Name, Command: m.Command, Args: m.Args,
			Env: resolveEnv(m.Env, lookupEnv),
		})
	}
	return out
}

// envRefPattern matches the env-var-reference value shape ${VAR_NAME}, with a
// VAR_NAME of [A-Z_][A-Z0-9_]* (ADR-0020). Anything else is a literal value.
var envRefPattern = regexp.MustCompile(`^\$\{([A-Z_][A-Z0-9_]*)\}$`)

// envRef reports whether v is an env-var reference of the form ${VAR_NAME} and,
// if so, returns the bare VAR_NAME. A literal value returns ("", false). This is
// the one place the reference shape is decoded for resolution and preflight.
func envRef(v string) (string, bool) {
	m := envRefPattern.FindStringSubmatch(v)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// envVarPattern constrains the VAR_NAME a secret row may reference (and so what
// the form will wrap as ${VAR_NAME}): an UPPER_SNAKE environment-variable name.
var envVarPattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

// resolveEnv expands ${VAR} references in an MCP server's Env using lookupEnv
// (nil falls back to os.Getenv). A literal value passes through unchanged; a
// reference resolves to its env value; a reference that resolves empty is
// OMITTED entirely (the key is unset), so the server never receives the literal
// "${VAR}" string. Returns nil when nothing remains.
func resolveEnv(env map[string]string, lookupEnv func(string) string) map[string]string {
	if len(env) == 0 {
		return nil
	}
	if lookupEnv == nil {
		lookupEnv = os.Getenv
	}
	out := make(map[string]string, len(env))
	for k, v := range env {
		if name, ok := envRef(v); ok {
			if got := lookupEnv(name); got != "" {
				out[k] = got
			}
			continue // unresolved reference → leave the key unset
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// This file is the MCP-server management page. It mirrors the forge-CRUD pattern
// (validated builders, rollback-on-invalid save, re-render the form in place on
// error) used for skills/instructions/agents, and adds a PATH preflight
// (exec.LookPath, behind the s.lookPath seam) so a curated stdio server whose
// binary is absent is flagged in the UI rather than surprise-failing when the
// session starts. The form now also edits Env via masked key/value rows: a
// secret row persists only a ${VAR_NAME} reference (never the secret), and the
// preflight additionally flags an enabled server whose ${VAR} resolves empty
// (ADR-0020), behind the s.lookupEnv seam.

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
		missing := s.missingEnvRefs(m)
		rows = append(rows, map[string]any{
			"ID": m.ID, "On": m.Enabled, "Name": m.Name,
			"Desc": truncate(cmd, 80), "Available": s.commandAvailable(m.Command),
			"EnvWarn": len(missing) > 0, "MissingEnvList": strings.Join(missing, ", "),
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

// missingEnvRefs returns the VAR_NAMEs an enabled server references that resolve
// empty in the environment (behind the s.lookupEnv seam) — the secret-preflight
// companion to the PATH probe. A disabled server is never flagged (it can't
// start a session); literals are ignored. Resolution uses the same seam as
// MCPServerSpecs, so what the page warns about is exactly what would be unset.
func (s *Server) missingEnvRefs(m ctxforge.MCPServer) []string {
	if !m.Enabled || len(m.Env) == 0 {
		return nil
	}
	lookup := s.lookupEnv
	if lookup == nil {
		lookup = os.Getenv
	}
	var missing []string
	for _, k := range sortedEnvKeys(m.Env) {
		if name, ok := envRef(m.Env[k]); ok && lookup(name) == "" {
			missing = append(missing, name)
		}
	}
	return missing
}

// sortedEnvKeys returns an Env map's keys in deterministic order (for stable
// rendering and preflight output).
func sortedEnvKeys(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// --- MCP server form ---

// envRow is one Env key/value editor row. For a secret row Value holds the bare
// VAR_NAME (rendered masked); on persist it is wrapped as ${VAR_NAME}. For a
// literal row Value holds the literal. The raw secret is never represented — the
// user names an environment variable, never types the secret (ADR-0020).
type envRow struct {
	Key    string
	Value  string
	Secret bool
}

// mcpEnvBlankRows is how many empty rows the editor appends so a user can add new
// vars without client-side row scripting; saving renders fresh blanks.
const mcpEnvBlankRows = 2

func renderMCPServerForm(m ctxforge.MCPServer, isNew bool, errMsg string, envRows []envRow) string {
	title, action := "Edit MCP server", "/mcp/"+m.ID
	if isNew {
		title, action = "New MCP server", "/mcp"
	}
	return formShell(title, action, "mcp", errMsg,
		idField(m.ID, isNew),
		textField("Name", "name", m.Name, false),
		textField("Command", "command", m.Command, true),
		textField("Args (comma-separated)", "args", strings.Join(m.Args, ", "), false),
		renderMCPEnvEditor(envRows),
		checkboxField("Enabled", "enabled", m.Enabled),
	)
}

// renderMCPEnvEditor renders the Env key/value rows plus trailing blanks. The
// per-row value input is masked (type=password) for a secret row; html/template
// auto-escapes every value as elsewhere (ADR-0001).
func renderMCPEnvEditor(rows []envRow) string {
	full := make([]envRow, 0, len(rows)+mcpEnvBlankRows)
	full = append(full, rows...)
	for i := 0; i < mcpEnvBlankRows; i++ {
		full = append(full, envRow{})
	}
	return frag("mcpEnvEditor", map[string]any{"Rows": full})
}

// envRowsFromEnv turns a persisted Env map into editor rows: a ${VAR} value
// becomes a secret row showing VAR_NAME; any other value is a literal row.
func envRowsFromEnv(env map[string]string) []envRow {
	rows := make([]envRow, 0, len(env))
	for _, k := range sortedEnvKeys(env) {
		if name, ok := envRef(env[k]); ok {
			rows = append(rows, envRow{Key: k, Value: name, Secret: true})
		} else {
			rows = append(rows, envRow{Key: k, Value: env[k], Secret: false})
		}
	}
	return rows
}

// envRowsFromForm reconstructs the editor rows from a posted form so a
// validation error re-renders exactly what the user typed (without losing work).
func envRowsFromForm(r *http.Request) []envRow {
	_ = r.ParseForm()
	var rows []envRow
	for i := 0; ; i++ {
		idx := strconv.Itoa(i)
		if _, ok := r.Form["env.key."+idx]; !ok {
			break
		}
		key := strings.TrimSpace(r.Form.Get("env.key." + idx))
		val := strings.TrimSpace(r.Form.Get("env.val." + idx))
		secret := r.Form.Get("env.secret."+idx) != ""
		if key == "" && val == "" {
			continue
		}
		rows = append(rows, envRow{Key: key, Value: val, Secret: secret})
	}
	return rows
}

// envFromForm builds the persisted Env map from the editor rows. A secret row
// persists ONLY the ${VAR_NAME} reference — never the value input — and its
// VAR_NAME must be a well-formed UPPER_SNAKE name so resolution never sends an
// unexpandable literal. A literal row persists its value verbatim. Returns nil
// when no rows carry a key.
func envFromForm(r *http.Request) (map[string]string, error) {
	env := map[string]string{}
	for _, row := range envRowsFromForm(r) {
		if row.Key == "" {
			continue
		}
		if row.Secret {
			if !envVarPattern.MatchString(row.Value) {
				return nil, fmt.Errorf("env %q: a secret value must name an environment variable (UPPER_SNAKE, e.g. GITHUB_TOKEN), got %q", row.Key, row.Value)
			}
			env[row.Key] = "${" + row.Value + "}"
			continue
		}
		env[row.Key] = row.Value
	}
	if len(env) == 0 {
		return nil, nil
	}
	return env, nil
}

func (s *Server) handleMCPServerNew(w http.ResponseWriter, r *http.Request) {
	s.writePartial(w, renderMCPServerForm(ctxforge.MCPServer{}, true, "", nil))
}

func (s *Server) handleMCPServerEdit(w http.ResponseWriter, r *http.Request) {
	s.hub.forgeMu.Lock()
	m := s.forge.MCPServer(r.PathValue("id"))
	var form string
	if m != nil {
		form = renderMCPServerForm(*m, false, "", envRowsFromEnv(m.Env))
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
	env, err := envFromForm(r)
	if err == nil {
		m.Env = env
		err = s.editForge(func() error { return s.forge.AddMCPServer(m) })
	}
	if err != nil {
		s.writePartial(w, renderMCPServerForm(m, true, err.Error(), envRowsFromForm(r)))
		return
	}
	s.writePartial(w, s.mcpServersPartial())
}

func (s *Server) handleMCPServerUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	m := mcpServerFromForm(r, id)
	// Env is now edited through the masked rows (ADR-0020): the submitted editor
	// is authoritative, including secret rows that persist only a ${VAR} reference.
	env, err := envFromForm(r)
	if err == nil {
		m.Env = env
		err = s.editForge(func() error { return s.forge.UpdateMCPServer(id, m) })
	}
	if err != nil {
		s.writePartial(w, renderMCPServerForm(m, false, err.Error(), envRowsFromForm(r)))
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
