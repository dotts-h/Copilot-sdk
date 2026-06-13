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

// The chat composer is a <textarea> (not a single-line <input>), so a multi-line
// prompt — or a multi-line snippet inserted via fillSnippet — keeps its newlines
// (TECH_DEBT #15 / ADR-0015). It must still carry the autocomplete hx-get and the
// after-request reset/focus guard from REGRESSIONS #8.
func TestComposerRendersTextarea(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	body := string(b)

	if !strings.Contains(body, `<textarea name="prompt"`) {
		t.Errorf("composer should render a <textarea name=\"prompt\">: %q", body)
	}
	// The old single-line input must be gone (its value-sanitisation is what
	// flattened multi-line snippets — TECH_DEBT #15).
	if strings.Contains(body, `<input type="text" name="prompt"`) {
		t.Errorf("composer should no longer use a single-line <input name=\"prompt\">: %q", body)
	}
	// Autocomplete wiring is preserved on the textarea.
	for _, want := range []string{`hx-get="/commands"`, `hx-trigger="keyup changed delay:40ms"`} {
		if !strings.Contains(body, want) {
			t.Errorf("composer textarea missing autocomplete wiring %q: %q", want, body)
		}
	}
	// The after-request handler still guards on event.target === this (REGRESSIONS
	// #8: don't wipe the field on the autocomplete GET) and refocuses the textarea.
	if !strings.Contains(body, "event.target !== this") {
		t.Errorf("composer must keep the event.target===this after-request guard: %q", body)
	}
	if !strings.Contains(body, `querySelector('textarea')`) {
		t.Errorf("composer after-request should refocus the textarea: %q", body)
	}
}

// The composer JS restores Enter-to-send (a <textarea> inserts a newline on Enter
// natively) and inserts a newline on Shift-Enter, and autosizes the field so an
// inserted multi-line snippet is visible.
func TestComposerKeydownAndAutosizeWired(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	body := string(b)

	for _, want := range []string{
		"function composerKeydown", // Enter/Shift-Enter handler
		"shiftKey",                 // Shift-Enter falls through to a newline
		"requestSubmit",            // Enter submits the form through htmx
		"function autosize",        // grow the textarea with its content
		`onkeydown="return composerKeydown(event)"`,
		`oninput="autosize(this)"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("composer JS missing %q: %q", want, body)
		}
	}
}

// A multi-line snippet body keeps its newlines in the autocomplete data-body
// attribute, so fillSnippet inserts it into the textarea with line breaks intact.
// (The flatten in TECH_DEBT #15 was purely the client-side <input>; the server has
// always carried the newlines — this pins that and pairs with the e2e insert spec.)
func TestCommandsMenuPreservesMultilineSnippetBody(t *testing.T) {
	s, _ := newTestServer()
	s.forge.Snippets = []ctxforge.Snippet{
		{ID: "multi", Name: "Multi-line", Body: "line one\nline two\nline three"},
	}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/commands?prompt=" + url.QueryEscape("/multi"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	body := string(b)

	if !strings.Contains(body, "line one\nline two\nline three") {
		t.Errorf("multi-line snippet body should keep its newlines in the menu: %q", body)
	}
}
