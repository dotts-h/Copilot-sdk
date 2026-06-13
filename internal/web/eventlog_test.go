// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package web

import (
	"io"
	"log"
	"os"
	"testing"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/config"
	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// newTestServerWithEventLog builds a Hub+Server with the per-run event log
// enabled (EventLogDir = dir). All other deps are the same as newTestServer.
func newTestServerWithEventLog(dir string) (*Server, *copilot.MockClient) {
	mock := copilot.NewMockClient()
	hub := New(Options{
		Client:      mock,
		Forge:       &ctxforge.Forge{},
		Config:      &config.Config{DefaultModel: "gpt-5"},
		Meter:       telemetry.NewMeter(telemetry.DefaultPriceBook()),
		Logger:      log.New(io.Discard, "", 0),
		EventLogDir: dir,
	})
	return hub.newSession("test"), mock
}

// TestEventLogDisabledLeavesRunBehaviourIdentical proves that when EventLogDir
// is empty (disabled), the appendRunEvent call is a no-op and the existing
// run/spend recording is unaffected — byte-identical to the no-log path.
func TestEventLogDisabledLeavesRunBehaviourIdentical(t *testing.T) {
	// Build a server with no event log dir (disabled).
	s, _ := newTestServer()
	if s.eventLogDir != "" {
		t.Fatal("newTestServer should have no eventLogDir")
	}

	// Simulate a run being active.
	run := twoStepRun(ctxforge.WorkflowSequential)
	run.runID = "run-disabled"
	run.started = time.Now()
	run.start()

	s.mu.Lock()
	s.run = run
	s.busy = true
	s.mu.Unlock()

	// appendRunEvent with eventLogDir="" is a no-op: no file written, no panic.
	e := copilot.Event{Type: copilot.EvMessage, Text: "hi"}
	s.appendRunEvent(e)

	// No file was created.
	if _, err := os.Stat(telemetry.RunEventLogPath("", "run-disabled")); !os.IsNotExist(err) {
		t.Fatalf("empty dir should never create a file; stat err = %v", err)
	}

	// runEventLog is still nil (no lazy creation).
	s.mu.Lock()
	evLog := s.runEventLog
	s.mu.Unlock()
	if evLog != nil {
		t.Fatal("runEventLog must remain nil when eventLogDir is empty")
	}
}

// TestEventLogAppendsRunEvents proves that with EventLogDir set, events flowing
// through appendRunEvent while a run is active are written to the per-run log.
func TestEventLogAppendsRunEvents(t *testing.T) {
	dir := t.TempDir()
	s, _ := newTestServerWithEventLog(dir)

	// Set up an active run.
	run := twoStepRun(ctxforge.WorkflowSequential)
	run.runID = "run-logged"
	run.started = time.Now()
	run.start()

	s.mu.Lock()
	s.run = run
	s.busy = true
	s.mu.Unlock()

	// Append a few events (synchronous via goroutine — wait for them).
	events := []copilot.Event{
		{Type: copilot.EvMessage, Text: "step output"},
		{Type: copilot.EvToolStart, Tool: "bash", ToolCall: &copilot.ToolCall{Args: "echo ok"}},
		{Type: copilot.EvIdle},
	}
	for _, e := range events {
		s.appendRunEvent(e)
	}

	// Give the goroutines time to complete the disk IO.
	waitForEventLog(t, dir, "run-logged", 3)

	// Reload and verify — 3 events are present (order may vary because each
	// append runs in a separate goroutine; we check the SET of types).
	evlog, err := telemetry.LoadRunEventLog(dir, "run-logged")
	if err != nil {
		t.Fatalf("LoadRunEventLog: %v", err)
	}
	got := evlog.Records()
	if len(got) != 3 {
		t.Fatalf("want 3 records, got %d: %+v", len(got), got)
	}
	// All expected types must appear at least once.
	byType := map[string]telemetry.RunEvent{}
	for _, r := range got {
		byType[r.Type] = r
	}
	msg, ok := byType["EvMessage"]
	if !ok || msg.Text != "step output" {
		t.Errorf("EvMessage not logged or wrong text: %+v", byType)
	}
	tool, ok := byType["EvToolStart"]
	if !ok || tool.Tool != "bash" || tool.Args != "echo ok" {
		t.Errorf("EvToolStart not logged or wrong fields: %+v", byType)
	}
	if _, ok := byType["EvIdle"]; !ok {
		t.Errorf("EvIdle not logged: %+v", byType)
	}
}

// TestEventLogDoesNotLogWhenNoRunActive verifies that appendRunEvent is a no-op
// when no run is active, even with eventLogDir set.
func TestEventLogDoesNotLogWhenNoRunActive(t *testing.T) {
	dir := t.TempDir()
	s, _ := newTestServerWithEventLog(dir)
	// No run active (s.run is nil).
	s.appendRunEvent(copilot.Event{Type: copilot.EvMessage, Text: "should not log"})

	// Nothing written.
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if !entry.IsDir() {
			t.Fatalf("no files should be written without an active run, found: %s", entry.Name())
		}
	}
}

// TestEventLogDoesNotLogSubagentEvents verifies that sub-agent tagged events
// are NOT appended to the run log (they route to the registry only, ADR-0040/0041).
func TestEventLogDoesNotLogSubagentEvents(t *testing.T) {
	dir := t.TempDir()
	s, _ := newTestServerWithEventLog(dir)

	run := twoStepRun(ctxforge.WorkflowSequential)
	run.runID = "run-sub"
	run.started = time.Now()
	run.start()
	s.mu.Lock()
	s.run = run
	s.busy = true
	s.mu.Unlock()

	// A sub-agent-tagged event — must be skipped.
	s.appendRunEvent(copilot.Event{
		AgentID: "sub-agent-instance-1",
		Type:    copilot.EvMessage,
		Text:    "sub-agent turn",
	})

	// Nothing written.
	time.Sleep(20 * time.Millisecond) // brief wait; no goroutine should fire
	logPath := telemetry.RunEventLogPath(dir, "run-sub")
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		data, _ := os.ReadFile(logPath)
		t.Fatalf("sub-agent events must not be logged; file exists:\n%s", data)
	}
}

// TestNormalizeRunEventPricesUsage proves the EvUsage case stamps the turn's token
// counts and the price-book estimate credits — the SAME figure recordUsage adds to
// the run's lane (run_adapter.go: l.credits += cost.Credits()), NOT the reported AIU,
// so the summed log credits reconcile against RunRecord.Credits on one basis (issue
// 0092). A reported NanoAIU must NOT change the stamped credits. Non-usage events
// carry no pricing fields.
func TestNormalizeRunEventPricesUsage(t *testing.T) {
	pb := telemetry.DefaultPriceBook()

	usage := copilot.UsageData{Model: "gpt-5", InputTokens: 1000, OutputTokens: 500}
	want := telemetry.Price(pb, telemetry.Usage{
		Model: "gpt-5", InputTokens: 1000, OutputTokens: 500,
	}).Credits()
	if want == 0 {
		t.Fatal("test precondition: gpt-5 must price to a non-zero estimate")
	}

	ev := normalizeRunEvent(copilot.Event{Type: copilot.EvUsage, Usage: usage}, "run-1", 1, pb)
	if ev.Type != "EvUsage" {
		t.Fatalf("type = %q, want EvUsage", ev.Type)
	}
	if ev.TokensIn != 1000 || ev.TokensOut != 500 {
		t.Fatalf("token counts not stamped: %+v", ev)
	}
	if ev.Credits != want {
		t.Fatalf("usage credits = %v, want the price-book estimate %v", ev.Credits, want)
	}

	// A reported authoritative cost (NanoAIU) must NOT shift the logged credits off
	// the estimate basis — else the inspector's cross-check would falsely amber every
	// reported run (the estimate-vs-reported gap is the ModelDrift table's job).
	reported := copilot.UsageData{Model: "gpt-5", InputTokens: 1000, OutputTokens: 500, NanoAIU: 9_000_000_000}
	evR := normalizeRunEvent(copilot.Event{Type: copilot.EvUsage, Usage: reported}, "run-1", 1, pb)
	if evR.Credits != want {
		t.Fatalf("a reported NanoAIU must not change the stamped estimate credits: got %v, want %v", evR.Credits, want)
	}

	// A non-usage event carries no pricing fields.
	msg := normalizeRunEvent(copilot.Event{Type: copilot.EvMessage, Text: "hi"}, "run-1", 0, pb)
	if msg.TokensIn != 0 || msg.TokensOut != 0 || msg.Credits != 0 {
		t.Fatalf("non-usage event must carry no pricing fields, got %+v", msg)
	}
}

// waitForEventLog polls until the event log for runID has at least wantCount
// records (with a short timeout), so tests don't have fragile fixed sleeps.
func waitForEventLog(t *testing.T, dir, runID string, wantCount int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		evlog, err := telemetry.LoadRunEventLog(dir, runID)
		if err == nil && evlog.Count() >= wantCount {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d events in run log %q", wantCount, runID)
}
