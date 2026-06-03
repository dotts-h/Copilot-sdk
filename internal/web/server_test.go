package web

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/copilot"
)

func TestWriteSSE(t *testing.T) {
	var b strings.Builder
	writeSSE(&b, "msg-delta", "<span>hi</span>")
	got := b.String()
	want := "event: msg-delta\ndata: <span>hi</span>\n\n"
	if got != want {
		t.Errorf("writeSSE = %q, want %q", got, want)
	}

	b.Reset()
	writeSSE(&b, "turn-end", "")
	if got := b.String(); got != "event: turn-end\ndata: \n\n" {
		t.Errorf("empty-data SSE = %q", got)
	}

	b.Reset()
	writeSSE(&b, "x", "a\nb")
	if got := b.String(); got != "event: x\ndata: a\ndata: b\n\n" {
		t.Errorf("multiline SSE = %q", got)
	}
}

func TestIndexServes(t *testing.T) {
	s := New(Options{Client: copilot.NewMockClient()})
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	for _, sub := range []string{"my-orchestra", `sse-connect="/events"`, "/static/htmx.min.js", `id="cur-msg"`} {
		if !strings.Contains(string(body), sub) {
			t.Errorf("index missing %q", sub)
		}
	}
}

func TestStaticAssetsServed(t *testing.T) {
	s := New(Options{Client: copilot.NewMockClient()})
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/static/htmx.min.js")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("htmx status = %d", resp.StatusCode)
	}
}

func TestSendEchoesUserBubble(t *testing.T) {
	mock := copilot.NewMockClient()
	s := New(Options{Client: mock})
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/send", url.Values{"prompt": {"hello there"}})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "hello there") || !strings.Contains(string(body), `class="turn user"`) {
		t.Errorf("send did not echo user bubble: %q", body)
	}
	if len(mock.Sent) != 1 || mock.Sent[0] != "hello there" {
		t.Errorf("prompt not forwarded to client: %v", mock.Sent)
	}
}

func TestSendEmptyPromptNoContent(t *testing.T) {
	mock := copilot.NewMockClient()
	s := New(Options{Client: mock})
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/send", url.Values{"prompt": {"   "}})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("empty prompt status = %d, want 204", resp.StatusCode)
	}
	if len(mock.Sent) != 0 {
		t.Errorf("empty prompt should not be sent: %v", mock.Sent)
	}
}

// TestEventsStreamsFragments drives the mock event stream and asserts the SSE
// endpoint renders the message contract end-to-end (the walking skeleton).
func TestEventsStreamsFragments(t *testing.T) {
	mock := copilot.NewMockClient()
	s := New(Options{Client: mock})
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type = %q", ct)
	}

	go func() {
		mock.Emit(copilot.Event{Type: copilot.EvMessageDelta, Text: "hi "})
		mock.Emit(copilot.Event{Type: copilot.EvMessage, Text: "hi there"})
		mock.Emit(copilot.Event{Type: copilot.EvIdle})
	}()

	want := map[string]bool{"msg-delta": false, "msg-done": false, "turn-end": false}
	remaining := len(want)
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		line := sc.Text()
		name, ok := strings.CutPrefix(line, "event: ")
		if !ok {
			continue
		}
		if seen, tracked := want[name]; tracked && !seen {
			want[name] = true
			remaining--
			if remaining == 0 {
				return // got every expected fragment
			}
		}
	}
	t.Fatalf("did not observe all SSE fragments: %v (scanner err: %v)", want, sc.Err())
}
