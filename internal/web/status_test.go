package web

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/dotts-h/copilot-sdk/internal/copilot"
)

// ctxFor returns the HTML of the "ctx" fragment emitted for an event, or "".
func ctxFor(s *Server, e copilot.Event) string {
	for _, f := range s.handleEvent(e) {
		if f.Event == "ctx" {
			return f.HTML
		}
	}
	return ""
}

func TestContextWindowFragmentShowsPercent(t *testing.T) {
	s, _ := newTestServer()
	got := ctxFor(s, copilot.Event{
		Type:    copilot.EvContextWindow,
		Context: copilot.ContextInfo{CurrentTokens: 12800, TokenLimit: 128000},
	})
	if !strings.Contains(got, "10%") {
		t.Errorf("context fragment should show fill percent: %q", got)
	}
	if !strings.Contains(got, "12.8k") || !strings.Contains(got, "128.0k") {
		t.Errorf("context fragment should show humanized token counts: %q", got)
	}
}

func TestContextWindowMarksNearFull(t *testing.T) {
	s, _ := newTestServer()
	got := ctxFor(s, copilot.Event{
		Type:    copilot.EvContextWindow,
		Context: copilot.ContextInfo{CurrentTokens: 120000, TokenLimit: 128000},
	})
	if !strings.Contains(got, "warn") {
		t.Errorf("near-full context should carry a warn class: %q", got)
	}
}

func TestStatusShowsElapsedTimerWhileActive(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	// A send starts a turn, so the OOB status carries a live elapsed timer.
	resp, err := srv.Client().PostForm(srv.URL+"/send", url.Values{"prompt": {"hi"}})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	got := statusFor(s, copilot.Event{Type: copilot.EvToolStart, Tool: "shell"})
	if !strings.Contains(got, `class="elapsed"`) || !strings.Contains(got, "data-start=") {
		t.Errorf("active status should carry an elapsed-timer element: %q", got)
	}

	// Idle clears the turn; no timer once inactive.
	if idle := statusFor(s, copilot.Event{Type: copilot.EvIdle}); strings.Contains(idle, "data-start=") {
		t.Errorf("idle status should not carry an elapsed timer: %q", idle)
	}
}

func TestCompactionSurfacesNotesAndCtx(t *testing.T) {
	s, _ := newTestServer()

	frags := s.handleEvent(copilot.Event{Type: copilot.EvCompactionStart})
	if !fragContains(frags, "ctx", "compacting") {
		t.Errorf("compaction start should mark the context meter compacting: %+v", frags)
	}
	if !fragContains(frags, "timeline", "compacting") {
		t.Errorf("compaction start should add a system note: %+v", frags)
	}

	end := s.handleEvent(copilot.Event{Type: copilot.EvCompactionEnd, Text: "compacted context (freed 5000 tokens)"})
	if !fragContains(end, "timeline", "freed 5000 tokens") {
		t.Errorf("compaction end should add the summary note: %+v", end)
	}
	if fragContains(end, "ctx", "compacting") {
		t.Errorf("compaction end should clear the compacting marker: %+v", end)
	}
}

func TestUsageEmitsStatlineWithTokenBreakdown(t *testing.T) {
	s, _ := newTestServer()
	frags := s.handleEvent(copilot.Event{
		Type: copilot.EvUsage,
		Usage: copilot.UsageData{
			Model: "gpt-5", InputTokens: 1000, CachedTokens: 3000,
			CacheWriteTokens: 500, OutputTokens: 250, ReasoningTokens: 80,
		},
	})
	stat := ""
	for _, f := range frags {
		if f.Event == "stat" {
			stat = f.HTML
		}
	}
	if stat == "" {
		t.Fatalf("EvUsage should emit a stat fragment: %+v", frags)
	}
	// cache-read 3000 + cache-write 500 surfaced, plus the cache-hit rate
	// (cached/(input+cached) = 3000/4000 = 75%), reasoning count, and tokens in/out.
	for _, want := range []string{"↑1.0k ↓250", "3.0k/500", "75%", "✻80"} {
		if !strings.Contains(stat, want) {
			t.Errorf("statline missing %q: %s", want, stat)
		}
	}
}

// TestStatlineLabelsReportedVsEstimatedSpend asserts the statusline cost cell makes
// the source of the actual-spend figure explicit (ADR-0033): before the runtime
// reports an AIU the cell carries the price-book estimate flagged "est", and once a
// turn is reported the cell shows the authoritative reported figure (1 AIU = 1 credit)
// flagged as reported — not the estimate.
func TestStatlineLabelsReportedVsEstimatedSpend(t *testing.T) {
	s, _ := newTestServer()

	// A turn with no reported AIU: the cell is the price-book estimate, labelled "est".
	est := statFor(s, copilot.Event{
		Type:  copilot.EvUsage,
		Usage: copilot.UsageData{Model: "gpt-5", InputTokens: 1000, OutputTokens: 250},
	})
	if !strings.Contains(est, "est") {
		t.Errorf("an unreported turn's cost cell should be flagged as an estimate: %q", est)
	}

	// A turn the runtime reports (NanoAIU set): the cell switches to the reported
	// figure — 1.5e9 nano-AIU = 1.5 AIU = 1.50 cr — and is no longer flagged "est".
	rep := statFor(s, copilot.Event{
		Type:  copilot.EvUsage,
		Usage: copilot.UsageData{Model: "gpt-5", InputTokens: 1000, OutputTokens: 250, NanoAIU: 1_500_000_000},
	})
	if !strings.Contains(rep, "1.50 cr") {
		t.Errorf("a reported turn's cost cell should show the reported figure 1.50 cr: %q", rep)
	}
	if strings.Contains(rep, ">est<") || strings.Contains(rep, " est ") {
		t.Errorf("a reported turn's cost cell must not be flagged as an estimate: %q", rep)
	}
}

// statFor returns the HTML of the "stat" (statline) fragment emitted for an
// event, or "".
func statFor(s *Server, e copilot.Event) string {
	for _, f := range s.handleEvent(e) {
		if f.Event == "stat" {
			return f.HTML
		}
	}
	return ""
}

func TestStatlineShowsPreflightEstimateAtCurrentContext(t *testing.T) {
	s, _ := newTestServer()
	s.spec.Model = "gpt-5" // pin the model so the estimate is deterministic

	// Before any context reading, there is nothing to estimate, so the statline
	// carries no pre-flight figure.
	if pre := statFor(s, copilot.Event{Type: copilot.EvToolStart, Tool: "shell"}); strings.Contains(pre, "next turn") {
		t.Errorf("no estimate expected before a context reading: %q", pre)
	}

	// A context-window reading drives the estimate: 100k gpt-5 input tokens at
	// $1.25/Mtok = $0.125 = 12.50 credits.
	got := statFor(s, copilot.Event{
		Type:    copilot.EvContextWindow,
		Context: copilot.ContextInfo{CurrentTokens: 100_000, TokenLimit: 128_000},
	})
	if !strings.Contains(got, "~12.50 cr") {
		t.Errorf("statline should show the pre-flight estimate ~12.50 cr: %q", got)
	}
	if !strings.Contains(got, "next turn") {
		t.Errorf("statline estimate should be labelled for the next turn: %q", got)
	}
}

func TestToolStartCountsTowardStatline(t *testing.T) {
	s, _ := newTestServer()
	frags := s.handleEvent(copilot.Event{Type: copilot.EvToolStart, Tool: "shell"})
	if !fragContains(frags, "stat", "⚙ 1") {
		t.Errorf("a tool start should bump the tools-used count in the statline: %+v", frags)
	}
}

func fragContains(frags []fragment, event, sub string) bool {
	for _, f := range frags {
		if f.Event == event && strings.Contains(f.HTML, sub) {
			return true
		}
	}
	return false
}

func TestHumanTokens(t *testing.T) {
	cases := map[int64]string{
		0:       "0",
		512:     "512",
		1500:    "1.5k",
		128000:  "128.0k",
		2500000: "2.5M",
	}
	for in, want := range cases {
		if got := humanTokens(in); got != want {
			t.Errorf("humanTokens(%d) = %q, want %q", in, got, want)
		}
	}
}
