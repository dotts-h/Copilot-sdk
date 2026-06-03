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
	if !s.planMode {
		t.Fatal("planMode should be true after /plan")
	}
	if out := s.runCommand("/plan"); !strings.Contains(out, "plan mode off") {
		t.Fatalf("second /plan should toggle off: %s", out)
	}
	if s.planMode {
		t.Fatal("planMode should be false after toggling off")
	}
	// Explicit on/off.
	s.runCommand("/plan off")
	if s.planMode {
		t.Fatal("/plan off should clear plan mode")
	}
	s.runCommand("/plan on")
	if !s.planMode {
		t.Fatal("/plan on should set plan mode")
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
	if s.planMode {
		t.Fatal("approving a plan should exit plan mode")
	}
}

func TestPlanInAutocomplete(t *testing.T) {
	if matches := matchCommands("/plan"); len(matches) == 0 {
		t.Fatal("/plan should appear in autocomplete")
	}
}
