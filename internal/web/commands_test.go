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

	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
)

func TestSeamSpecThreadsWorkspaceAndAutoApprove(t *testing.T) {
	cs := ctxforge.SessionSpec{Hooks: ctxforge.DefaultHooks()}
	env := func(string) string { return "" }

	on := SeamSpec(cs, "gpt-5", "high", env, "/home/u/project", true)
	if !on.AutoApproveTools {
		t.Fatal("SeamSpec dropped AutoApproveTools=true; the settings toggle never reaches the seam")
	}
	if on.Workspace != "/home/u/project" {
		t.Fatalf("Workspace = %q, want the threaded root", on.Workspace)
	}

	off := SeamSpec(cs, "gpt-5", "high", env, "", false)
	if off.AutoApproveTools {
		t.Fatal("SeamSpec set AutoApproveTools when the toggle was off")
	}
}

func TestParseCommand(t *testing.T) {
	for _, tc := range []struct {
		in              string
		wantName, wantA string
		wantOK          bool
	}{
		{"/model gpt-5", "model", "gpt-5", true},
		{"/clear", "clear", "", true},
		{"/AGENT  builder ", "agent", "builder", true},
		{"hello", "", "", false},
		{"/", "", "", true},
	} {
		name, args, ok := parseCommand(tc.in)
		if name != tc.wantName || args != tc.wantA || ok != tc.wantOK {
			t.Errorf("parseCommand(%q) = (%q,%q,%v), want (%q,%q,%v)",
				tc.in, name, args, ok, tc.wantName, tc.wantA, tc.wantOK)
		}
	}
}

// postSend posts a composer input and returns the response body.
func postSend(t *testing.T, srv *httptest.Server, prompt string) string {
	t.Helper()
	resp, err := http.PostForm(srv.URL+"/send", url.Values{"prompt": {prompt}})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return string(body)
}

func TestModelCommandSwitchesAndRestarts(t *testing.T) {
	s, mock := newTestServer()
	s.sessionID = "sess-1"
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := postSend(t, srv, "/model claude-sonnet-4.6")
	if !strings.Contains(body, "model → claude-sonnet-4.6") || !strings.Contains(body, `hx-swap-oob`) {
		t.Errorf("model command body = %q", body)
	}
	if s.spec.Model != "claude-sonnet-4.6" {
		t.Errorf("spec model not updated: %q", s.spec.Model)
	}
	if s.sessionID != "" {
		t.Errorf("session not restarted: %q", s.sessionID)
	}
	if s.config.DefaultModel != "claude-sonnet-4.6" {
		t.Errorf("default model not persisted: %q", s.config.DefaultModel)
	}
	// Command must not be forwarded to the model.
	if len(mock.Sent) != 0 {
		t.Errorf("command leaked to model: %v", mock.Sent)
	}
}

func TestModelCommandNoArgReportsCurrent(t *testing.T) {
	s, _ := newTestServer()
	s.spec.Model = "gpt-5"
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := postSend(t, srv, "/model")
	if !strings.Contains(body, "model: gpt-5") {
		t.Errorf("no-arg /model body = %q", body)
	}
}

func TestAgentCommandAppliesSpec(t *testing.T) {
	s, _ := newTestServer()
	s.forge.Agents = []ctxforge.Agent{{ID: "builder", Name: "Builder", Model: "gpt-5-codex", ReasoningEffort: "high"}}
	s.sessionID = "sess-1"
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := postSend(t, srv, "/agent builder")
	if !strings.Contains(body, "agent → Builder") {
		t.Errorf("agent command body = %q", body)
	}
	if s.config.DefaultAgent != "builder" || s.spec.Model != "gpt-5-codex" || s.spec.ReasoningEffort != "high" {
		t.Errorf("agent spec not applied: agent=%q model=%q effort=%q",
			s.config.DefaultAgent, s.spec.Model, s.spec.ReasoningEffort)
	}
	if s.sessionID != "" {
		t.Errorf("session not restarted")
	}

	// /agent none clears it and resets to the default model.
	s.config.DefaultModel = "gpt-5"
	_ = postSend(t, srv, "/agent none")
	if s.config.DefaultAgent != "" || s.spec.Model != "gpt-5" {
		t.Errorf("agent not cleared: agent=%q model=%q", s.config.DefaultAgent, s.spec.Model)
	}
}

func TestAgentCommandUnknown(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	body := postSend(t, srv, "/agent ghost")
	if !strings.Contains(body, "unknown agent: ghost") {
		t.Errorf("unknown agent body = %q", body)
	}
}

func TestClearCommandResetsState(t *testing.T) {
	s, _ := newTestServer()
	s.state.AddUser("earlier message")
	s.sessionID = "sess-1"
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := postSend(t, srv, "/clear")
	if strings.Contains(body, "earlier message") {
		t.Errorf("cleared timeline still has old turn: %q", body)
	}
	if !strings.Contains(body, "conversation cleared") {
		t.Errorf("clear note missing: %q", body)
	}
	if s.sessionID != "" {
		t.Errorf("session not restarted on clear")
	}
}

func TestNavCommandSwapsMain(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := postSend(t, srv, "/telemetry")
	if !strings.Contains(body, `id="main"`) || !strings.Contains(body, "Token Telemetry") {
		t.Errorf("nav command did not OOB-swap main: %q", body)
	}
}

func TestCostCommand(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := postSend(t, srv, "/cost")
	if !strings.Contains(body, "cost:") || !strings.Contains(body, `id="cost-footer"`) {
		t.Errorf("cost command body = %q", body)
	}
}

func TestAttachQueuesForNextSend(t *testing.T) {
	s, mock := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	_ = postSend(t, srv, "/attach /tmp/diagram.png")
	if len(s.pending) != 1 || s.pending[0] != "/tmp/diagram.png" {
		t.Fatalf("attachment not queued: %v", s.pending)
	}
	_ = postSend(t, srv, "describe this")
	if len(mock.LastAttach) != 1 || mock.LastAttach[0] != "/tmp/diagram.png" {
		t.Errorf("attachment not forwarded with next send: %v", mock.LastAttach)
	}
	if len(s.pending) != 0 {
		t.Errorf("pending not drained: %v", s.pending)
	}
}

func TestHelpCommand(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	body := postSend(t, srv, "/help")
	if !strings.Contains(body, "/model") || !strings.Contains(body, "/clear") {
		t.Errorf("help body = %q", body)
	}
}

func TestHelpPageServed(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/page/help")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	for _, sub := range []string{"Composer commands", "/model", "Panels", "Telemetry"} {
		if !strings.Contains(string(body), sub) {
			t.Errorf("help page missing %q", sub)
		}
	}
}

// TestNavCommandReachesHelp checks the /help-adjacent nav slug routes too.
func TestNavCommandReachesHelp(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	body := postSend(t, srv, "/help")
	// /help is a command (timeline note), not a nav swap.
	if !strings.Contains(body, "id=\"timeline\"") {
		t.Errorf("/help should be a timeline note: %q", body)
	}
}

func TestUnknownCommand(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	body := postSend(t, srv, "/frobnicate")
	if !strings.Contains(body, "unknown command: /frobnicate") {
		t.Errorf("unknown command body = %q", body)
	}
}
