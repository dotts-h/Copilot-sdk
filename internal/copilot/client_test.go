package copilot

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"
)

// fakeSidecar reads requests from r and responds/emits on w, implementing just
// enough of the protocol to exercise the client.
func fakeSidecar(t *testing.T, r io.Reader, w io.Writer) {
	t.Helper()
	enc := json.NewEncoder(w)
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var req Request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			continue
		}
		switch req.Method {
		case "createSession":
			_ = enc.Encode(Frame{ID: req.ID, Result: mustJSON(createSessionResult{SessionID: "sess-1"})})
		case "send":
			// Ack the call, then stream an event sequence.
			_ = enc.Encode(Frame{ID: req.ID, Result: mustJSON(map[string]string{"messageId": "m1"})})
			_ = enc.Encode(Frame{Event: EventMessageDelta, Data: mustJSON(DeltaData{SessionID: "sess-1", DeltaContent: "Hello"})})
			_ = enc.Encode(Frame{Event: EventMessageDelta, Data: mustJSON(DeltaData{SessionID: "sess-1", DeltaContent: " world"})})
			_ = enc.Encode(Frame{Event: EventUsage, Data: mustJSON(UsageData{SessionID: "sess-1", Model: "gpt-5", InputTokens: 100, OutputTokens: 20})})
			_ = enc.Encode(Frame{Event: EventIdle, Data: mustJSON(map[string]string{"sessionId": "sess-1"})})
		case "abort":
			_ = enc.Encode(Frame{ID: req.ID, Result: mustJSON(map[string]bool{"ok": true})})
		case "boom":
			_ = enc.Encode(Frame{ID: req.ID, Error: "kaboom"})
		}
	}
}

func newPipedClient(t *testing.T) *StdioClient {
	t.Helper()
	cToS_r, cToS_w := io.Pipe() // client -> sidecar
	sToC_r, sToC_w := io.Pipe() // sidecar -> client
	go fakeSidecar(t, cToS_r, sToC_w)
	c := NewStdioClient(sToC_r, cToS_w, func() {
		_ = cToS_w.Close()
		_ = sToC_w.Close()
	})
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestStdioCreateSession(t *testing.T) {
	c := newPipedClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	id, err := c.CreateSession(ctx, SessionSpec{Model: "gpt-5", Streaming: true})
	if err != nil {
		t.Fatal(err)
	}
	if id != "sess-1" {
		t.Fatalf("unexpected session id %q", id)
	}
}

func TestStdioSendStreamsEvents(t *testing.T) {
	c := newPipedClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := c.CreateSession(ctx, SessionSpec{Model: "gpt-5"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Send(ctx, "sess-1", "hi"); err != nil {
		t.Fatal(err)
	}

	var text string
	var sawUsage, sawIdle bool
	timeout := time.After(2 * time.Second)
	for !sawIdle {
		select {
		case ev := <-c.Events():
			switch ev.Type {
			case EventMessageDelta:
				var d DeltaData
				_ = json.Unmarshal(ev.Data, &d)
				text += d.DeltaContent
			case EventUsage:
				var u UsageData
				_ = json.Unmarshal(ev.Data, &u)
				if u.InputTokens != 100 || u.OutputTokens != 20 || u.Model != "gpt-5" {
					t.Fatalf("bad usage event: %+v", u)
				}
				sawUsage = true
			case EventIdle:
				sawIdle = true
			}
		case <-timeout:
			t.Fatalf("timed out; text so far %q usage=%v", text, sawUsage)
		}
	}
	if text != "Hello world" {
		t.Fatalf("streamed text = %q, want %q", text, "Hello world")
	}
	if !sawUsage {
		t.Fatal("never saw usage event")
	}
}

func TestStdioSidecarError(t *testing.T) {
	c := newPipedClient(t)
	ctx := context.Background()
	_, err := c.call(ctx, "boom", nil)
	if err == nil {
		t.Fatal("expected error from sidecar")
	}
}

func TestStdioContextCancel(t *testing.T) {
	// Sidecar that accepts (drains) requests but never replies: the call should
	// still return once the context deadline elapses.
	cToS_r, cToS_w := io.Pipe()
	sToC_r, sToC_w := io.Pipe()
	go func() {
		s := bufio.NewScanner(cToS_r)
		for s.Scan() { // drain and discard; never respond
		}
	}()
	c := NewStdioClient(sToC_r, cToS_w, func() {
		_ = cToS_w.Close()
		_ = sToC_w.Close()
	})
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := c.CreateSession(ctx, SessionSpec{Model: "gpt-5"})
	if err == nil {
		t.Fatal("expected context deadline error")
	}
}

func TestClosedClientRejectsCalls(t *testing.T) {
	c := newPipedClient(t)
	_ = c.Close()
	_, err := c.CreateSession(context.Background(), SessionSpec{Model: "gpt-5"})
	if err != ErrClosed {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestMockClient(t *testing.T) {
	m := NewMockClient()
	defer m.Close()
	if _, err := m.CreateSession(context.Background(), SessionSpec{}); err != nil {
		t.Fatal(err)
	}
	if err := m.Send(context.Background(), "mock-session", "hello"); err != nil {
		t.Fatal(err)
	}
	if len(m.Sent) != 1 || m.Sent[0] != "hello" {
		t.Fatalf("Send not recorded: %v", m.Sent)
	}
	m.Emit(Event{Type: EventMessage, Data: mustJSON(MessageData{Content: "hi"})})
	ev := <-m.Events()
	if ev.Type != EventMessage {
		t.Fatalf("unexpected event %q", ev.Type)
	}
}
