package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/pause"
)

// waitForPause blocks until exactly one pause is registered and returns its id, so
// a test can resolve the pause the escalate goroutine just parked on.
func waitForPause(t *testing.T, s *Server) string {
	t.Helper()
	for i := 0; i < 500; i++ {
		if p := s.pauses.Pending(); len(p) == 1 {
			return p[0].ID
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("a pause was never registered")
	return ""
}

// fakeClock is a mutex-guarded, advanceable time source so a test can make the
// escalate back-channel's paused-duration accounting deterministic: the lane stamps
// pausedAt at park and reads now() again when the pause closes, so advancing between
// the two yields an exact span. now() is read from the escalate goroutine, so the
// lock matters.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock(t time.Time) *fakeClock { return &fakeClock{t: t} }

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// AC: a completed run records, per lane, how many times it parked on a human and for
// how long (S6) — the orchestration history's "where were humans the bottleneck?"
// signal. Drives the escalate back-channel end-to-end with an injected clock so the
// paused span is exact, then maps the finished run and asserts the per-lane fields.
func TestRunRecordsLanePauseCountAndDuration(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	clk := newFakeClock(time.Date(2026, 6, 11, 9, 0, 0, 0, time.UTC))
	s.now = clk.now

	run := startParallelRun(s) // lanes s0, s1 running with session ids

	result := make(chan string, 1)
	go func() {
		result <- s.escalate(escalateReq{
			laneSession: "s0", agentID: "s0", kind: pause.KindInput,
			message: "which branch?", caps: []pause.Cap{pause.CapContinue, pause.CapCancel},
		})
	}()

	id := waitForPause(t, s)
	clk.advance(30 * time.Second) // 30s pass while parked
	if _, err := http.PostForm(srv.URL+"/pause/"+id, url.Values{
		"action": {"continue"}, "payload": {"use main"},
	}); err != nil {
		t.Fatal(err)
	}
	<-result // escalate returned → the lane's pause is closed out

	s.mu.Lock()
	rec := runRecord(run)
	s.mu.Unlock()
	if rec.Lanes[0].Pauses != 1 {
		t.Fatalf("lane 0 should record 1 pause, got %+v", rec.Lanes[0])
	}
	if rec.Lanes[0].PausedMs != 30_000 {
		t.Fatalf("lane 0 should record 30s paused, got %d ms", rec.Lanes[0].PausedMs)
	}
	if rec.Lanes[1].Pauses != 0 || rec.Lanes[1].PausedMs != 0 {
		t.Fatalf("lane 1 never parked, should be zero: %+v", rec.Lanes[1])
	}
}

// Aborting a paused run still attributes the lane's pause (count + the time it was
// parked up to the abort), and the pause resolves exactly once — the S4 idempotency
// invariant. Asserts the PERSISTED record (what abortRun writes), not a hand-rolled
// runRecord after the fact: abort is the one completion path that records while a lane
// is still parked, so the open span must be folded in BEFORE recordRun runs (the
// escalate goroutine's closeLanePause can't re-acquire s.mu until abortRun returns).
func TestAbortedPausedLaneStillRecordsPause(t *testing.T) {
	s, _ := newTestServer()
	rs := withRunStore(s)
	clk := newFakeClock(time.Date(2026, 6, 11, 9, 0, 0, 0, time.UTC))
	s.now = clk.now
	startParallelRun(s)

	result := make(chan string, 1)
	go func() {
		result <- s.escalate(escalateReq{laneSession: "s0", agentID: "s0",
			message: "blocked", caps: []pause.Cap{pause.CapContinue, pause.CapCancel}})
	}()
	waitForPause(t, s)

	clk.advance(10 * time.Second)
	s.abortRun(t.Context()) // records the run synchronously, with the span closed
	if got := <-result; !strings.Contains(got, "cancelled") {
		t.Fatalf("abort should cancel the pause, got %q", got)
	}

	s.mu.Lock()
	pending := len(s.pauses.Pending())
	s.mu.Unlock()
	if pending != 0 {
		t.Fatalf("abort should resolve the pause exactly once, %d still pending", pending)
	}
	recs := rs.Records()
	if len(recs) != 1 {
		t.Fatalf("abort should persist exactly one run, got %d", len(recs))
	}
	if l := recs[0].Lanes[0]; l.Pauses != 1 || l.PausedMs != 10_000 {
		t.Fatalf("an aborted-while-parked lane should record its pause (full span): %+v", l)
	}
}

// The out-of-band attention marker (S6) counts PENDING PAUSES — every parked lane or
// sub-agent that needs the human, so a lane escalation with no registry sub-agent
// still raises the title/favicon dot — and rides every pause re-render so the signal
// never drifts from the ledger.
func TestAttentionMarkerCountsPendingPauses(t *testing.T) {
	s, _ := newTestServer()
	if f := s.attentionFrag(); f.Event != "attention" || !strings.Contains(f.HTML, `data-count="0"`) {
		t.Fatalf("no pending pause should mark count 0: %+v", f)
	}
	s.pauses.Register(pause.Pause{AgentID: "a", Kind: pause.KindInput})
	s.pauses.Register(pause.Pause{AgentID: "b", Kind: pause.KindIssue})
	if f := s.attentionFrag(); !strings.Contains(f.HTML, `data-count="2"`) {
		t.Fatalf("two pending pauses should mark count 2: %+v", f)
	}
	// Every pause re-render carries the attention marker so the dot tracks the ledger.
	var sawAttention bool
	for _, f := range s.pauseFrags() {
		if f.Event == "attention" {
			sawAttention = true
		}
	}
	if !sawAttention {
		t.Error("pauseFrags must include the attention marker so the out-of-band dot updates live")
	}
}

// AC2: a sub-agent calls escalate → a pause event → POST /pause/{id} continue with a
// payload → the tool returns the payload to the sub-agent (seam test, no browser).
func TestEscalateContinueReturnsPayload(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	result := make(chan string, 1)
	go func() {
		result <- s.escalate(escalateReq{
			agentID: "sub-1", kind: pause.KindInput, message: "which branch?",
			caps: []pause.Cap{pause.CapContinue, pause.CapCancel},
		})
	}()

	id := waitForPause(t, s)
	resp, err := http.PostForm(srv.URL+"/pause/"+id, url.Values{
		"action": {"continue"}, "payload": {"use main"},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if got := <-result; got != "use main" {
		t.Fatalf("escalate returned %q, want the human's payload", got)
	}
	if p := s.pauses.Pending(); len(p) != 0 {
		t.Fatalf("the resolved pause should be cleared, still have %d", len(p))
	}
}

// AC2: cancel returns a wrap-up directive (not the payload) to the sub-agent.
func TestEscalateCancelReturnsDirective(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	result := make(chan string, 1)
	go func() {
		result <- s.escalate(escalateReq{
			agentID: "sub-1", kind: pause.KindIssue, message: "stuck",
			caps: []pause.Cap{pause.CapContinue, pause.CapCancel},
		})
	}()

	id := waitForPause(t, s)
	if _, err := http.PostForm(srv.URL+"/pause/"+id, url.Values{"action": {"cancel"}}); err != nil {
		t.Fatal(err)
	}
	if got := <-result; !strings.Contains(got, "cancelled") {
		t.Fatalf("cancel should return a wrap-up directive, got %q", got)
	}
}

// A duplicate POST after a pause already resolved is a harmless no-op (the
// idempotency invariant at the HTTP seam).
func TestPauseDoubleResolveIsNoOp(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	result := make(chan string, 1)
	go func() {
		result <- s.escalate(escalateReq{agentID: "sub-1", message: "go?",
			caps: []pause.Cap{pause.CapContinue, pause.CapCancel}})
	}()
	id := waitForPause(t, s)

	if _, err := http.PostForm(srv.URL+"/pause/"+id, url.Values{"action": {"continue"}, "payload": {"yes"}}); err != nil {
		t.Fatal(err)
	}
	if got := <-result; got != "yes" {
		t.Fatalf("first resolution should win: %q", got)
	}
	// A second POST for the same id changes nothing and must not panic or block.
	resp, err := http.PostForm(srv.URL+"/pause/"+id, url.Values{"action": {"cancel"}})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("duplicate resolve status = %d", resp.StatusCode)
	}
}

// AC3: a lane parks as input-required (the run stays live), parallel siblings keep
// streaming, and continue resumes the lane to running.
func TestLaneParksInputRequiredSiblingsStreamThenResumes(t *testing.T) {
	s, _ := newTestServer()
	run := startParallelRun(s) // lanes s0, s1 both running
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	result := make(chan string, 1)
	go func() {
		result <- s.escalate(escalateReq{
			laneSession: "s0", agentID: "s0", kind: pause.KindIssue,
			message: "blocked on tests", caps: []pause.Cap{pause.CapContinue, pause.CapCancel},
		})
	}()
	id := waitForPause(t, s)

	s.mu.Lock()
	st0, st1, allSettled := run.lanes[0].status, run.lanes[1].status, run.allSettled()
	s.mu.Unlock()
	if st0 != laneInputRequired {
		t.Fatalf("lane 0 should park input-required, got %v", st0)
	}
	if st1 != laneRunning {
		t.Fatalf("sibling lane 1 should keep running, got %v", st1)
	}
	if allSettled {
		t.Fatal("a run with a parked (non-terminal) lane must not be settled")
	}

	// The sibling streams while lane 0 is parked.
	s.handleEvent(copilot.Event{Type: copilot.EvMessageDelta, SessionID: "s1", Text: "beta streams"})
	s.mu.Lock()
	beta := run.lanes[1].text
	s.mu.Unlock()
	if !strings.Contains(beta, "beta streams") {
		t.Fatalf("a sibling should keep streaming while a lane is parked, got %q", beta)
	}

	if _, err := http.PostForm(srv.URL+"/pause/"+id, url.Values{"action": {"continue"}, "payload": {"skip it"}}); err != nil {
		t.Fatal(err)
	}
	if got := <-result; got != "skip it" {
		t.Fatalf("escalate returned %q", got)
	}
	s.mu.Lock()
	st0 = run.lanes[0].status
	s.mu.Unlock()
	if st0 != laneRunning {
		t.Fatalf("continue should resume lane 0 to running, got %v", st0)
	}
}

// AC3: cooperative cancel arms the lane, which then settles failed(cancelled) at its
// next idle (the sub-agent wraps up first).
func TestLaneCancelSettlesFailed(t *testing.T) {
	s, _ := newTestServer()
	run := startParallelRun(s)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	result := make(chan string, 1)
	go func() {
		result <- s.escalate(escalateReq{
			laneSession: "s0", agentID: "s0", kind: pause.KindIssue,
			message: "blocked", caps: []pause.Cap{pause.CapContinue, pause.CapCancel},
		})
	}()
	id := waitForPause(t, s)
	if _, err := http.PostForm(srv.URL+"/pause/"+id, url.Values{"action": {"cancel"}}); err != nil {
		t.Fatal(err)
	}
	if got := <-result; !strings.Contains(got, "cancelled") {
		t.Fatalf("got %q", got)
	}

	s.mu.Lock()
	cr := run.lanes[0].cancelReason
	s.mu.Unlock()
	if cr == "" {
		t.Fatal("cancel should arm the lane's cancelReason")
	}

	// The sub-agent wraps up and goes idle → the lane settles failed(cancelled).
	s.handleEvent(copilot.Event{Type: copilot.EvIdle, SessionID: "s0"})
	s.mu.Lock()
	st0, detail := run.lanes[0].status, run.lanes[0].detail
	s.mu.Unlock()
	if st0 != laneFailed {
		t.Fatalf("a cancelled lane should settle failed, got %v", st0)
	}
	if !strings.Contains(detail, "cancelled") {
		t.Fatalf("the lane detail should record the cancellation: %q", detail)
	}
}

// An abort force-resolves a pending pause so the blocked escalate goroutine unblocks
// (no leaked goroutine, no stuck run).
func TestAbortCancelsPendingPause(t *testing.T) {
	s, _ := newTestServer()
	startParallelRun(s)

	result := make(chan string, 1)
	go func() {
		result <- s.escalate(escalateReq{
			laneSession: "s0", agentID: "s0", message: "blocked",
			caps: []pause.Cap{pause.CapContinue, pause.CapCancel},
		})
	}()
	waitForPause(t, s)

	s.abortRun(t.Context())

	select {
	case got := <-result:
		if !strings.Contains(got, "cancelled") {
			t.Fatalf("abort should cancel the pause, got %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("abort did not unblock the parked escalate goroutine")
	}
	if p := s.pauses.Pending(); len(p) != 0 {
		t.Fatalf("abort should clear pending pauses, still have %d", len(p))
	}
}

// Aborting a run while a lane is parked settles the lane failed with the abort
// detail — the resuming escalate goroutine must not overwrite "⏹ aborted" with a
// "cancelled" note (the abort-vs-escalate display race).
func TestAbortParkedLaneKeepsAbortedDetail(t *testing.T) {
	s, _ := newTestServer()
	run := startParallelRun(s) // lanes s0, s1 running with session ids
	result := make(chan string, 1)
	go func() {
		result <- s.escalate(escalateReq{laneSession: "s0", agentID: "s0",
			message: "blocked", caps: []pause.Cap{pause.CapContinue, pause.CapCancel}})
	}()
	waitForPause(t, s)

	s.abortRun(t.Context())
	if got := <-result; !strings.Contains(got, "cancelled") {
		t.Fatalf("escalate should return a wrap-up directive after abort, got %q", got)
	}
	s.mu.Lock()
	st0, detail0 := run.lanes[0].status, run.lanes[0].detail
	s.mu.Unlock()
	if st0 != laneFailed {
		t.Fatalf("an aborted parked lane should settle failed, got %v", st0)
	}
	if !strings.Contains(detail0, "aborted") {
		t.Fatalf("the abort detail must survive the resuming goroutine, got %q", detail0)
	}
}

// Escalating into a run that already settled returns a cancel directive immediately
// without registering a pause — a pause nothing would resolve would block the handler
// goroutine forever and render a phantom form.
func TestEscalateOnDoneRunReturnsCancelWithoutBlocking(t *testing.T) {
	s, _ := newTestServer()
	run := startParallelRun(s)
	s.mu.Lock()
	run.done = true // the run settled in the window before escalate registered
	s.mu.Unlock()

	got := s.escalate(escalateReq{laneSession: "s0", agentID: "s0", message: "late",
		caps: []pause.Cap{pause.CapContinue, pause.CapCancel}})
	if !strings.Contains(got, "cancelled") {
		t.Fatalf("escalate into a done run should return cancel, got %q", got)
	}
	if p := s.pauses.Pending(); len(p) != 0 {
		t.Fatalf("escalate into a done run must not register a pause, have %d", len(p))
	}
}

// AC4: a pause form renders only the capability-flagged buttons, posts to /pause/{id},
// and escapes the model-originated message.
func TestRenderPauseFormCapabilityFlagged(t *testing.T) {
	out := renderPauseForm(pause.Pause{ID: "pause-1", Kind: pause.KindIssue,
		Message: "need a hand", Caps: []pause.Cap{pause.CapContinue, pause.CapCancel}})
	for _, want := range []string{`hx-post="/pause/pause-1"`, "need a hand", "continue", "cancel", `name="payload"`} {
		if !strings.Contains(out, want) {
			t.Errorf("pause form missing %q: %s", want, out)
		}
	}

	// A cancel-only pause shows no continue button and no reply field.
	out = renderPauseForm(pause.Pause{ID: "pause-2", Caps: []pause.Cap{pause.CapCancel}})
	if strings.Contains(out, ">continue<") {
		t.Errorf("cancel-only pause should not render a continue button: %s", out)
	}
	if strings.Contains(out, `name="payload"`) {
		t.Errorf("cancel-only pause should not render a reply field: %s", out)
	}

	// Model-originated text is escaped (ADR-0001).
	out = renderPauseForm(pause.Pause{ID: "pause-3", Message: "<script>x</script>",
		Caps: []pause.Cap{pause.CapContinue}})
	if strings.Contains(out, "<script>") {
		t.Errorf("pause message must be escaped: %s", out)
	}
}

// Idempotent render: replaying the same pending set yields identical HTML.
func TestRenderPausesIdempotent(t *testing.T) {
	pending := []pause.Pause{
		{ID: "pause-1", Message: "a", Caps: []pause.Cap{pause.CapContinue, pause.CapCancel}},
		{ID: "pause-2", Message: "b", Caps: []pause.Cap{pause.CapCancel}},
	}
	first := renderPauses(pending)
	second := renderPauses(pending)
	if first != second {
		t.Errorf("renderPauses should be deterministic for the same input:\n%q\n%q", first, second)
	}
}

// The chat page renders a pending pause inline in the #pauses region.
func TestChatPartialRendersPendingPause(t *testing.T) {
	s, _ := newTestServer()
	s.pauses.Register(pause.Pause{AgentID: "sub-1", Kind: pause.KindInput,
		Message: "which branch?", Caps: []pause.Cap{pause.CapContinue, pause.CapCancel}})
	out := s.chatPartial()
	if !strings.Contains(out, `id="pauses"`) {
		t.Errorf("chat page missing the #pauses region: %s", firstN(out, 200))
	}
	if !strings.Contains(out, "which branch?") {
		t.Error("the pending pause should render in the chat page")
	}
}

func firstN(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
