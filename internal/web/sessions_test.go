// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

func TestSessionsPageListsSessions(t *testing.T) {
	s, mock := newTestServer()
	mock.Sessions = []copilot.SessionMeta{
		{ID: "sess-1", Summary: "Refactor auth", Modified: time.Now()},
		{ID: "sess-2", Summary: "", Modified: time.Now().Add(-time.Hour)},
	}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := get(t, srv, "/page/sessions")
	for _, sub := range []string{
		"Refactor auth",
		`hx-post="/sessions/sess-1/resume"`,
		`hx-post="/sessions/sess-2/resume"`,
		`hx-post="/sessions/sess-1/delete"`,
		`hx-post="/sessions/new"`,
	} {
		if !strings.Contains(body, sub) {
			t.Errorf("sessions page missing %q\n%s", sub, body)
		}
	}
}

func TestSessionsPageShowsPerSessionCost(t *testing.T) {
	s, mock := newTestServer()
	mock.Sessions = []copilot.SessionMeta{
		{ID: "sess-1", Summary: "Refactor auth", Modified: time.Now()},
		{ID: "sess-2", Summary: "Read the docs", Modified: time.Now().Add(-time.Hour)},
	}
	// sess-1 has two metered turns (0.05 + 0.05 = $0.10 → 10.00 cr); sess-2 has
	// none (shows the no-cost state); sess-3 is a since-deleted session whose spend
	// has no row to join against (must not appear).
	seedLedger(t, s,
		telemetry.SpendRecord{SessionID: "sess-1", Model: "gpt-5", USD: 0.05},
		telemetry.SpendRecord{SessionID: "sess-1", Model: "gpt-5", USD: 0.05},
		telemetry.SpendRecord{SessionID: "sess-3", Model: "gpt-5", USD: 0.99},
	)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := get(t, srv, "/page/sessions")
	for _, sub := range []string{
		"2 turns",     // sess-1 turn count
		"10.00 cr",    // sess-1 credits (FormatCredits)
		"no cost yet", // sess-2 has no spend but is still listed
	} {
		if !strings.Contains(body, sub) {
			t.Errorf("sessions page missing %q\n%s", sub, body)
		}
	}
	// The since-deleted session's spend bucket joins no listed row, so its credits
	// ($0.99 → 99.00 cr) never render.
	if strings.Contains(body, "99.00 cr") {
		t.Errorf("a spend bucket with no matching session must not be shown\n%s", body)
	}
}

func TestSessionsPageNoSpendStoreNoPanic(t *testing.T) {
	// With no spend store wired (s.spend == nil) the page renders its prior shape:
	// no cost cells, no panic.
	s, mock := newTestServer()
	mock.Sessions = []copilot.SessionMeta{{ID: "sess-1", Summary: "x", Modified: time.Now()}}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := get(t, srv, "/page/sessions")
	if !strings.Contains(body, `hx-post="/sessions/sess-1/resume"`) {
		t.Errorf("sessions page should still render rows without a spend store\n%s", body)
	}
	if strings.Contains(body, "no cost yet") || strings.Contains(body, " cr") {
		t.Errorf("no spend store should mean no cost cells\n%s", body)
	}
}

func TestSessionResumeRebuildsTranscript(t *testing.T) {
	s, mock := newTestServer()
	mock.Histories = map[string][]copilot.Event{
		"sess-1": {
			{Type: copilot.EvUserMessage, Text: "fix the bug"},
			{Type: copilot.EvReasoning, Text: "considering the options"},
			{Type: copilot.EvMessage, Text: "done — patched it"},
			{Type: copilot.EvToolStart, ToolCall: &copilot.ToolCall{ID: "t1", Name: "bash", Args: "go test"}},
			{Type: copilot.EvToolEnd, ToolCall: &copilot.ToolCall{ID: "t1", Result: "ok", Success: true}},
		},
	}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/sessions/sess-1/resume", url.Values{})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body := readBody(t, resp)

	if len(mock.Resumed) != 1 || mock.Resumed[0] != "sess-1" {
		t.Errorf("expected ResumeSession(sess-1), got %v", mock.Resumed)
	}
	if s.sessionID != "sess-1" {
		t.Errorf("sessionID = %q, want sess-1", s.sessionID)
	}
	// The resumed session must be bound so its live events route back here
	// (otherwise the streamed reply to the next turn would go nowhere).
	if s.hub.route("sess-1") != s {
		t.Errorf("resumed session not bound for event routing")
	}
	for _, sub := range []string{"fix the bug", "done — patched it", "bash"} {
		if !strings.Contains(body, sub) {
			t.Errorf("resumed transcript missing %q\n%s", sub, body)
		}
	}
}

func TestSessionNewClearsConversation(t *testing.T) {
	s, _ := newTestServer()
	s.sessionID = "old-session"
	s.state.AddUser("a previous prompt")
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/sessions/new", url.Values{})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body := readBody(t, resp)

	if s.sessionID != "" {
		t.Errorf("new chat should clear sessionID, got %q", s.sessionID)
	}
	if strings.Contains(body, "a previous prompt") {
		t.Errorf("new chat should not show the old transcript:\n%s", body)
	}
}

func TestSessionDelete(t *testing.T) {
	s, mock := newTestServer()
	mock.Sessions = []copilot.SessionMeta{{ID: "sess-1", Summary: "x", Modified: time.Now()}}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/sessions/sess-1/delete", url.Values{})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if len(mock.Deleted) != 1 || mock.Deleted[0] != "sess-1" {
		t.Errorf("expected DeleteSession(sess-1), got %v", mock.Deleted)
	}
}
