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

	"github.com/dotts-h/copilot-sdk/internal/copilot"
)

func planFor(s *Server, e copilot.Event) string {
	for _, f := range s.handleEvent(e) {
		if f.Event == "plan" {
			return f.HTML
		}
	}
	return ""
}

func samplePlan(id string) copilot.Event {
	return copilot.Event{Type: copilot.EvPlanReview, Plan: &copilot.PlanRequest{
		ID: id, Summary: "Refactor the parser", Plan: "1. extract lexer\n2. add tests",
		Actions: []string{"proceed", "edit plan"}, Recommended: "proceed",
	}}
}

func TestPlanReviewEmitsForm(t *testing.T) {
	s, _ := newTestServer()
	html := planFor(s, samplePlan("plan-1"))
	if html == "" {
		t.Fatal("EvPlanReview should emit a plan fragment")
	}
	for _, want := range []string{`hx-post="/plan/plan-1"`, "Refactor the parser", "proceed", "edit plan", `name="action"`, `name="feedback"`} {
		if !strings.Contains(html, want) {
			t.Errorf("plan form missing %q: %s", want, html)
		}
	}
	if len(s.plans) != 1 {
		t.Fatalf("plan request not tracked: %d", len(s.plans))
	}
}

func TestPlanReviewMarksRecommended(t *testing.T) {
	s, _ := newTestServer()
	html := planFor(s, samplePlan("plan-1"))
	if !strings.Contains(html, "recommended") {
		t.Errorf("recommended action should be marked: %s", html)
	}
}

func TestPlanApproveResolvesViaClient(t *testing.T) {
	s, mock := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	s.handleEvent(samplePlan("plan-1"))

	resp, err := http.PostForm(srv.URL+"/plan/plan-1", url.Values{"action": {"proceed"}, "feedback": {""}})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if len(mock.RespondedPlan) != 1 {
		t.Fatalf("plan decision not forwarded: %v", mock.RespondedPlan)
	}
	d := mock.RespondedPlan[0]
	if !d.Approved || d.Action != "proceed" {
		t.Fatalf("decision = %+v, want approved+proceed", d)
	}
	if len(s.plans) != 0 {
		t.Errorf("resolved plan should be dropped: %d remain", len(s.plans))
	}
	if !strings.Contains(string(body), `id="timeline"`) {
		t.Errorf("plan response should OOB-refresh the timeline: %s", string(body))
	}
}

func TestPlanDeclineWithFeedback(t *testing.T) {
	s, mock := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	s.handleEvent(samplePlan("plan-2"))

	_, err := http.PostForm(srv.URL+"/plan/plan-2", url.Values{"action": {""}, "feedback": {"add tests first"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(mock.RespondedPlan) != 1 {
		t.Fatalf("plan decision not forwarded: %v", mock.RespondedPlan)
	}
	d := mock.RespondedPlan[0]
	if d.Approved || d.Feedback != "add tests first" {
		t.Fatalf("decision = %+v, want declined+feedback", d)
	}
}

func TestPlanChangedEmitsSystemNote(t *testing.T) {
	s, _ := newTestServer()
	frags := s.handleEvent(copilot.Event{Type: copilot.EvPlanChanged, Text: "plan updated"})
	var found bool
	for _, f := range frags {
		if f.Event == "timeline" && strings.Contains(f.HTML, "plan updated") {
			found = true
		}
	}
	if !found {
		t.Errorf("plan-changed should add a system note to the timeline: %+v", frags)
	}
}

func TestClearResetsPlans(t *testing.T) {
	s, _ := newTestServer()
	s.handleEvent(samplePlan("plan-1"))
	out := s.runCommand("/clear")
	if len(s.plans) != 0 {
		t.Errorf("clear should drop pending plans: %d", len(s.plans))
	}
	if !strings.Contains(out, `id="plans"`) {
		t.Errorf("clear should OOB-clear the #plans container: %s", out)
	}
}

func TestChatPartialRendersPendingPlan(t *testing.T) {
	s, _ := newTestServer()
	s.handleEvent(samplePlan("plan-1"))
	if html := s.chatPartial(); !strings.Contains(html, `hx-post="/plan/plan-1"`) {
		t.Errorf("chat page should render the pending plan form: %s", html)
	}
}
