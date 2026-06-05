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
	for _, sub := range []string{`<form`, `hx-post="/mcp"`, `name="id"`, `name="command"`, `name="args"`, `name="name"`} {
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

func TestMCPUpdatePreservesEnv(t *testing.T) {
	s, _ := newTestServer()
	s.forge.MCPServers = []ctxforge.MCPServer{{
		ID: "fetch", Name: "fetch", Command: "uvx", Env: map[string]string{"TOKEN": "secret"}, Enabled: true,
	}}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	// The form has no env field; an edit must not wipe a manually-set Env.
	resp, err := http.PostForm(srv.URL+"/mcp/fetch", url.Values{
		"id": {"fetch"}, "name": {"Fetch v2"}, "command": {"uvx"},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	got := s.forge.MCPServer("fetch")
	if got.Name != "Fetch v2" {
		t.Fatalf("update did not apply: %+v", got)
	}
	if got.Env["TOKEN"] != "secret" {
		t.Errorf("update should preserve Env, got %+v", got.Env)
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
	if MCPServerSpecs(nil) != nil {
		t.Error("empty input should convert to nil")
	}
	in := []ctxforge.MCPServer{
		{ID: "git", Name: "Git", Command: "uvx", Args: []string{"mcp-server-git"}, Env: map[string]string{"K": "v"}, Enabled: true},
	}
	got := MCPServerSpecs(in)
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
	s.applyAgentSpec(c)

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
