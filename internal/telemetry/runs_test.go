package telemetry

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func sampleRun() RunRecord {
	return RunRecord{
		ID:         "run-1",
		WorkflowID: "build-and-harden",
		Name:       "Build & harden",
		Mode:       "sequential",
		StartedAt:  time.Date(2026, 6, 6, 10, 0, 0, 0, time.UTC),
		FinishedAt: time.Date(2026, 6, 6, 10, 2, 0, 0, time.UTC),
		Outcome:    "finished",
		Lanes: []RunLane{
			{Index: 0, AgentID: "builder", Status: "done", Credits: 2.6},
			{Index: 1, AgentID: "sdet", Status: "done", Credits: 2.2},
		},
	}
}

func TestLoadRunStoreMissingIsEmpty(t *testing.T) {
	s, err := LoadRunStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if s.Count() != 0 {
		t.Fatalf("fresh run store should be empty, got %d", s.Count())
	}
}

func TestRunStoreAppendPersistsAndReloads(t *testing.T) {
	dir := t.TempDir()
	s, err := LoadRunStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Append(sampleRun()); err != nil {
		t.Fatal(err)
	}
	// Atomic write leaves the canonical file in place (no stray .tmp).
	if _, err := os.Stat(filepath.Join(dir, "runs.json")); err != nil {
		t.Fatalf("runs file not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "runs.json.tmp")); !os.IsNotExist(err) {
		t.Fatalf("temp file should be renamed away, stat err = %v", err)
	}

	// A second store reads the persisted history back (survives "restart").
	reloaded, err := LoadRunStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	runs := reloaded.Records()
	if len(runs) != 1 {
		t.Fatalf("reloaded %d runs, want 1", len(runs))
	}
	got := runs[0]
	if got.ID != "run-1" || got.WorkflowID != "build-and-harden" || got.Mode != "sequential" || got.Outcome != "finished" {
		t.Fatalf("round trip lost run fields: %+v", got)
	}
	if len(got.Lanes) != 2 || got.Lanes[1].AgentID != "sdet" || got.Lanes[1].Credits != 2.2 {
		t.Fatalf("round trip lost lane fields: %+v", got.Lanes)
	}
	if !got.StartedAt.Equal(sampleRun().StartedAt) {
		t.Fatalf("StartedAt not preserved: %v", got.StartedAt)
	}
}

func TestRunStoreStampsFinishedAtWhenZero(t *testing.T) {
	s, _ := LoadRunStore("")
	r := sampleRun()
	r.FinishedAt = time.Time{}
	if err := s.Append(r); err != nil {
		t.Fatal(err)
	}
	if s.Records()[0].FinishedAt.IsZero() {
		t.Fatal("Append should stamp FinishedAt when zero")
	}
}

func TestRunStoreEphemeralNeverWrites(t *testing.T) {
	s, err := LoadRunStore("") // empty dir => in-memory only (demo/tests)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Append(sampleRun()); err != nil {
		t.Fatal(err)
	}
	if s.Count() != 1 {
		t.Fatalf("ephemeral run store should still accumulate, got %d", s.Count())
	}
}

func TestLoadRunStoreRejectsCorruptFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "runs.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRunStore(dir); err == nil {
		t.Fatal("expected error on corrupt run history")
	}
}

func TestLoadRunStoreToleratesNewerSchema(t *testing.T) {
	// Forward-compatible: a file written by a newer minor version (extra fields,
	// higher version) still yields its runs — the array is the stable contract.
	dir := t.TempDir()
	body := `{"version":99,"runs":[{"id":"r","workflow":"w","name":"W","mode":"parallel","outcome":"finished","future":"ignored","lanes":[]}]}`
	if err := os.WriteFile(filepath.Join(dir, "runs.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := LoadRunStore(dir)
	if err != nil {
		t.Fatalf("newer schema should be readable: %v", err)
	}
	if s.Count() != 1 {
		t.Fatalf("want 1 run, got %d", s.Count())
	}
}

func TestRunRecordCarriesSkippedLane(t *testing.T) {
	// A branched run's per-lane outcomes — including a skipped lane that incurred no
	// cost — must round-trip (the reason a sibling run store exists, not spend tags).
	dir := t.TempDir()
	s, _ := LoadRunStore(dir)
	r := sampleRun()
	r.Lanes = append(r.Lanes, RunLane{Index: 2, AgentID: "fixer", Status: "skipped"})
	if err := s.Append(r); err != nil {
		t.Fatal(err)
	}
	reloaded, _ := LoadRunStore(dir)
	lanes := reloaded.Records()[0].Lanes
	if len(lanes) != 3 {
		t.Fatalf("want 3 lanes, got %d", len(lanes))
	}
	skip := lanes[2]
	if skip.Status != "skipped" {
		t.Fatalf("skipped lane status = %q, want skipped", skip.Status)
	}
	if skip.Credits != 0 {
		t.Fatalf("a skipped lane incurs no cost, got %v credits", skip.Credits)
	}
}
