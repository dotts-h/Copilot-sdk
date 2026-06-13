// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/dotts-h/copilot-sdk/internal/convo"
	"github.com/dotts-h/copilot-sdk/internal/copilot"
)

// The PostToolUse command field + executor surfacing (G5, ADR-0032).

// The hook form exposes the command + command-args fields and a command preflight.
func TestHookFormHasCommandFields(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := get(t, srv, "/hooks/new")
	for _, sub := range []string{
		`name="command"`, `name="commandArgs"`,
		// the command preflight posts to its own endpoint and never executes.
		`hx-post="/hooks/command-preflight"`,
	} {
		if !strings.Contains(body, sub) {
			t.Errorf("new hook form missing %q", sub)
		}
	}
}

// A post-tool-use hook with a command round-trips through the forge.
func TestHookCreatePersistsCommand(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/hooks", url.Values{
		"id": {"fmt-go"}, "event": {"post-tool-use"}, "toolKind": {"write"},
		"action": {"ask"}, "command": {"gofmt"}, "commandArgs": {"-w, ${TARGET_FILE}"},
		"enabled": {"1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	h := s.forge.Hook("fmt-go")
	if h == nil {
		t.Fatalf("hook not created: %+v", s.forge.Hooks)
	}
	if h.Command != "gofmt" || strings.Join(h.CommandArgs, "|") != "-w|${TARGET_FILE}" {
		t.Errorf("command not persisted: %+v", h)
	}
}

// A pre-tool-use hook carrying a command is rejected (the form re-renders with the
// error) — an untrusted command must never become a pre-gate control surface.
func TestHookCreatePreToolCommandRejected(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/hooks", url.Values{
		"id": {"bad-pre"}, "event": {"pre-tool-use"}, "toolKind": {"shell"},
		"action": {"ask"}, "command": {"echo"}, "enabled": {"1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if s.forge.Hook("bad-pre") != nil {
		t.Fatal("a pre-tool-use command hook must not be created")
	}
	if !strings.Contains(string(body), "error") {
		t.Errorf("expected the form to re-render with an error: %s", body)
	}
}

// The command preflight reports the resolved command line and warns about an
// unset ${VAR} — mirroring the MCP env preflight; it NEVER executes.
func TestHookCommandPreflight(t *testing.T) {
	s, _ := newTestServer()
	// SET_VAR resolves; MISSING_VAR is unset → flagged.
	s.lookupEnv = func(k string) string {
		if k == "SET_VAR" {
			return "yes"
		}
		return ""
	}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/hooks/command-preflight", url.Values{
		"command": {"gofmt"}, "commandArgs": {"-w, ${SET_VAR}, ${MISSING_VAR}"},
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	got := string(body)
	if !strings.Contains(got, "gofmt") {
		t.Errorf("preflight should echo the command: %s", got)
	}
	if !strings.Contains(got, "MISSING_VAR") {
		t.Errorf("preflight should flag the unset ${VAR}: %s", got)
	}
}

// An EvHookRun reduces into a compact timeline annotation carrying the hook id and
// its ESCAPED output snippet (ADR-0001) — display-only telemetry, never re-run.
func TestHookRunRendersInTimeline(t *testing.T) {
	s, _ := newTestServer()
	s.handleEvent(copilot.Event{Type: copilot.EvHookRun, HookRun: &copilot.HookRun{
		HookID: "fmt-go", Command: "gofmt -l main.go",
		Output: "<script>alert(1)</script>", ExitCode: 1, Failed: true,
	}})
	body := s.chatPartial()
	if !strings.Contains(body, "fmt-go") {
		t.Errorf("hook-run annotation missing the hook id: %s", body)
	}
	// The untrusted output must be HTML-escaped, never rendered as live markup.
	if strings.Contains(body, "<script>alert(1)</script>") {
		t.Errorf("untrusted hook output was not escaped: %s", body)
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Errorf("escaped output snippet missing: %s", body)
	}
}

// The render helper maps a hook-run to a note without a server.
func TestRenderHookRunNote(t *testing.T) {
	ok := renderHookRunNote(&convo.HookRunView{HookID: "x", Command: "echo hi", Output: "hi"})
	if !strings.Contains(ok, "echo hi") || !strings.Contains(ok, "x") {
		t.Errorf("hook-run note = %s", ok)
	}
	if renderHookRunNote(nil) != "" {
		t.Error("nil hook-run should render empty")
	}
}
