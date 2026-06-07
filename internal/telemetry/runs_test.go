package telemetry

import (
	"bytes"
	"encoding/csv"
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

func TestRunRecordDuration(t *testing.T) {
	// A finished run reports its wall-clock span.
	if d := sampleRun().Duration(); d != 2*time.Minute {
		t.Fatalf("Duration = %v, want 2m", d)
	}
	// An unfinished record (zero FinishedAt) → 0, not a huge negative span.
	unfinished := sampleRun()
	unfinished.FinishedAt = time.Time{}
	if d := unfinished.Duration(); d != 0 {
		t.Fatalf("unfinished Duration = %v, want 0", d)
	}
	// A zero StartedAt (malformed) → 0.
	noStart := sampleRun()
	noStart.StartedAt = time.Time{}
	if d := noStart.Duration(); d != 0 {
		t.Fatalf("zero-start Duration = %v, want 0", d)
	}
	// A negative span (Finished before Started — clock skew) → 0.
	skewed := sampleRun()
	skewed.FinishedAt = skewed.StartedAt.Add(-time.Minute)
	if d := skewed.Duration(); d != 0 {
		t.Fatalf("negative Duration = %v, want 0", d)
	}
	// A zero-length span (same instant) → 0 (guarded as non-positive).
	instant := sampleRun()
	instant.FinishedAt = instant.StartedAt
	if d := instant.Duration(); d != 0 {
		t.Fatalf("zero-span Duration = %v, want 0", d)
	}
}

func TestRunAggregatesMixedHistory(t *testing.T) {
	base := time.Date(2026, 6, 6, 10, 0, 0, 0, time.UTC)
	// A mixed history: workflow "alpha" runs three times (one a failure), workflow
	// "beta" runs once with a branched/skipped lane that adds zero cost. Order is
	// shuffled to prove the roll-up doesn't depend on input order.
	records := []RunRecord{
		{WorkflowID: "beta", Name: "Beta", StartedAt: base, FinishedAt: base.Add(time.Minute),
			Outcome: "finished", Lanes: []RunLane{
				{Index: 0, Status: "done", Credits: 4},
				{Index: 1, Status: "skipped"}, // a skipped branch adds zero cost
			}},
		{WorkflowID: "alpha", Name: "Alpha", StartedAt: base, FinishedAt: base.Add(2 * time.Minute),
			Outcome: "finished", Lanes: []RunLane{{Index: 0, Status: "done", Credits: 2}}},
		{WorkflowID: "alpha", Name: "Alpha", StartedAt: base, FinishedAt: base.Add(4 * time.Minute),
			Outcome: "failed", Lanes: []RunLane{{Index: 0, Status: "failed", Credits: 1}}},
		{WorkflowID: "alpha", Name: "Alpha", StartedAt: base, FinishedAt: base.Add(6 * time.Minute),
			Outcome: "finished", Lanes: []RunLane{{Index: 0, Status: "done", Credits: 3}}},
	}
	got := RunAggregates(records)
	if len(got) != 2 {
		t.Fatalf("want 2 workflow aggregates, got %d: %+v", len(got), got)
	}
	// Deterministic ordering: alpha (3 runs) before beta (1 run).
	if got[0].WorkflowID != "alpha" || got[1].WorkflowID != "beta" {
		t.Fatalf("ordering = %q,%q; want alpha,beta", got[0].WorkflowID, got[1].WorkflowID)
	}
	a := got[0]
	if a.Runs != 3 || a.Failures != 1 {
		t.Fatalf("alpha runs/failures = %d/%d, want 3/1", a.Runs, a.Failures)
	}
	approx(t, a.TotalCredits, 6) // 2 + 1 + 3
	approx(t, a.AvgCredits, 2)   // 6 / 3
	approx(t, a.FailureRate(), 1.0/3.0)
	if a.TotalDuration != 12*time.Minute { // 2 + 4 + 6
		t.Fatalf("alpha total duration = %v, want 12m", a.TotalDuration)
	}
	if a.AvgDuration != 4*time.Minute { // 12m / 3
		t.Fatalf("alpha avg duration = %v, want 4m", a.AvgDuration)
	}
	b := got[1]
	if b.Runs != 1 || b.Failures != 0 {
		t.Fatalf("beta runs/failures = %d/%d, want 1/0", b.Runs, b.Failures)
	}
	approx(t, b.TotalCredits, 4) // the skipped lane added nothing
	approx(t, b.AvgCredits, 4)
	if b.FailureRate() != 0 {
		t.Fatalf("beta failure rate = %v, want 0", b.FailureRate())
	}
}

func TestRunAggregatesDeterministicOrder(t *testing.T) {
	// Two workflows with the SAME run count must tie-break by workflow id ascending,
	// regardless of input order — a stable, total order over the unique key.
	base := time.Date(2026, 6, 6, 10, 0, 0, 0, time.UTC)
	mk := func(id string) RunRecord {
		return RunRecord{WorkflowID: id, StartedAt: base, FinishedAt: base.Add(time.Minute), Outcome: "finished"}
	}
	for _, order := range [][]RunRecord{
		{mk("zeta"), mk("alpha")},
		{mk("alpha"), mk("zeta")},
	} {
		got := RunAggregates(order)
		if len(got) != 2 || got[0].WorkflowID != "alpha" || got[1].WorkflowID != "zeta" {
			t.Fatalf("ties must sort by id asc; got %+v", got)
		}
	}
}

func TestRunAggregatesEdgeCases(t *testing.T) {
	// Empty history → empty roll-up (no divide-by-zero, no nil-vs-empty surprise).
	if got := RunAggregates(nil); len(got) != 0 {
		t.Fatalf("empty history should aggregate to nothing, got %+v", got)
	}
	base := time.Date(2026, 6, 6, 10, 0, 0, 0, time.UTC)
	// A single all-failed workflow: failure rate 1.0, averages over one run.
	single := []RunRecord{
		{WorkflowID: "w", Name: "W", StartedAt: base, FinishedAt: base.Add(time.Minute),
			Outcome: "failed", Lanes: []RunLane{{Index: 0, Status: "failed", Credits: 5}}},
	}
	got := RunAggregates(single)
	if len(got) != 1 || got[0].Runs != 1 || got[0].Failures != 1 || got[0].FailureRate() != 1 {
		t.Fatalf("single all-failed run wrong: %+v", got)
	}
	approx(t, got[0].AvgCredits, 5)
	// A workflow whose runs all span zero duration → zero total/avg duration, no panic.
	zeroDur := []RunRecord{
		{WorkflowID: "z", StartedAt: base, FinishedAt: base, Outcome: "finished"},
		{WorkflowID: "z", StartedAt: time.Time{}, FinishedAt: base, Outcome: "finished"},
	}
	zg := RunAggregates(zeroDur)
	if len(zg) != 1 || zg[0].TotalDuration != 0 || zg[0].AvgDuration != 0 {
		t.Fatalf("zero-duration workflow should roll up to 0 duration: %+v", zg)
	}
	if zg[0].Runs != 2 {
		t.Fatalf("zero-duration runs still count: got %d, want 2", zg[0].Runs)
	}
}

func TestRunAggregatesLatestNameWins(t *testing.T) {
	// A renamed workflow reads under its current (latest, chronological-append) name.
	base := time.Date(2026, 6, 6, 10, 0, 0, 0, time.UTC)
	records := []RunRecord{
		{WorkflowID: "w", Name: "Old name", StartedAt: base, FinishedAt: base.Add(time.Minute), Outcome: "finished"},
		{WorkflowID: "w", Name: "New name", StartedAt: base, FinishedAt: base.Add(time.Minute), Outcome: "finished"},
	}
	got := RunAggregates(records)
	if len(got) != 1 || got[0].Name != "New name" {
		t.Fatalf("latest name should win, got %+v", got)
	}
}

func TestRunAggregatesLastRun(t *testing.T) {
	base := time.Date(2026, 6, 6, 10, 0, 0, 0, time.UTC)

	// Across a multi-run workflow, the LATEST run by StartedAt wins — its outcome and
	// start surface as the last-run signal, regardless of input order. Here the most
	// recent run (base+20m) FAILED, even though earlier runs finished, and the records
	// are shuffled to prove the pick doesn't depend on order.
	multi := []RunRecord{
		{WorkflowID: "w", Name: "W", StartedAt: base.Add(10 * time.Minute), FinishedAt: base.Add(11 * time.Minute), Outcome: "finished"},
		{WorkflowID: "w", Name: "W", StartedAt: base.Add(20 * time.Minute), FinishedAt: base.Add(21 * time.Minute), Outcome: "failed"},
		{WorkflowID: "w", Name: "W", StartedAt: base, FinishedAt: base.Add(time.Minute), Outcome: "finished"},
	}
	got := RunAggregates(multi)
	if len(got) != 1 {
		t.Fatalf("want one aggregate, got %d", len(got))
	}
	if got[0].LastOutcome != "failed" {
		t.Fatalf("last outcome = %q, want failed (the most recent run)", got[0].LastOutcome)
	}
	if !got[0].LastStartedAt.Equal(base.Add(20 * time.Minute)) {
		t.Fatalf("last started = %v, want %v", got[0].LastStartedAt, base.Add(20*time.Minute))
	}

	// A single run: the last-run signal is that one run.
	single := RunAggregates([]RunRecord{
		{WorkflowID: "s", Name: "S", StartedAt: base, FinishedAt: base.Add(time.Minute), Outcome: "finished"},
	})
	if single[0].LastOutcome != "finished" || !single[0].LastStartedAt.Equal(base) {
		t.Fatalf("single run last-signal wrong: %+v", single[0])
	}

	// An UNFINISHED latest run (latest StartedAt, zero FinishedAt) is still the last
	// run — the pick is by StartedAt, and an in-flight/malformed finish must not
	// demote it to an older, finished run.
	unfinished := RunAggregates([]RunRecord{
		{WorkflowID: "u", Name: "U", StartedAt: base, FinishedAt: base.Add(time.Minute), Outcome: "finished"},
		{WorkflowID: "u", Name: "U", StartedAt: base.Add(5 * time.Minute), FinishedAt: time.Time{}, Outcome: "failed"},
	})
	if unfinished[0].LastOutcome != "failed" || !unfinished[0].LastStartedAt.Equal(base.Add(5*time.Minute)) {
		t.Fatalf("unfinished latest should still be the last run: %+v", unfinished[0])
	}

	// Tie-break: two runs share a StartedAt — the one with the LATER FinishedAt wins,
	// deterministically (the Workflows demo hits same-instant runs). Order-independent.
	for _, order := range [][]RunRecord{
		{
			{WorkflowID: "t", StartedAt: base, FinishedAt: base.Add(time.Minute), Outcome: "finished"},
			{WorkflowID: "t", StartedAt: base, FinishedAt: base.Add(3 * time.Minute), Outcome: "failed"},
		},
		{
			{WorkflowID: "t", StartedAt: base, FinishedAt: base.Add(3 * time.Minute), Outcome: "failed"},
			{WorkflowID: "t", StartedAt: base, FinishedAt: base.Add(time.Minute), Outcome: "finished"},
		},
	} {
		tie := RunAggregates(order)
		if tie[0].LastOutcome != "failed" {
			t.Fatalf("StartedAt tie should break to the later FinishedAt (failed); got %q for order %+v", tie[0].LastOutcome, order)
		}
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

func TestWriteRunsCSV(t *testing.T) {
	// The runs export flattens to one row per lane (run-level columns repeated), so a
	// branched run's SKIPPED lane — which leaves no spend record and so can't appear in
	// the spend CSV — is first-class in the orchestration export. Columns are fixed.
	r := sampleRun()
	r.Lanes = append(r.Lanes, RunLane{Index: 2, AgentID: "fixer", Status: "skipped"})
	var buf bytes.Buffer
	if err := WriteRunsCSV(&buf, []RunRecord{r}); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatalf("CSV is not parseable: %v", err)
	}
	// header + one row per lane (3 lanes).
	if len(rows) != 4 {
		t.Fatalf("want header + 3 lane rows, got %d", len(rows))
	}
	want := []string{"run", "workflow", "name", "mode", "startedAt", "finishedAt", "durationSeconds", "outcome", "lane", "agent", "status", "credits"}
	if len(rows[0]) != len(want) {
		t.Fatalf("header width = %d, want %d: %+v", len(rows[0]), len(want), rows[0])
	}
	for i, h := range want {
		if rows[0][i] != h {
			t.Fatalf("header[%d] = %q, want %q", i, rows[0][i], h)
		}
	}
	// First lane row carries the run-level columns and lane 0.
	first := rows[1]
	if first[0] != "run-1" || first[1] != "build-and-harden" || first[3] != "sequential" || first[7] != "finished" {
		t.Fatalf("run-level columns wrong: %+v", first)
	}
	if first[6] != "120" { // duration 2m = 120s, repeated on every lane row
		t.Fatalf("durationSeconds = %q, want 120", first[6])
	}
	if first[8] != "0" || first[9] != "builder" || first[10] != "done" || first[11] != "2.6" { // RunLane.Credits is already in credits
		t.Fatalf("lane-0 columns wrong: %+v", first)
	}
	// The skipped lane is exported with zero credits — the orchestration-unique grain.
	skip := rows[3]
	if skip[8] != "2" || skip[9] != "fixer" || skip[10] != "skipped" || skip[11] != "0" {
		t.Fatalf("skipped lane row wrong: %+v", skip)
	}
	// Run-level columns repeat verbatim on the skipped lane row.
	if skip[0] != "run-1" || skip[6] != "120" {
		t.Fatalf("run-level columns not repeated on lane row: %+v", skip)
	}
}

func TestWriteRunsCSVHeaderOnlyWhenEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteRunsCSV(&buf, nil); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatalf("CSV is not parseable: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("an empty history exports the header only, got %d rows", len(rows))
	}
}
