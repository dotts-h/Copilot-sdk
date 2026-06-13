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

// askFor returns the HTML of the "ask" fragment emitted for an event, or "".
func askFor(s *Server, e copilot.Event) string {
	for _, f := range s.handleEvent(e) {
		if f.Event == "ask" {
			return f.HTML
		}
	}
	return ""
}

func TestUserInputEmitsAskForm(t *testing.T) {
	s, _ := newTestServer()
	html := askFor(s, copilot.Event{Type: copilot.EvUserInput, Input: &copilot.InputRequest{
		ID: "ask-1", Question: "Which file?", Choices: []string{"a.go", "b.go"}, AllowFreeform: true,
	}})
	if html == "" {
		t.Fatal("EvUserInput should emit an ask fragment")
	}
	for _, want := range []string{`hx-post="/ask/ask-1"`, "Which file?", "a.go", "b.go", `name="answer"`} {
		if !strings.Contains(html, want) {
			t.Errorf("ask form missing %q: %s", want, html)
		}
	}
	// The request should be tracked so the chat page can re-render it.
	if len(s.inputs) != 1 {
		t.Fatalf("input request not tracked: %d", len(s.inputs))
	}
}

func TestUserInputNoFreeformOmitsTextInput(t *testing.T) {
	s, _ := newTestServer()
	html := askFor(s, copilot.Event{Type: copilot.EvUserInput, Input: &copilot.InputRequest{
		ID: "ask-1", Question: "Pick", Choices: []string{"yes", "no"}, AllowFreeform: false,
	}})
	if strings.Contains(html, `type="text"`) {
		t.Errorf("no-freeform ask should not render a text input: %s", html)
	}
}

func TestAskResolvesViaClient(t *testing.T) {
	s, mock := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	// Seed a pending input request.
	s.handleEvent(copilot.Event{Type: copilot.EvUserInput, Input: &copilot.InputRequest{
		ID: "ask-1", Question: "Which?", Choices: []string{"a", "b"}, AllowFreeform: true,
	}})

	resp, err := http.PostForm(srv.URL+"/ask/ask-1", url.Values{"answer": {"", "a"}})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if len(mock.RespondedInput) != 1 || mock.RespondedInput[0].Answer != "a" {
		t.Fatalf("answer not forwarded to client: %v", mock.RespondedInput)
	}
	if len(s.inputs) != 0 {
		t.Errorf("resolved input should be dropped: %d remain", len(s.inputs))
	}
	if !strings.Contains(string(body), `id="timeline"`) {
		t.Errorf("ask response should OOB-refresh the timeline: %s", string(body))
	}
}

func TestAskPicksFirstNonEmptyAnswer(t *testing.T) {
	s, mock := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	s.handleEvent(copilot.Event{Type: copilot.EvUserInput, Input: &copilot.InputRequest{
		ID: "ask-2", Question: "Name?", AllowFreeform: true,
	}})
	_, err := http.PostForm(srv.URL+"/ask/ask-2", url.Values{"answer": {"", "custom-name"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(mock.RespondedInput) != 1 || mock.RespondedInput[0].Answer != "custom-name" {
		t.Fatalf("freeform answer not forwarded: %v", mock.RespondedInput)
	}
}

func TestClearResetsInputs(t *testing.T) {
	s, _ := newTestServer()
	s.handleEvent(copilot.Event{Type: copilot.EvUserInput, Input: &copilot.InputRequest{ID: "ask-1", Question: "q"}})
	out := s.runCommand("/clear")
	if len(s.inputs) != 0 {
		t.Errorf("clear should drop pending inputs: %d", len(s.inputs))
	}
	if !strings.Contains(out, `id="asks"`) {
		t.Errorf("clear should OOB-clear the #asks container: %s", out)
	}
}

func TestChatPartialRendersPendingAsk(t *testing.T) {
	s, _ := newTestServer()
	s.handleEvent(copilot.Event{Type: copilot.EvUserInput, Input: &copilot.InputRequest{
		ID: "ask-1", Question: "Which?", Choices: []string{"a"},
	}})
	if html := s.chatPartial(); !strings.Contains(html, `hx-post="/ask/ask-1"`) {
		t.Errorf("chat page should render the pending ask form: %s", html)
	}
}
