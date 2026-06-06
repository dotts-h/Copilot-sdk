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

// seedSnippets installs a couple of library snippets for the test server.
func seedSnippets(s *Server) {
	s.forge.Snippets = []ctxforge.Snippet{
		{ID: "review-pr", Name: "Review this PR", Body: "Review the current diff and summarise the risks."},
		{ID: "explain", Name: "Explain code", Body: "Explain the selected code step by step."},
	}
}

// The composer autocomplete surfaces snippets alongside commands, marked as a
// snippet and carrying the body for client-side insertion.
func TestCommandsMenuIncludesSnippets(t *testing.T) {
	s, _ := newTestServer()
	seedSnippets(s)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	get := func(q string) string {
		resp, err := http.Get(srv.URL + "/commands?prompt=" + url.QueryEscape(q))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}

	// "/re" prefixes the review-pr snippet trigger.
	body := get("/re")
	for _, want := range []string{"cmd-snippet", "fillSnippet", "/review-pr", "Review the current diff", "Review this PR"} {
		if !strings.Contains(body, want) {
			t.Errorf("snippet menu for /re missing %q: %q", want, body)
		}
	}

	// A bare slash lists snippets too (alongside commands).
	if all := get("/"); !strings.Contains(all, "/explain") || !strings.Contains(all, "fillCmd") {
		t.Errorf("bare-slash menu missing snippet or command: %q", all)
	}
}

// A snippet body with HTML is escaped in the data attribute (ADR-0001).
func TestCommandsMenuEscapesSnippetBody(t *testing.T) {
	s, _ := newTestServer()
	s.forge.Snippets = []ctxforge.Snippet{
		{ID: "xss", Name: "<b>bad</b>", Body: `<script>alert("x")</script>`},
	}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/commands?prompt=" + url.QueryEscape("/xss"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	body := string(b)
	if strings.Contains(body, "<script>alert") {
		t.Errorf("snippet body not escaped in menu: %q", body)
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Errorf("expected escaped snippet body, got: %q", body)
	}
}

// A reserved command name is never shadowed by a snippet with the same id: the
// command wins and the colliding snippet is not offered as a snippet entry.
func TestReservedCommandBeatsSnippet(t *testing.T) {
	s, _ := newTestServer()
	s.forge.Snippets = []ctxforge.Snippet{
		{ID: "clear", Name: "Not a command", Body: "this should never run /clear"},
	}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/commands?prompt=" + url.QueryEscape("/clear"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(b), "cmd-snippet") {
		t.Errorf("reserved-name snippet should not appear as a snippet entry: %q", b)
	}

	// Submitting /clear runs the command, not the snippet expansion.
	resp2, err := http.PostForm(srv.URL+"/send", url.Values{"prompt": {"/clear"}})
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if got := s.client.(interface{ SentCount() int }).SentCount(); got != 0 {
		t.Errorf("/clear should not have sent a prompt, sent %d", got)
	}
}

// The Snippets nav page lists the library entries.
func TestSnippetsPageListsSnippets(t *testing.T) {
	s, _ := newTestServer()
	seedSnippets(s)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/page/snippets")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	body := string(b)
	for _, want := range []string{"Snippets", "Review this PR", "Explain code", "/snippets/review-pr/edit"} {
		if !strings.Contains(body, want) {
			t.Errorf("snippets page missing %q: %q", want, body)
		}
	}
}

// CRUD through the handlers: create, then delete, via the validated forge path.
func TestSnippetCreateAndDelete(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	_, err := http.PostForm(srv.URL+"/snippets", url.Values{
		"id": {"daily"}, "name": {"Daily standup"}, "body": {"Summarise what changed today."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sn := s.forge.Snippet("daily"); sn == nil || sn.Body != "Summarise what changed today." {
		t.Fatalf("snippet not created: %v", sn)
	}

	// An invalid create re-renders the form with the error, no persistence.
	resp, err := http.PostForm(srv.URL+"/snippets", url.Values{"id": {"bad"}, "name": {""}, "body": {"x"}})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), "name is required") {
		t.Errorf("invalid create should surface the error: %q", b)
	}
	if s.forge.Snippet("bad") != nil {
		t.Error("invalid snippet should not persist")
	}

	// Delete.
	_, err = http.PostForm(srv.URL+"/snippets/daily/delete", nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.forge.Snippet("daily") != nil {
		t.Error("snippet not deleted")
	}
}

// Submitting a bare /trigger expands the snippet body and sends it as a prompt.
func TestSlashSnippetExpandsAndSends(t *testing.T) {
	s, mock := newTestServer()
	seedSnippets(s)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/send", url.Values{"prompt": {"/review-pr"}})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if mock.SentCount() != 1 {
		t.Fatalf("snippet trigger should send exactly one prompt, sent %d", mock.SentCount())
	}
	if got := mock.SentAt(0); got != "Review the current diff and summarise the risks." {
		t.Errorf("snippet body not sent: %q", got)
	}
}

// An unknown /trigger (no matching snippet, not a command) stays an unknown
// command — it must not be silently sent to the model.
func TestUnknownSlashIsNotSent(t *testing.T) {
	s, mock := newTestServer()
	seedSnippets(s)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/send", url.Values{"prompt": {"/nope"}})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if mock.SentCount() != 0 {
		t.Errorf("unknown command should not send a prompt, sent %d", mock.SentCount())
	}
	if !strings.Contains(string(b), "unknown command") {
		t.Errorf("expected unknown-command note: %q", b)
	}
}
