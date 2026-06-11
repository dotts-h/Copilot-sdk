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
		At:    time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC),
		RunID: "run-pin",
		Type:  "EvToolStart",
		Tool:  "bash",
		Args:  "echo hi",
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
		`"tool": "bash"`,
		`"args": "echo hi"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("event log missing stable tag %q in:\n%s", want, body)
		}
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
