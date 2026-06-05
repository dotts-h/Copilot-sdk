package web

import (
	"io"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/dotts-h/copilot-sdk/internal/config"
	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// recordSpend folds a single priced usage into both the account-wide and the
// per-session meter (as the real EvUsage reducer does) so tests can drive the
// budget past a threshold on either gauge.
func recordSpend(s *Server, inputTokens int64) {
	u := telemetry.Usage{Model: "gpt-5", InputTokens: inputTokens}
	s.meter.Record(u)
	s.sessionMeter.Record(u)
}

func TestStatlineTurnsAmberOverSoftThreshold(t *testing.T) {
	s, _ := newTestServer()
	s.spec.Model = "gpt-5"
	s.allowance = 100 // 100 credits = $1.00
	s.warnFraction = 0.8

	// Under the threshold: no warn class on the cost item.
	recordSpend(s, 400_000) // $0.50 = 50 cr → 50% of allowance
	if got := renderStatline(s); strings.Contains(got, "stat-cost warn") {
		t.Errorf("under the soft threshold the cost item must not be amber: %q", got)
	}

	// Cross 80%: the cost item turns amber.
	recordSpend(s, 300_000) // +$0.375 = 87.5 cr total → 87.5%
	if got := renderStatline(s); !strings.Contains(got, "stat-cost warn") {
		t.Errorf("over the soft threshold the cost item must be amber: %q", got)
	}
}

func TestCostFooterWarnsOverSoftThreshold(t *testing.T) {
	s, _ := newTestServer()
	s.allowance = 100
	s.warnFraction = 0.8
	recordSpend(s, 700_000) // $0.875 = 87.5 cr → 87.5%

	got := renderCostFooter(s.meter, s.budget())
	if !strings.Contains(got, "warn") {
		t.Errorf("cost footer should carry a warn marker over the soft threshold: %q", got)
	}
}

// gateOn primes a session so the next turn's projected spend breaches the hard
// cap, then returns the test HTTP server.
func gateOn(t *testing.T) (*Server, *copilot.MockClient, *httptest.Server) {
	t.Helper()
	s, mock := newTestServer()
	s.spec.Model = "gpt-5"
	// A disk-backed config so the "raise cap" persistence path is genuinely exercised.
	cfg := config.Default(t.TempDir())
	cfg.Telemetry.HardCapCredits = 1 // 1 credit ceiling
	*s.config = *cfg
	s.hardCap = 1
	// 100k gpt-5 input tokens project ~12.5 cr for the next turn, well over the cap.
	s.ctxCurrent = 100_000
	s.ctxLimit = 128_000
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return s, mock, srv
}

func TestHardCapPausesOverBudgetTurn(t *testing.T) {
	s, mock, srv := gateOn(t)

	resp, err := srv.Client().PostForm(srv.URL+"/send", url.Values{"prompt": {"expensive turn"}})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if len(mock.Sent) != 0 {
		t.Fatalf("an over-cap turn must not be dispatched until confirmed: %v", mock.Sent)
	}
	if !strings.Contains(string(body), "budget") || !strings.Contains(string(body), "exceed") {
		t.Errorf("send should return the inline budget gate: %q", body)
	}
	// The prompt's user bubble still shows immediately (type-ahead UX).
	if !strings.Contains(string(body), "expensive turn") {
		t.Errorf("the user bubble should appear even when gated: %q", body)
	}
	if s.gate == nil {
		t.Fatal("a pending gate should be recorded on the session")
	}
}

func TestHardCapProceedDispatchesHeldTurn(t *testing.T) {
	s, mock, srv := gateOn(t)
	_, _ = srv.Client().PostForm(srv.URL+"/send", url.Values{"prompt": {"expensive turn"}})

	resp, err := srv.Client().PostForm(srv.URL+"/budget/proceed", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if len(mock.Sent) != 1 || mock.Sent[0] != "expensive turn" {
		t.Fatalf("proceed should dispatch the held prompt: %v", mock.Sent)
	}
	if s.gate != nil {
		t.Error("the gate should be cleared after a decision")
	}
	// The cap is untouched, so a later over-cap turn gates again.
	if s.hardCap != 1 {
		t.Errorf("proceed must not change the cap, got %v", s.hardCap)
	}
}

func TestHardCapRaiseLiftsCapAndDispatches(t *testing.T) {
	s, mock, srv := gateOn(t)
	_, _ = srv.Client().PostForm(srv.URL+"/send", url.Values{"prompt": {"expensive turn"}})

	resp, err := srv.Client().PostForm(srv.URL+"/budget/raise", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if len(mock.Sent) != 1 || mock.Sent[0] != "expensive turn" {
		t.Fatalf("raise should dispatch the held prompt: %v", mock.Sent)
	}
	if s.hardCap != 0 {
		t.Errorf("raise should disable the cap, got %v", s.hardCap)
	}
	if s.config.Telemetry.HardCapCredits != 0 {
		t.Errorf("raise should persist the lifted cap to config, got %v", s.config.Telemetry.HardCapCredits)
	}
}

func TestHardCapCancelDropsTurn(t *testing.T) {
	s, mock, srv := gateOn(t)
	_, _ = srv.Client().PostForm(srv.URL+"/send", url.Values{"prompt": {"expensive turn"}})

	resp, err := srv.Client().PostForm(srv.URL+"/budget/cancel", nil)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if len(mock.Sent) != 0 {
		t.Fatalf("cancel must not dispatch: %v", mock.Sent)
	}
	if s.gate != nil {
		t.Error("cancel should clear the gate")
	}
	if s.busy {
		t.Error("cancel should clear the busy state so the composer is free")
	}
	if !strings.Contains(string(body), "cancelled") {
		t.Errorf("cancel should note the dropped turn in the timeline: %q", body)
	}
}

func TestHardCapGatesQueuedTurnOnDrain(t *testing.T) {
	s, mock := newTestServer()
	s.spec.Model = "gpt-5"
	s.hardCap = 1
	s.ctxCurrent = 100_000
	// Simulate a prompt typed ahead while a turn was in flight: busy, with a
	// backing session and the prompt waiting in the queue.
	s.busy = true
	s.sessionID = "sess-1"
	s.queue = []string{"queued over-budget turn"}

	frags := s.handleEvent(copilot.Event{Type: copilot.EvIdle})

	if len(mock.Sent) != 0 {
		t.Fatalf("a queued over-cap turn must gate, not dispatch on drain: %v", mock.Sent)
	}
	if s.gate == nil || s.gate.prompt != "queued over-budget turn" {
		t.Fatalf("draining an over-cap queued turn should record a gate: %+v", s.gate)
	}
	if !fragContains(frags, "budget", "exceed your budget cap") {
		t.Errorf("drain should surface the budget gate over SSE: %+v", frags)
	}
	if len(s.queue) != 0 {
		t.Errorf("the gated prompt should be removed from the queue: %v", s.queue)
	}
}

func TestUnderCapDispatchesNormally(t *testing.T) {
	s, mock := newTestServer()
	s.spec.Model = "gpt-5"
	s.hardCap = 1000 // generous ceiling
	s.ctxCurrent = 100_000
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	_, err := srv.Client().PostForm(srv.URL+"/send", url.Values{"prompt": {"cheap turn"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(mock.Sent) != 1 {
		t.Fatalf("an under-cap turn should dispatch without a gate: %v", mock.Sent)
	}
	if s.gate != nil {
		t.Error("no gate should be recorded for an under-cap turn")
	}
}
