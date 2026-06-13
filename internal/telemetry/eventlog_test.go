// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package telemetry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRunEventLogRoundTrip verifies that a RunEventLog records events and reads
// them back correctly (round-trip + atomic-write + ephemeral/empty-dir shapes).
func TestRunEventLogRoundTrip(t *testing.T) {
	dir := t.TempDir()
	log, err := LoadRunEventLog(dir, "run-abc")
	if err != nil {
		t.Fatal(err)
	}

	ev1 := RunEvent{
		At:    time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC),
		RunID: "run-abc",
		Type:  "EvMessage",
		Text:  "hello world",
	}
	ev2 := RunEvent{
		At:    time.Date(2026, 6, 11, 10, 0, 1, 0, time.UTC),
		RunID: "run-abc",
		Type:  "EvToolStart",
		Tool:  "bash",
	}
	if err := log.Append(ev1); err != nil {
		t.Fatal(err)
	}
	if err := log.Append(ev2); err != nil {
		t.Fatal(err)
	}

	// Atomic write leaves the canonical file in place (no stray .tmp on success).
	path := RunEventLogPath(dir, "run-abc")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("event log file not written: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temp file should be renamed away, stat err = %v", err)
	}

	// A second store reads the persisted records back (survives "restart").
	reloaded, err := LoadRunEventLog(dir, "run-abc")
	if err != nil {
		t.Fatal(err)
	}
	got := reloaded.Records()
	if len(got) != 2 {
		t.Fatalf("round trip: got %d events, want 2", len(got))
	}
	if got[0].Type != "EvMessage" || got[0].Text != "hello world" {
		t.Fatalf("event[0] mismatch: %+v", got[0])
	}
	if got[1].Type != "EvToolStart" || got[1].Tool != "bash" {
		t.Fatalf("event[1] mismatch: %+v", got[1])
	}
}

// TestRunEventLogEphemeralNeverWrites verifies an empty-dir log is in-memory only.
func TestRunEventLogEphemeralNeverWrites(t *testing.T) {
	log, err := LoadRunEventLog("", "run-xyz") // empty dir => in-memory only
	if err != nil {
		t.Fatal(err)
	}
	if err := log.Append(RunEvent{Type: "EvMessage", Text: "hi"}); err != nil {
		t.Fatal(err)
	}
	if log.Count() != 1 {
		t.Fatalf("ephemeral store should still accumulate, got %d", log.Count())
	}
}

// TestRunEventLogMissingIsEmpty verifies a fresh (absent file) log loads empty.
func TestRunEventLogMissingIsEmpty(t *testing.T) {
	log, err := LoadRunEventLog(t.TempDir(), "run-new")
	if err != nil {
		t.Fatal(err)
	}
	if log.Count() != 0 {
		t.Fatalf("fresh log should be empty, got %d", log.Count())
	}
}

// TestRunEventLogRejectsCorruptFile verifies a corrupt file returns an error.
func TestRunEventLogRejectsCorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := RunEventLogPath(dir, "run-bad")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRunEventLog(dir, "run-bad"); err == nil {
		t.Fatal("expected error on corrupt event log file")
	}
}

// TestRunEventLogToleratesNewerSchema verifies forward-compatibility.
func TestRunEventLogToleratesNewerSchema(t *testing.T) {
	dir := t.TempDir()
	path := RunEventLogPath(dir, "run-future")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"version":99,"events":[{"at":"2026-06-11T10:00:00Z","runId":"run-future","type":"EvMessage","text":"hi","future":"ignored"}],"extra":true}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	log, err := LoadRunEventLog(dir, "run-future")
	if err != nil {
		t.Fatalf("newer schema should be readable: %v", err)
	}
	if log.Count() != 1 || log.Records()[0].Text != "hi" {
		t.Fatalf("want the one newer-schema record, got %+v", log.Records())
	}
}

// TestRunEventLogOnDiskTagsAreStable pins the on-disk JSON tags as a stable
// contract — the event log writes `"version"`, `"events"`, and the per-event
// fields, byte-identically to what this test pins. Any drift breaks replay.
func TestRunEventLogOnDiskTagsAreStable(t *testing.T) {
	dir := t.TempDir()
	log, err := LoadRunEventLog(dir, "run-pin")
	if err != nil {
		t.Fatal(err)
	}
	ev := RunEvent{
		At:        time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC),
		RunID:     "run-pin",
		Type:      "EvToolStart",
		LaneIndex: 2, // non-zero to pin the laneIndex tag explicitly
		Tool:      "bash",
		Args:      "echo hi",
	}
	if err := log.Append(ev); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(RunEventLogPath(dir, "run-pin"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, want := range []string{
		`"version": 1`,
		`"events": [`,
		`"runId": "run-pin"`,
		`"type": "EvToolStart"`,
		`"laneIndex": 2`,
		`"tool": "bash"`,
		`"args": "echo hi"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("event log missing stable tag %q in:\n%s", want, body)
		}
	}
}

// TestRunEventLogPricedUsageRoundTrips proves the additive O2 pricing fields
// (tokensIn/tokensOut/credits) round-trip on disk and pin their stable JSON tags,
// while a record written WITHOUT them reads back zero — the additive-only rule
// (ADR-0048): a pre-O2 log is still readable and prices to nothing. — issue 0092.
func TestRunEventLogPricedUsageRoundTrips(t *testing.T) {
	dir := t.TempDir()
	log, err := LoadRunEventLog(dir, "run-priced")
	if err != nil {
		t.Fatal(err)
	}
	priced := RunEvent{
		At:        time.Date(2026, 6, 12, 9, 0, 0, 0, time.UTC),
		RunID:     "run-priced",
		Type:      "EvUsage",
		LaneIndex: 1,
		TokensIn:  1200,
		TokensOut: 340,
		Credits:   0.55,
	}
	// An unpriced (pre-O2-shaped) event: no token/credit fields set.
	unpriced := RunEvent{Type: "EvMessage", LaneIndex: 0, Text: "hi"}
	if err := log.Append(priced); err != nil {
		t.Fatal(err)
	}
	if err := log.Append(unpriced); err != nil {
		t.Fatal(err)
	}

	// Pin the new on-disk tags so a drift that breaks replay fails loud.
	data, err := os.ReadFile(RunEventLogPath(dir, "run-priced"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, want := range []string{`"tokensIn": 1200`, `"tokensOut": 340`, `"credits": 0.55`} {
		if !strings.Contains(body, want) {
			t.Fatalf("priced usage event missing stable tag %q in:\n%s", want, body)
		}
	}
	// The unpriced event omits the additive fields entirely (omitempty), so an
	// older reader sees no cost-column noise.
	if strings.Count(body, `"credits"`) != 1 {
		t.Fatalf("the unpriced event must omit the credits tag (omitempty):\n%s", body)
	}

	// Reload: the priced event keeps its figures; the unpriced one reads back zero.
	got, err := LoadRunEventLog(dir, "run-priced")
	if err != nil {
		t.Fatal(err)
	}
	recs := got.Records()
	if len(recs) != 2 {
		t.Fatalf("want 2 records, got %d", len(recs))
	}
	if recs[0].TokensIn != 1200 || recs[0].TokensOut != 340 || recs[0].Credits != 0.55 {
		t.Fatalf("priced event lost its figures on read-back: %+v", recs[0])
	}
	if recs[1].TokensIn != 0 || recs[1].TokensOut != 0 || recs[1].Credits != 0 {
		t.Fatalf("unpriced event must read back zero-valued, got %+v", recs[1])
	}
}

// TestRunEventLogPerRunIsolation verifies two runs get separate files and
// separate records — keyed by run id.
func TestRunEventLogPerRunIsolation(t *testing.T) {
	dir := t.TempDir()
	logA, err := LoadRunEventLog(dir, "run-a")
	if err != nil {
		t.Fatal(err)
	}
	logB, err := LoadRunEventLog(dir, "run-b")
	if err != nil {
		t.Fatal(err)
	}
	if err := logA.Append(RunEvent{Type: "EvMessage", Text: "from A"}); err != nil {
		t.Fatal(err)
	}
	if err := logB.Append(RunEvent{Type: "EvMessage", Text: "from B"}); err != nil {
		t.Fatal(err)
	}
	if err := logB.Append(RunEvent{Type: "EvIdle"}); err != nil {
		t.Fatal(err)
	}

	if logA.Count() != 1 {
		t.Fatalf("run-a count = %d, want 1", logA.Count())
	}
	if logB.Count() != 2 {
		t.Fatalf("run-b count = %d, want 2", logB.Count())
	}

	// Files are separate on disk.
	if RunEventLogPath(dir, "run-a") == RunEventLogPath(dir, "run-b") {
		t.Fatal("run-a and run-b must have distinct file paths")
	}
}
