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

// statusFor returns the HTML of the status fragment emitted for an event, or ""
// if the event emits no status fragment.
func statusFor(s *Server, e copilot.Event) string {
	for _, f := range s.handleEvent(e) {
		if f.Event == "status" {
			return f.HTML
		}
	}
	return ""
}

func TestSendShowsAbortControl(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/send", url.Values{"prompt": {"hi"}})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	str := string(body)
	if !strings.Contains(str, `id="status"`) || !strings.Contains(str, `hx-swap-oob`) {
		t.Fatalf("send should OOB-update #status with the live turn: %s", str)
	}
	if !strings.Contains(str, `hx-post="/abort"`) {
		t.Errorf("send should show an abort control while the turn runs: %s", str)
	}
}

func TestStatusFragmentActiveShowsAbort(t *testing.T) {
	s, _ := newTestServer()
	if got := statusFor(s, copilot.Event{Type: copilot.EvToolStart, Tool: "shell"}); !strings.Contains(got, `hx-post="/abort"`) {
		t.Errorf("active (tool running) status should include abort: %q", got)
	}
}

func TestStatusFragmentIdleHidesAbort(t *testing.T) {
	s, _ := newTestServer()
	if got := statusFor(s, copilot.Event{Type: copilot.EvIdle}); strings.Contains(got, "/abort") {
		t.Errorf("idle status should not include abort: %q", got)
	}
	s2, _ := newTestServer()
	if got := statusFor(s2, copilot.Event{Type: copilot.EvMessage, Text: "done"}); strings.Contains(got, "/abort") {
		t.Errorf("finished-message status should not include abort: %q", got)
	}
}

func TestAbortCallsClient(t *testing.T) {
	s, mock := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	// Establish a session via a send, then abort it.
	_, _ = http.PostForm(srv.URL+"/send", url.Values{"prompt": {"hi"}})
	resp, err := http.PostForm(srv.URL+"/abort", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(mock.Aborted) != 1 {
		t.Errorf("abort not forwarded to client: %v", mock.Aborted)
	}
}
