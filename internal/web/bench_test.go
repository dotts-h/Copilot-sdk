package web

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/convo"
	"github.com/dotts-h/copilot-sdk/internal/copilot"
)

// This file is the performance layer: micro-benchmarks for the hot rendering and
// reducer paths, plus a concurrency stress test that drives the full HTTP
// surface from many goroutines to flush out data races and lock-ordering
// deadlocks (run with -race).

// bigState builds a representative transcript of n exchanges: a user prompt, a
// reasoning block, a completed tool call, and an assistant answer each.
func bigState(n int) *convo.State {
	st := &convo.State{}
	for i := 0; i < n; i++ {
		st.AddUser(fmt.Sprintf("user message number %d with a fair amount of text to render", i))
		st.AppendReasoning("considering the request and weighing a couple of options before answering ")
		id := fmt.Sprintf("tool-%d", i)
		st.ToolStart(id, "bash", fmt.Sprintf("echo run-%d", i))
		st.ToolProgress(id, "running…")
		st.ToolEnd(id, "ok: command completed with some output text", true)
		st.AppendDelta("Here is a reasonably sized assistant answer that spans a sentence or two. ")
		st.Finish("")
	}
	return st
}

func BenchmarkRenderTimelineSmall(b *testing.B) {
	st := bigState(5)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = renderTimelineInner(st)
	}
}

func BenchmarkRenderTimelineLarge(b *testing.B) {
	st := bigState(200)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = renderTimelineInner(st)
	}
}

func BenchmarkChatPartial(b *testing.B) {
	s, _ := newTestServer()
	s.state = *bigState(50)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.chatPartial()
	}
}

// BenchmarkHandleEventDelta measures the SSE fast path: an incremental message
// delta after the live kind is already message (the per-token hot loop).
func BenchmarkHandleEventDelta(b *testing.B) {
	s, _ := newTestServer()
	s.handleEvent(copilot.Event{Type: copilot.EvMessageDelta, Text: "warmup "}) // prime live kind
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.handleEvent(copilot.Event{Type: copilot.EvMessageDelta, Text: "tok "})
	}
}

// BenchmarkHandleEventToolCycle measures a full structural tool lifecycle, which
// re-renders the timeline each step.
func BenchmarkHandleEventToolCycle(b *testing.B) {
	s, _ := newTestServer()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := fmt.Sprintf("b-%d", i)
		_ = s.handleEvent(copilot.Event{Type: copilot.EvToolStart, Tool: "bash",
			ToolCall: &copilot.ToolCall{ID: id, Name: "bash", Args: "echo hi"}})
		_ = s.handleEvent(copilot.Event{Type: copilot.EvToolEnd,
			ToolCall: &copilot.ToolCall{ID: id, Result: "hi", Success: true}})
	}
}

// TestConcurrentMultiSessionLoad hammers the full HTTP handler from many
// goroutines across many independent sessions while a parallel cohort reads
// pages and opens SSE streams. Its job is to surface races (run with -race) and
// lock-ordering deadlocks (it fails if the batch does not finish within budget),
// not to benchmark throughput.
func TestConcurrentMultiSessionLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("load test skipped in -short mode")
	}
	hub, mock := newTestHub()

	const (
		sessions    = 12
		sendsEach   = 40
		readersEach = 5
	)
	ids := make([]string, sessions)
	for i := range ids {
		ids[i] = fmt.Sprintf("load-%d", i)
		hub.newSession(ids[i])
	}
	srv := httptest.NewServer(hub.Handler())
	defer srv.Close()

	client := &http.Client{Timeout: 10 * time.Second}
	do := func(method, path, sid, body string) (int, error) {
		var rdr io.Reader
		if body != "" {
			rdr = strings.NewReader(body)
		}
		req, _ := http.NewRequest(method, srv.URL+path, rdr)
		if body != "" {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		req.AddCookie(&http.Cookie{Name: cookieName, Value: sid})
		resp, err := client.Do(req)
		if err != nil {
			return 0, err
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		return resp.StatusCode, nil
	}

	var (
		wg     sync.WaitGroup
		errs   atomic.Int64
		bad    atomic.Int64
		okResp atomic.Int64
	)
	pages := []string{"chat", "telemetry", "skills", "instructions", "agents", "models", "settings", "help"}

	deadline := time.Now()
	for _, sid := range ids {
		// Writers: concurrent sends to this session.
		for j := 0; j < sendsEach; j++ {
			wg.Add(1)
			go func(sid string, j int) {
				defer wg.Done()
				code, err := do(http.MethodPost, "/send", sid,
					url.Values{"prompt": {fmt.Sprintf("%s-msg-%d", sid, j)}}.Encode())
				if err != nil {
					errs.Add(1)
					return
				}
				if code >= 500 {
					bad.Add(1)
				} else {
					okResp.Add(1)
				}
			}(sid, j)
		}
		// Readers: concurrent page reads racing against the writers' mutations.
		for r := 0; r < readersEach; r++ {
			wg.Add(1)
			go func(sid string, r int) {
				defer wg.Done()
				code, err := do(http.MethodGet, "/page/"+pages[r%len(pages)], sid, "")
				if err != nil {
					errs.Add(1)
					return
				}
				if code >= 500 {
					bad.Add(1)
				}
			}(sid, r)
		}
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("concurrent load did not finish within 30s — possible deadlock")
	}
	elapsed := time.Since(deadline)

	if e := errs.Load(); e > 0 {
		t.Errorf("%d transport errors under load", e)
	}
	if b := bad.Load(); b > 0 {
		t.Errorf("%d server errors (5xx) under load", b)
	}
	// Every session's first send dispatches to the client; the rest queue behind
	// the in-flight turn (no EvIdle is emitted to drain them). So the client sees
	// exactly one Send per session, and no send is lost or double-dispatched.
	if got := mock.SentCount(); got != sessions {
		t.Errorf("client saw %d sends, want %d (one dispatch per session; remainder queued)", got, sessions)
	}
	t.Logf("handled %d sends + reads across %d sessions in %s (race-clean)", okResp.Load(), sessions, elapsed)
}
