package web

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/config"
	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// day parses an RFC3339 timestamp for fixed-date test records.
func day(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

// newSpendServer builds a session wired with an ephemeral spend ledger so the
// persistence path is exercised without touching disk.
func newSpendServer(t *testing.T) (*Server, *telemetry.SpendStore) {
	t.Helper()
	store, err := telemetry.LoadSpendStore("") // ephemeral
	if err != nil {
		t.Fatal(err)
	}
	hub := New(Options{
		Client: copilot.NewMockClient(),
		Forge:  &ctxforge.Forge{},
		Config: &config.Config{DefaultModel: "gpt-5"},
		Meter:  telemetry.NewMeter(telemetry.DefaultPriceBook()),
		Spend:  store,
		Logger: log.New(io.Discard, "", 0),
	})
	return hub.newSession("spend-test"), store
}

func TestUsagePersistsSpendRecord(t *testing.T) {
	s, store := newSpendServer(t)
	s.sessionID = "sess-xyz"
	s.handleEvent(copilot.Event{Type: copilot.EvUsage, Usage: copilot.UsageData{
		Model: "gpt-5", InputTokens: 1200, CachedTokens: 200, OutputTokens: 340, NanoAIU: 1.2e7,
	}})
	recs := store.Records()
	if len(recs) != 1 {
		t.Fatalf("expected 1 persisted record, got %d", len(recs))
	}
	r := recs[0]
	if r.SessionID != "sess-xyz" || r.Model != "gpt-5" || r.InputTokens != 1200 {
		t.Fatalf("record fields wrong: %+v", r)
	}
	if r.USD <= 0 {
		t.Fatalf("record should carry a priced USD cost, got %v", r.USD)
	}
	// NanoAIU (1.2e7 nano) folds into AIU at 1e-9.
	if r.AIU <= 0 {
		t.Fatalf("record should carry the reported AIU, got %v", r.AIU)
	}
}

func TestUsageTagsActiveAgent(t *testing.T) {
	s, store := newSpendServer(t)
	s.agentID = "builder" // the active agent persona
	s.handleEvent(copilot.Event{Type: copilot.EvUsage, Usage: copilot.UsageData{
		Model: "gpt-5", InputTokens: 1000, OutputTokens: 200,
	}})
	rec := store.Records()[0]
	if rec.AgentID != "builder" {
		t.Fatalf("chat turn should be tagged with the active agent, got %q", rec.AgentID)
	}
	// A plain chat turn carries no workflow attribution.
	if rec.WorkflowID != "" || rec.LaneIndex != 0 {
		t.Fatalf("chat turn must not carry workflow attribution: %+v", rec)
	}
}

func TestWorkflowUsageTagsWorkflowAndLane(t *testing.T) {
	s, store := newSpendServer(t)
	// A parallel run whose lanes have distinct backing session ids, so the EvUsage
	// routes to lane 1 by SessionID (run id "w", lane agent "b", index 1).
	run := twoStepRun(ctxforge.WorkflowParallel)
	s.mu.Lock()
	s.run = run
	s.busy = true
	run.start()
	run.lanes[0].sessionID = "s0"
	run.lanes[1].sessionID = "s1"
	s.mu.Unlock()

	s.handleEvent(copilot.Event{Type: copilot.EvUsage, SessionID: "s1", Usage: copilot.UsageData{
		Model: "gpt-5", InputTokens: 1000, OutputTokens: 200,
	}})
	rec := store.Records()[0]
	if rec.WorkflowID != "w" || rec.LaneIndex != 1 || rec.AgentID != "b" {
		t.Fatalf("workflow turn should be tagged with workflow id + lane index + lane agent: %+v", rec)
	}
}

func TestTelemetryPageShowsAttributionBreakdown(t *testing.T) {
	s, store := newSpendServer(t)
	s.forge.Agents = []ctxforge.Agent{{ID: "builder", Name: "Builder", Model: "gpt-5"}}
	s.forge.Workflows = []ctxforge.Workflow{{ID: "ship", Name: "Ship", Mode: ctxforge.WorkflowSequential}}
	for _, r := range []telemetry.SpendRecord{
		{At: day("2026-06-04T10:00:00Z"), Model: "gpt-5", USD: 0.20, AgentID: "builder"},
		{At: day("2026-06-05T10:00:00Z"), Model: "gpt-5", USD: 0.30, AgentID: "builder", WorkflowID: "ship", LaneIndex: 0},
		{At: day("2026-06-05T11:00:00Z"), Model: "gpt-5", USD: 0.10}, // plain chat, empty agent
	} {
		if err := store.Append(r); err != nil {
			t.Fatal(err)
		}
	}
	html := s.telemetryPartial()
	for _, want := range []string{"Cost by agent", "Builder", "chat (built-in)", "Cost by workflow", "Ship"} {
		if !strings.Contains(html, want) {
			t.Fatalf("telemetry page missing %q\n%s", want, html)
		}
	}
}

func TestNewSessionSeedsActiveAgentFromConfig(t *testing.T) {
	// A new session inherits the launch-time active agent, so its first chat turn
	// attributes spend correctly even before any /agent switch.
	hub := New(Options{
		Client: copilot.NewMockClient(),
		Forge:  &ctxforge.Forge{},
		Config: &config.Config{DefaultModel: "gpt-5", DefaultAgent: "builder"},
		Meter:  telemetry.NewMeter(telemetry.DefaultPriceBook()),
		Logger: log.New(io.Discard, "", 0),
	})
	if got := hub.newSession("seed").agentID; got != "builder" {
		t.Fatalf("session should seed the active agent from config, got %q", got)
	}
}

func TestUsageWithoutLedgerDoesNotPanic(t *testing.T) {
	s, _ := newTestServer() // no Spend wired (s.spend == nil)
	s.handleEvent(copilot.Event{Type: copilot.EvUsage, Usage: copilot.UsageData{
		Model: "gpt-5", InputTokens: 100, OutputTokens: 50,
	}})
	// Reaching here without a panic is the assertion.
}

func TestTelemetryPageShowsTrendFromLedger(t *testing.T) {
	s, store := newSpendServer(t)
	// Two days, two models, so both trend lists have content.
	for _, r := range []telemetry.SpendRecord{
		{At: day("2026-06-04T10:00:00Z"), Model: "gpt-5", USD: 0.20},
		{At: day("2026-06-05T10:00:00Z"), Model: "claude-sonnet-4-6", USD: 0.30},
	} {
		if err := store.Append(r); err != nil {
			t.Fatal(err)
		}
	}
	html := s.telemetryPartial()
	for _, want := range []string{"Spend history", "Spend over time", "Per-model share", "2026-06-04", "2026-06-05", "Export CSV"} {
		if !strings.Contains(html, want) {
			t.Fatalf("telemetry page missing %q\n%s", want, html)
		}
	}
}

func TestTelemetryTrendWindowsAndScalesToVisibleMax(t *testing.T) {
	s, store := newSpendServer(t)
	// 20 days of history: the all-time biggest spender is day 0 (off-screen,
	// outside the most-recent-14 window); inside the window the biggest is the
	// last day. Bars must scale to the in-window max so it fills to 100%.
	base := day("2026-05-01T10:00:00Z")
	if err := store.Append(telemetry.SpendRecord{At: base, Model: "gpt-5", USD: 99.0}); err != nil {
		t.Fatal(err)
	}
	for i := 1; i < 20; i++ {
		usd := 0.10 * float64(i) // grows toward the recent end
		if err := store.Append(telemetry.SpendRecord{At: base.AddDate(0, 0, i), Model: "gpt-5", USD: usd}); err != nil {
			t.Fatal(err)
		}
	}
	days, _, has := s.spendTrend()
	if !has {
		t.Fatal("expected history")
	}
	if len(days) != 14 {
		t.Fatalf("trend should window to 14 days, got %d", len(days))
	}
	// The off-screen $99 day must not appear; the busiest in-window day fills.
	for _, d := range days {
		if d["Day"] == "2026-05-01" {
			t.Fatal("the off-screen all-time-max day leaked into the window")
		}
	}
	if w := days[len(days)-1]["Width"]; w != "100.0%" {
		t.Fatalf("busiest in-window day should fill the bar, got width %v", w)
	}
}

func TestTelemetryPageEmptyHistoryNote(t *testing.T) {
	s, _ := newSpendServer(t) // ledger wired but empty
	html := s.telemetryPartial()
	if !strings.Contains(html, "no persisted history yet") {
		t.Fatalf("empty ledger should show the no-history note:\n%s", html)
	}
}

func TestSpendExportReturnsCSV(t *testing.T) {
	s, store := newSpendServer(t)
	if err := store.Append(telemetry.SpendRecord{At: day("2026-06-05T10:00:00Z"), Model: "gpt-5", InputTokens: 1200, USD: 0.5}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/telemetry/export.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Fatalf("Content-Type = %q, want text/csv", ct)
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
		t.Fatalf("Content-Disposition = %q, want attachment", cd)
	}
	body, _ := io.ReadAll(resp.Body)
	out := string(body)
	if !strings.Contains(out, "at,session,model") || !strings.Contains(out, "gpt-5") {
		t.Fatalf("CSV body missing header or data:\n%s", out)
	}
}
