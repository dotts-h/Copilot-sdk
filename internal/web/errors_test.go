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

func TestEvErrorEndsTurn(t *testing.T) {
	s, _ := newTestServer()
	// Simulate an in-flight turn.
	s.mu.Lock()
	s.busy = true
	s.queue = []string{"queued"}
	s.turnStartMs = nowMs()
	s.mu.Unlock()

	status := statusFor(s, copilot.Event{Type: copilot.EvError, Err: errStr("rate_limit: slow down")})
	if status == "" {
		t.Fatal("EvError should emit a status fragment")
	}
	if strings.Contains(status, "stop") {
		t.Errorf("EvError status should be inactive (no abort button): %s", status)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy || len(s.queue) != 0 || s.turnStartMs != 0 {
		t.Fatalf("EvError should clear busy/queue/timer: busy=%v queue=%d ts=%d", s.busy, len(s.queue), s.turnStartMs)
	}
	// The error message lands in the transcript.
	found := false
	for _, turn := range s.state.Committed() {
		if strings.Contains(turn.Text, "rate_limit: slow down") {
			found = true
		}
	}
	if !found {
		t.Error("error message should appear in the transcript")
	}
}

func TestSendFailureSurfaced(t *testing.T) {
	s, mock := newTestServer()
	mock.SendErr = errStr("runtime gone")
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/send", url.Values{"prompt": {"hello"}})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if !strings.Contains(string(body), "send failed: runtime gone") {
		t.Errorf("send failure should be surfaced in the response: %s", string(body))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy {
		t.Error("a failed send should not leave the server busy")
	}
}

// errStr is a tiny error type so tests can assert on message text.
type errStr string

func (e errStr) Error() string { return string(e) }
