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

// elicitReq is a representative multi-field elicitation request used across tests.
func elicitReq(id string) *copilot.ElicitRequest {
	return &copilot.ElicitRequest{
		ID: id, Message: "Configure deploy", Source: "deploy-mcp",
		Fields: []copilot.ElicitField{
			{Name: "env", Label: "Environment", Type: "string", Required: true,
				Enum: []string{"staging", "production"}, Default: "staging"},
			{Name: "replicas", Label: "Replicas", Type: "integer", Default: "3"},
			{Name: "migrate", Label: "Run migrations", Type: "boolean"},
		},
	}
}

// elicitFor returns the HTML of the "elicit" fragment emitted for an event, or "".
func elicitFor(s *Server, e copilot.Event) string {
	for _, f := range s.handleEvent(e) {
		if f.Event == "elicit" {
			return f.HTML
		}
	}
	return ""
}

func TestElicitationEmitsForm(t *testing.T) {
	s, _ := newTestServer()
	html := elicitFor(s, copilot.Event{Type: copilot.EvElicitation, Elicit: elicitReq("elicit-1")})
	if html == "" {
		t.Fatal("EvElicitation should emit an elicit fragment")
	}
	for _, want := range []string{
		`hx-post="/elicit/elicit-1"`, "Configure deploy", "deploy-mcp",
		`name="f.env"`, `<select`, "staging", "production",
		`name="f.replicas"`, `type="number"`,
		`name="f.migrate"`, `type="checkbox"`,
		`value="accept"`, `value="decline"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("elicit form missing %q:\n%s", want, html)
		}
	}
	if len(s.elicits) != 1 {
		t.Fatalf("elicitation not tracked: %d", len(s.elicits))
	}
}

func TestElicitDefaultSelectedAndRequiredMarked(t *testing.T) {
	s, _ := newTestServer()
	html := elicitFor(s, copilot.Event{Type: copilot.EvElicitation, Elicit: elicitReq("elicit-1")})
	if !strings.Contains(html, `value="staging" selected`) {
		t.Errorf("default enum option should be pre-selected: %s", html)
	}
	if !strings.Contains(html, `class="req"`) {
		t.Errorf("required field should be marked: %s", html)
	}
}

func TestElicitAcceptCoercesTypes(t *testing.T) {
	s, mock := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	s.handleEvent(copilot.Event{Type: copilot.EvElicitation, Elicit: elicitReq("elicit-1")})

	resp, err := http.PostForm(srv.URL+"/elicit/elicit-1", url.Values{
		"action":     {"accept"},
		"f.env":      {"production"},
		"f.replicas": {"5"},
		"f.migrate":  {"true"}, // checkbox present → true
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if len(mock.RespondedElicit) != 1 {
		t.Fatalf("expected one RespondElicit, got %v", mock.RespondedElicit)
	}
	d := mock.RespondedElicit[0]
	if d.Action != "accept" {
		t.Fatalf("action = %q, want accept", d.Action)
	}
	if d.Content["env"] != "production" {
		t.Errorf("env = %v, want production string", d.Content["env"])
	}
	if n, ok := d.Content["replicas"].(int64); !ok || n != 5 {
		t.Errorf("replicas = %#v, want int64(5)", d.Content["replicas"])
	}
	if b, ok := d.Content["migrate"].(bool); !ok || !b {
		t.Errorf("migrate = %#v, want bool(true)", d.Content["migrate"])
	}
	if len(s.elicits) != 0 {
		t.Errorf("resolved elicitation should be dropped: %d remain", len(s.elicits))
	}
	if !strings.Contains(string(body), `id="timeline"`) {
		t.Errorf("elicit response should OOB-refresh the timeline: %s", string(body))
	}
}

func TestElicitUncheckedBooleanIsFalse(t *testing.T) {
	s, mock := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	s.handleEvent(copilot.Event{Type: copilot.EvElicitation, Elicit: elicitReq("elicit-1")})

	// migrate omitted entirely (an unchecked checkbox submits nothing).
	_, err := http.PostForm(srv.URL+"/elicit/elicit-1", url.Values{
		"action": {"accept"}, "f.env": {"staging"},
	})
	if err != nil {
		t.Fatal(err)
	}
	d := mock.RespondedElicit[0]
	if b, ok := d.Content["migrate"].(bool); !ok || b {
		t.Errorf("unchecked checkbox should be false, got %#v", d.Content["migrate"])
	}
	// A blank integer field is omitted, not coerced to zero.
	if _, present := d.Content["replicas"]; present {
		t.Errorf("blank integer field should be omitted, got %v", d.Content["replicas"])
	}
}

func TestElicitDeclineSendsNoContent(t *testing.T) {
	s, mock := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	s.handleEvent(copilot.Event{Type: copilot.EvElicitation, Elicit: elicitReq("elicit-1")})

	_, err := http.PostForm(srv.URL+"/elicit/elicit-1", url.Values{"action": {"decline"}})
	if err != nil {
		t.Fatal(err)
	}
	d := mock.RespondedElicit[0]
	if d.Action != "decline" || d.Content != nil {
		t.Fatalf("decline should carry no content: %+v", d)
	}
}

func TestClearResetsElicits(t *testing.T) {
	s, _ := newTestServer()
	s.handleEvent(copilot.Event{Type: copilot.EvElicitation, Elicit: elicitReq("elicit-1")})
	out := s.runCommand("/clear")
	if len(s.elicits) != 0 {
		t.Errorf("clear should drop pending elicitations: %d", len(s.elicits))
	}
	if !strings.Contains(out, `id="elicits"`) {
		t.Errorf("clear should OOB-clear the #elicits container: %s", out)
	}
}

func TestChatPartialRendersPendingElicit(t *testing.T) {
	s, _ := newTestServer()
	s.handleEvent(copilot.Event{Type: copilot.EvElicitation, Elicit: elicitReq("elicit-1")})
	if html := s.chatPartial(); !strings.Contains(html, `hx-post="/elicit/elicit-1"`) {
		t.Errorf("chat page should render the pending elicit form: %s", html)
	}
}
