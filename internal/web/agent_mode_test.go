// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestPlanCommandTogglesMode(t *testing.T) {
	s, _ := newTestServer()
	if out := s.runCommand("/plan"); !strings.Contains(out, "plan mode on") {
		t.Fatalf("/plan should turn plan mode on: %s", out)
	}
	if s.mode != "plan" {
		t.Fatalf("mode should be plan after /plan, got %q", s.mode)
	}
	if out := s.runCommand("/plan"); !strings.Contains(out, "plan mode off") {
		t.Fatalf("second /plan should toggle off: %s", out)
	}
	if s.mode != "" {
		t.Fatalf("mode should be empty after toggling off, got %q", s.mode)
	}
	// Explicit on/off.
	s.runCommand("/plan off")
	if s.mode != "" {
		t.Fatal("/plan off should clear plan mode")
	}
	s.runCommand("/plan on")
	if s.mode != "plan" {
		t.Fatal("/plan on should set plan mode")
	}
}

func TestAutoAndAskModesAreExclusive(t *testing.T) {
	s, _ := newTestServer()
	if out := s.runCommand("/auto"); !strings.Contains(out, "autopilot on") {
		t.Fatalf("/auto should turn autopilot on: %s", out)
	}
	if s.mode != "autopilot" {
		t.Fatalf("mode = %q, want autopilot", s.mode)
	}
	// Switching to ask mode replaces autopilot (modes are mutually exclusive).
	if out := s.runCommand("/ask"); !strings.Contains(out, "ask mode on") {
		t.Fatalf("/ask should turn ask mode on: %s", out)
	}
	if s.mode != "interactive" {
		t.Fatalf("mode = %q, want interactive", s.mode)
	}
	s.runCommand("/ask off")
	if s.mode != "" {
		t.Fatalf("mode = %q, want empty after /ask off", s.mode)
	}
}

func TestEffortCommandSetsSpec(t *testing.T) {
	s, _ := newTestServer()
	if out := s.runCommand("/effort high"); !strings.Contains(out, "high") {
		t.Fatalf("/effort high: %s", out)
	}
	if s.spec.ReasoningEffort != "high" {
		t.Fatalf("ReasoningEffort = %q, want high", s.spec.ReasoningEffort)
	}
	if out := s.runCommand("/effort bogus"); !strings.Contains(out, "usage:") {
		t.Fatalf("/effort bogus should show usage: %s", out)
	}
}

func TestSendCarriesPlanMode(t *testing.T) {
	s, mock := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	s.runCommand("/plan on")
	resp, err := http.PostForm(srv.URL+"/send", url.Values{"prompt": {"build a feature"}})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if mock.SentCount() != 1 {
		t.Fatalf("prompt not sent: %d", mock.SentCount())
	}
	if got := mock.SentModeAt(0); got != "plan" {
		t.Fatalf("send agent mode = %q, want plan", got)
	}
}

func TestSendDefaultModeEmpty(t *testing.T) {
	s, mock := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	_, err := http.PostForm(srv.URL+"/send", url.Values{"prompt": {"hi"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := mock.SentModeAt(0); got != "" {
		t.Fatalf("default agent mode = %q, want empty", got)
	}
}

func TestPlanApprovalExitsPlanMode(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	s.runCommand("/plan on")
	s.handleEvent(samplePlan("plan-1"))
	_, err := http.PostForm(srv.URL+"/plan/plan-1", url.Values{"action": {"proceed"}})
	if err != nil {
		t.Fatal(err)
	}
	if s.mode != "" {
		t.Fatal("approving a plan should exit plan mode")
	}
}

func TestPlanInAutocomplete(t *testing.T) {
	if matches := matchCommands("/plan"); len(matches) == 0 {
		t.Fatal("/plan should appear in autocomplete")
	}
}
