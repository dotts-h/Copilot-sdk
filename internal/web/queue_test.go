package web

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/copilot"
)

// newHTTP starts a test HTTP server for s and registers its cleanup.
func newHTTP(t *testing.T, s *Server) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return srv
}

// waitFor polls cond until it is true or the deadline passes.
func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", msg)
}

// busyAndQueue reads the server's turn state under the lock.
func (s *Server) busyAndQueue() (bool, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.busy, len(s.queue)
}

// TestSendWhileBusyQueues verifies a prompt sent during an in-flight turn is
// queued (not dispatched) and shows in the timeline immediately, then drains on
// the turn-end (EvIdle) event.
func TestSendWhileBusyQueues(t *testing.T) {
	s, mock := newTestServer()
	srv := newHTTP(t, s)

	// First prompt starts a turn → dispatched, server is busy.
	_ = postSend(t, srv, "first")
	waitFor(t, func() bool { return mock.SentCount() == 1 }, "first dispatched")
	if busy, _ := s.busyAndQueue(); !busy {
		t.Fatalf("server should be busy after first send")
	}

	// Second prompt while busy → queued, not yet sent.
	body := postSend(t, srv, "second")
	if !strings.Contains(body, "second") {
		t.Errorf("queued prompt should appear in timeline: %q", body)
	}
	if got := mock.SentCount(); got != 1 {
		t.Fatalf("queued prompt should not dispatch yet, sent=%d", got)
	}
	if _, qlen := s.busyAndQueue(); qlen != 1 {
		t.Fatalf("queue len = %d, want 1", qlen)
	}

	// Turn ends → the queued prompt drains and dispatches.
	s.handleEvent(copilot.Event{Type: copilot.EvIdle})
	waitFor(t, func() bool { return mock.SentCount() == 2 }, "queued prompt drained")
	if got := mock.SentAt(1); got != "second" {
		t.Errorf("drained prompt = %q, want %q", got, "second")
	}
	if busy, _ := s.busyAndQueue(); !busy {
		t.Errorf("server should remain busy while draining a queued turn")
	}

	// Final idle with an empty queue clears busy.
	s.handleEvent(copilot.Event{Type: copilot.EvIdle})
	if busy, _ := s.busyAndQueue(); busy {
		t.Errorf("server should be idle after queue drains")
	}
}

// TestSendWhenIdleDispatchesImmediately is the non-busy fast path.
func TestSendWhenIdleDispatchesImmediately(t *testing.T) {
	s, mock := newTestServer()
	srv := newHTTP(t, s)
	_ = postSend(t, srv, "hello")
	waitFor(t, func() bool { return mock.SentCount() == 1 }, "dispatched")
	if got := mock.SentAt(0); got != "hello" {
		t.Errorf("sent = %q", got)
	}
}

// TestAbortClearsQueue verifies aborting cancels queued prompts and resets busy.
func TestAbortClearsQueue(t *testing.T) {
	s, mock := newTestServer()
	srv := newHTTP(t, s)

	_ = postSend(t, srv, "first")
	waitFor(t, func() bool { return mock.SentCount() == 1 }, "first dispatched")
	_ = postSend(t, srv, "queued one")
	if _, qlen := s.busyAndQueue(); qlen != 1 {
		t.Fatalf("queue not populated: %d", qlen)
	}

	resp, err := srv.Client().Post(srv.URL+"/abort", "application/x-www-form-urlencoded", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	busy, qlen := s.busyAndQueue()
	if busy || qlen != 0 {
		t.Errorf("abort did not clear queue/busy: busy=%v qlen=%d", busy, qlen)
	}
	// Subsequent idle must not dispatch the cancelled prompt.
	s.handleEvent(copilot.Event{Type: copilot.EvIdle})
	if got := mock.SentCount(); got != 1 {
		t.Errorf("cancelled prompt should not dispatch, sent=%d", got)
	}
}
