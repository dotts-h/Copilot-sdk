package web

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"strings"
	"testing"

	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
)

func TestMCPNewForm(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := get(t, srv, "/mcp/new")
	for _, sub := range []string{
		`<form`, `hx-post="/mcp"`, `name="id"`, `name="command"`, `name="args"`, `name="name"`,
		// The Env editor renders repeatable masked key/value/secret rows (ADR-0020).
		`name="env.key.0"`, `name="env.val.0"`, `name="env.secret.0"`, `env-editor`,
	} {
		if !strings.Contains(body, sub) {
			t.Errorf("new MCP form missing %q: %s", sub, body)
		}
	}
}

func TestMCPCreate(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/mcp", url.Values{
		"id": {"git"}, "name": {"git"}, "command": {"uvx"}, "args": {"mcp-server-git, --repo, ."},
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if len(s.forge.MCPServers) != 1 || s.forge.MCPServers[0].ID != "git" {
		t.Fatalf("server not created: %+v", s.forge.MCPServers)
	}
	if got := s.forge.MCPServers[0].Args; len(got) != 3 || got[0] != "mcp-server-git" {
		t.Errorf("args not parsed: %+v", got)
	}
	// Created disabled by default (no enabled field posted).
	if s.forge.MCPServers[0].Enabled {
		t.Errorf("server should default to disabled")
	}
	if !strings.Contains(string(body), "git") || !strings.Contains(string(body), "MCP") {
		t.Errorf("create did not return MCP list: %s", body)
	}
}

func TestMCPCreateInvalidReshowsForm(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	// Missing command → validation error.
	resp, err := http.PostForm(srv.URL+"/mcp", url.Values{"id": {"bad"}, "name": {"Bad"}})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if len(s.forge.MCPServers) != 0 {
		t.Fatalf("invalid server should not be added: %+v", s.forge.MCPServers)
	}
	if !strings.Contains(string(body), `<form`) || !strings.Contains(string(body), "error") {
		t.Errorf("invalid create should reshow form with error: %s", body)
	}
}

func TestMCPEditFormPrefilled(t *testing.T) {
	s, _ := newTestServer()
	s.forge.MCPServers = []ctxforge.MCPServer{{ID: "fs", Name: "filesystem", Command: "npx", Args: []string{"-y", "srv"}}}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := get(t, srv, "/mcp/fs/edit")
	if !strings.Contains(body, `hx-post="/mcp/fs"`) {
		t.Errorf("edit form should post to /mcp/fs: %s", body)
	}
	if !strings.Contains(body, `value="filesystem"`) || !strings.Contains(body, `value="npx"`) {
		t.Errorf("edit form not prefilled: %s", body)
	}
}

// TestMCPEditFormShowsEnvRows: editing a server with an existing Env preloads
// its rows — a literal as a plain row, a ${VAR} reference as a masked secret row
// showing the bare VAR_NAME (never a raw secret).
func TestMCPEditFormShowsEnvRows(t *testing.T) {
	s, _ := newTestServer()
	s.forge.MCPServers = []ctxforge.MCPServer{{
		ID: "gh", Name: "github", Command: "uvx",
		Env: map[string]string{"REGION": "eu", "GITHUB_TOKEN": "${GH_PAT}"}, Enabled: true,
	}}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := get(t, srv, "/mcp/gh/edit")
	// Literal row round-trips its value in a plain text input.
	if !strings.Contains(body, `value="REGION"`) || !strings.Contains(body, `value="eu"`) {
		t.Errorf("literal env row not prefilled: %s", body)
	}
	// Secret row shows the VAR_NAME masked, with the secret box checked — and the
	// reference's ${…} wrapper never leaks into the value attribute.
	if !strings.Contains(body, `value="GITHUB_TOKEN"`) || !strings.Contains(body, `value="GH_PAT"`) {
		t.Errorf("secret env row not prefilled with VAR_NAME: %s", body)
	}
	if strings.Contains(body, `value="${GH_PAT}"`) {
		t.Errorf("secret value input must show the bare VAR_NAME, not the ${...} reference: %s", body)
	}
	if !strings.Contains(body, `type="password"`) {
		t.Errorf("secret value input should be masked: %s", body)
	}
}

// TestMCPUpdateRoundTripsEnvViaEditor: the masked editor is authoritative on
// save. A literal row persists verbatim; a secret row persists ONLY the
// ${VAR_NAME} reference (never the raw value field) — the core ADR-0020 guard.
func TestMCPUpdateRoundTripsEnvViaEditor(t *testing.T) {
	s, _ := newTestServer()
	s.forge.MCPServers = []ctxforge.MCPServer{{
		ID: "fetch", Name: "fetch", Command: "uvx", Enabled: true,
	}}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/mcp/fetch", url.Values{
		"id": {"fetch"}, "name": {"Fetch v2"}, "command": {"uvx"},
		"env.key.0": {"REGION"}, "env.val.0": {"eu"}, // literal
		"env.key.1": {"GITHUB_TOKEN"}, "env.val.1": {"GH_PAT"}, "env.secret.1": {"1"}, // secret
	})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	got := s.forge.MCPServer("fetch")
	if got.Name != "Fetch v2" {
		t.Fatalf("update did not apply: %+v", got)
	}
	if got.Env["REGION"] != "eu" {
		t.Errorf("literal env value should round-trip verbatim, got %q", got.Env["REGION"])
	}
	// Only the reference is on disk — the var name wrapped, and crucially nothing
	// that looks like the raw secret the value field might have carried.
	if got.Env["GITHUB_TOKEN"] != "${GH_PAT}" {
		t.Errorf("secret row should persist only the ${VAR} reference, got %q", got.Env["GITHUB_TOKEN"])
	}
}

// TestMCPCreateRejectsMalformedSecretRef: a secret row whose value isn't a valid
// UPPER_SNAKE environment-variable name is rejected (re-render with error), so a
// reference that could never expand is never written — which would otherwise be
// sent to the runtime as an unexpanded literal.
func TestMCPCreateRejectsMalformedSecretRef(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/mcp", url.Values{
		"id": {"bad"}, "name": {"Bad"}, "command": {"uvx"},
		"env.key.0": {"TOKEN"}, "env.val.0": {"not a var name"}, "env.secret.0": {"1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if len(s.forge.MCPServers) != 0 {
		t.Fatalf("server with a malformed secret reference must not be added: %+v", s.forge.MCPServers)
	}
	if !strings.Contains(string(body), `<form`) || !strings.Contains(string(body), "error") {
		t.Errorf("malformed secret ref should reshow the form with an error: %s", body)
	}
	// The user's work is preserved on the error re-render (their typed rows).
	if !strings.Contains(string(body), `value="TOKEN"`) {
		t.Errorf("error re-render should preserve the typed env rows: %s", body)
	}
}

// TestMCPServerSpecsResolvesEnv exercises the forge→seam secret resolution
// (ADR-0020) behind an injected env lookup: a literal passes through, a resolved
// ${VAR} expands to its value, and an UNRESOLVED ${VAR} is left unset — never
// forwarded as the literal "${VAR}" string.
func TestMCPServerSpecsResolvesEnv(t *testing.T) {
	in := []ctxforge.MCPServer{{
		ID: "gh", Name: "github", Command: "uvx", Enabled: true,
		Env: map[string]string{
			"REGION":       "eu",        // literal
			"GITHUB_TOKEN": "${GH_PAT}", // resolves
			"EXTRA_KEY":    "${ABSENT}", // unresolved → unset
		},
	}}
	env := map[string]string{"GH_PAT": "ghp_xxx"} // ABSENT deliberately missing
	got := MCPServerSpecs(in, func(k string) string { return env[k] })
	if len(got) != 1 {
		t.Fatalf("expected one spec, got %d", len(got))
	}
	gotEnv := got[0].Env
	if gotEnv["REGION"] != "eu" {
		t.Errorf("literal should pass through, got %q", gotEnv["REGION"])
	}
	if gotEnv["GITHUB_TOKEN"] != "ghp_xxx" {
		t.Errorf("resolved reference should expand to the env value, got %q", gotEnv["GITHUB_TOKEN"])
	}
	if v, ok := gotEnv["EXTRA_KEY"]; ok {
		t.Errorf("unresolved reference must be left unset, not sent as %q", v)
	}
}

// TestMCPPreflightFlagsUnresolvedEnv: the page preflight flags an ENABLED
// server whose ${VAR} resolves empty (behind the injected env seam) with a
// "missing key" badge, and does not flag a disabled one or a resolved reference.
func TestMCPPreflightFlagsUnresolvedEnv(t *testing.T) {
	s, _ := newTestServer()
	s.forge.MCPServers = []ctxforge.MCPServer{
		{ID: "needs", Name: "needs", Command: "present-cmd", Enabled: true, Env: map[string]string{"GITHUB_TOKEN": "${MISSING_PAT}"}},
		{ID: "ok", Name: "ok", Command: "present-cmd", Enabled: true, Env: map[string]string{"GITHUB_TOKEN": "${HAVE_PAT}"}},
		{ID: "off", Name: "off", Command: "present-cmd", Enabled: false, Env: map[string]string{"GITHUB_TOKEN": "${MISSING_PAT}"}},
	}
	s.lookPath = func(string) (string, error) { return "/usr/bin/present-cmd", nil }
	s.lookupEnv = func(k string) string {
		if k == "HAVE_PAT" {
			return "ghp_present"
		}
		return ""
	}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := get(t, srv, "/page/mcp")
	// Exactly the enabled-with-unresolved-ref row is flagged missing-key.
	if n := strings.Count(body, "missing-key"); n != 1 {
		t.Errorf("expected exactly one missing-key row, got %d: %s", n, body)
	}
	if !strings.Contains(body, ">missing key</span>") {
		t.Errorf("expected a missing-key badge for the unresolved reference: %s", body)
	}
}

func TestMCPToggleViaHandler(t *testing.T) {
	s, _ := newTestServer()
	s.forge.MCPServers = []ctxforge.MCPServer{{ID: "fs", Name: "fs", Command: "npx", Enabled: false}}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	_, err := http.Post(srv.URL+"/mcp/fs/toggle", "application/x-www-form-urlencoded", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !s.forge.MCPServer("fs").Enabled {
		t.Error("toggle should have enabled the server")
	}
}

func TestMCPDeleteViaHandler(t *testing.T) {
	s, _ := newTestServer()
	s.forge.MCPServers = []ctxforge.MCPServer{{ID: "fs", Name: "fs", Command: "npx"}}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	_, err := http.PostForm(srv.URL+"/mcp/fs/delete", url.Values{})
	if err != nil {
		t.Fatal(err)
	}
	if s.forge.MCPServer("fs") != nil {
		t.Error("server should be deleted")
	}
}

// TestMCPPagePreflightMarksUnavailable exercises the exec.LookPath preflight seam:
// a server whose command does not resolve on PATH is marked unavailable, while a
// resolvable one is not. The seam (s.lookPath) keeps the renderer unit-testable
// without depending on what's actually installed on the host.
func TestMCPPagePreflightMarksUnavailable(t *testing.T) {
	s, _ := newTestServer()
	s.forge.MCPServers = []ctxforge.MCPServer{
		{ID: "here", Name: "here", Command: "present-cmd"},
		{ID: "gone", Name: "gone", Command: "absent-cmd"},
	}
	s.lookPath = func(cmd string) (string, error) {
		if cmd == "present-cmd" {
			return "/usr/bin/present-cmd", nil
		}
		return "", errors.New("not found")
	}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := get(t, srv, "/page/mcp")
	// Exactly the missing-command row is flagged: the <li> gains the "unavailable"
	// class, and the present-command row does not. (The page intro also mentions
	// the word, so assert on the row-level class, not a raw word count.)
	if n := strings.Count(body, `row unavailable`); n != 1 {
		t.Errorf("expected exactly one unavailable row, got %d: %s", n, body)
	}
	if !strings.Contains(body, `class="badge warn"`) {
		t.Errorf("expected an unavailable badge for the missing command: %s", body)
	}
}

func TestMCPServerSpecsConverts(t *testing.T) {
	if MCPServerSpecs(nil, nil) != nil {
		t.Error("empty input should convert to nil")
	}
	in := []ctxforge.MCPServer{
		{ID: "git", Name: "Git", Command: "uvx", Args: []string{"mcp-server-git"}, Env: map[string]string{"K": "v"}, Enabled: true},
	}
	got := MCPServerSpecs(in, func(string) string { return "" })
	if len(got) != 1 || got[0].ID != "git" || got[0].Name != "Git" || got[0].Command != "uvx" {
		t.Fatalf("conversion dropped fields: %+v", got)
	}
	if got[0].Key() != "git" {
		t.Errorf("Key() should be the id, got %q", got[0].Key())
	}
	if got[0].Env["K"] != "v" || len(got[0].Args) != 1 {
		t.Errorf("args/env not carried: %+v", got[0])
	}
}

// TestEnabledMCPServerReachesSessionSpec is the end-to-end wiring guard: an
// enabled forge MCP server must flow through Compile → compiledSpec →
// applyAgentSpec into the live copilot spec (so the runtime actually starts it).
// A disabled server must not.
func TestEnabledMCPServerReachesSessionSpec(t *testing.T) {
	s, _ := newTestServer()
	s.forge.MCPServers = []ctxforge.MCPServer{
		{ID: "git", Name: "Git", Command: "uvx", Args: []string{"mcp-server-git"}, Enabled: true},
		{ID: "off", Name: "Off", Command: "npx", Enabled: false},
	}
	s.hub.forgeMu.Lock()
	c := s.compiledSpec("")
	s.hub.forgeMu.Unlock()
	s.applyAgentSpec(c, "")

	s.mu.Lock()
	got := s.spec.MCPServers
	s.mu.Unlock()
	if len(got) != 1 {
		t.Fatalf("only the enabled server should reach the spec, got %d: %+v", len(got), got)
	}
	if got[0].ID != "git" || got[0].Key() != "git" {
		t.Errorf("wrong server wired: %+v", got[0])
	}
}

func TestNewServerDefaultsLookPath(t *testing.T) {
	s, _ := newTestServer()
	if s.lookPath == nil {
		t.Fatal("server should default lookPath to exec.LookPath")
	}
	// Sanity: the default is exec.LookPath's behaviour (a definitely-absent binary errors).
	if _, err := s.lookPath("definitely-not-a-real-binary-xyz"); err == nil {
		t.Error("expected lookup of an absent binary to error")
	}
	_ = exec.ErrNotFound
}
