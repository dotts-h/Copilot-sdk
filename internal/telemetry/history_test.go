package telemetry

import (
	"bytes"
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func day(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestSpendRecordCreditsAndDay(t *testing.T) {
	r := SpendRecord{USD: 0.25, At: day("2026-06-05T09:30:00Z")}
	approx(t, r.Credits(), 25) // $0.25 / $0.01 per credit
	if got := r.Day(); got != "2026-06-05" {
		t.Fatalf("Day = %q, want 2026-06-05", got)
	}
	// Day is computed in UTC, so a late-evening local time rolls correctly.
	r2 := SpendRecord{At: day("2026-06-05T23:59:00Z")}
	if got := r2.Day(); got != "2026-06-05" {
		t.Fatalf("Day = %q, want 2026-06-05", got)
	}
}

func TestLoadSpendStoreMissingIsEmpty(t *testing.T) {
	s, err := LoadSpendStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if s.Count() != 0 {
		t.Fatalf("fresh store should be empty, got %d", s.Count())
	}
}

func TestSpendStoreAppendPersistsAndReloads(t *testing.T) {
	dir := t.TempDir()
	s, err := LoadSpendStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	rec := SpendRecord{SessionID: "sess-1", Model: "gpt-5", InputTokens: 1200, OutputTokens: 340, USD: 0.5}
	if err := s.Append(rec); err != nil {
		t.Fatal(err)
	}
	// Atomic write leaves the canonical file in place (no stray .tmp).
	if _, err := os.Stat(filepath.Join(dir, "spend.json")); err != nil {
		t.Fatalf("spend file not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "spend.json.tmp")); !os.IsNotExist(err) {
		t.Fatalf("temp file should be renamed away, stat err = %v", err)
	}

	// A second store reads the persisted history back (survives "restart").
	reloaded, err := LoadSpendStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	recs := reloaded.Records()
	if len(recs) != 1 {
		t.Fatalf("reloaded %d records, want 1", len(recs))
	}
	if recs[0].SessionID != "sess-1" || recs[0].Model != "gpt-5" || recs[0].USD != 0.5 {
		t.Fatalf("round trip lost fields: %+v", recs[0])
	}
	if recs[0].At.IsZero() {
		t.Fatal("Append should stamp At when zero")
	}
}

func TestSpendStoreEphemeralNeverWrites(t *testing.T) {
	s, err := LoadSpendStore("") // empty dir => in-memory only (demo/tests)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Append(SpendRecord{Model: "gpt-5", USD: 1}); err != nil {
		t.Fatal(err)
	}
	if s.Count() != 1 {
		t.Fatalf("ephemeral store should still accumulate, got %d", s.Count())
	}
}

func TestLoadSpendStoreRejectsCorruptFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "spend.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSpendStore(dir); err == nil {
		t.Fatal("expected error on corrupt spend history")
	}
}

func TestLoadSpendStoreToleratesNewerSchema(t *testing.T) {
	// Forward-compatible: a file written by a newer minor version (extra fields,
	// higher version) still yields its records — the array is the stable contract.
	dir := t.TempDir()
	body := `{"version":99,"records":[{"at":"2026-06-05T10:00:00Z","model":"gpt-5","usd":0.1,"future":"ignored"}]}`
	if err := os.WriteFile(filepath.Join(dir, "spend.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSpendStore(dir)
	if err != nil {
		t.Fatalf("newer schema should be readable: %v", err)
	}
	if s.Count() != 1 {
		t.Fatalf("want 1 record, got %d", s.Count())
	}
}

func TestDailyTotals(t *testing.T) {
	recs := []SpendRecord{
		{At: day("2026-06-05T10:00:00Z"), USD: 0.10},
		{At: day("2026-06-04T10:00:00Z"), USD: 0.20},
		{At: day("2026-06-05T18:00:00Z"), USD: 0.30},
	}
	got := DailyTotals(recs)
	if len(got) != 2 {
		t.Fatalf("want 2 days, got %d: %+v", len(got), got)
	}
	// Sorted ascending by day.
	if got[0].Day != "2026-06-04" || got[1].Day != "2026-06-05" {
		t.Fatalf("days not sorted ascending: %+v", got)
	}
	approx(t, got[0].USD, 0.20)
	approx(t, got[1].USD, 0.40) // 0.10 + 0.30
	if got[1].Count != 2 {
		t.Fatalf("day count = %d, want 2", got[1].Count)
	}
	approx(t, got[1].Credits, 40) // $0.40 => 40 credits
}

func TestModelShares(t *testing.T) {
	recs := []SpendRecord{
		{Model: "gpt-5", USD: 0.75},
		{Model: "claude-sonnet-4-6", USD: 0.25},
		{Model: "gpt-5", USD: 0.0},
	}
	got := ModelShares(recs)
	if len(got) != 2 {
		t.Fatalf("want 2 models, got %d", len(got))
	}
	// Sorted by spend desc, so gpt-5 leads.
	if got[0].Model != "gpt-5" {
		t.Fatalf("biggest spender should lead: %+v", got)
	}
	approx(t, got[0].Fraction, 0.75)
	approx(t, got[1].Fraction, 0.25)
	var sum float64
	for _, m := range got {
		sum += m.Fraction
	}
	approx(t, sum, 1.0)
}

func TestModelSharesEmpty(t *testing.T) {
	if got := ModelShares(nil); len(got) != 0 {
		t.Fatalf("nil records should yield no shares, got %+v", got)
	}
}

func TestSpendRecordRoundTripsAttributionTags(t *testing.T) {
	// A v2 record carries the additive agent/workflow/lane tags through a
	// persist + reload cycle, and a non-first lane index round-trips too.
	dir := t.TempDir()
	s, err := LoadSpendStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	rec := SpendRecord{
		Model: "gpt-5", USD: 0.5,
		AgentID: "builder", WorkflowID: "ship", LaneIndex: 2,
	}
	if err := s.Append(rec); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadSpendStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := reloaded.Records()
	if len(got) != 1 {
		t.Fatalf("reloaded %d records, want 1", len(got))
	}
	if got[0].AgentID != "builder" || got[0].WorkflowID != "ship" || got[0].LaneIndex != 2 {
		t.Fatalf("attribution tags lost on round trip: %+v", got[0])
	}
}

func TestSpendStoreReadsV1RecordWithoutTags(t *testing.T) {
	// Backward-readable: a v1 file (no agent/workflow/lane keys, version 1) still
	// loads — older records read back with empty/zero attribution tags.
	dir := t.TempDir()
	body := `{"version":1,"records":[{"at":"2026-06-05T10:00:00Z","session":"s1","model":"gpt-5","in":100,"out":50,"usd":0.1}]}`
	if err := os.WriteFile(filepath.Join(dir, "spend.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSpendStore(dir)
	if err != nil {
		t.Fatalf("v1 file should still be readable: %v", err)
	}
	recs := s.Records()
	if len(recs) != 1 {
		t.Fatalf("want 1 record, got %d", len(recs))
	}
	if recs[0].AgentID != "" || recs[0].WorkflowID != "" || recs[0].LaneIndex != 0 {
		t.Fatalf("a v1 record should read back with empty attribution: %+v", recs[0])
	}
	if recs[0].Model != "gpt-5" || recs[0].USD != 0.1 {
		t.Fatalf("v1 fields lost: %+v", recs[0])
	}
}

func TestAgentShares(t *testing.T) {
	recs := []SpendRecord{
		{AgentID: "builder", USD: 0.60},
		{AgentID: "sdet", USD: 0.20},
		{AgentID: "builder", USD: 0.20},
		{AgentID: "", USD: 0.0}, // empty-agent (built-in chat) bucket, zero spend
	}
	got := AgentShares(recs)
	// builder, sdet, and the empty-agent bucket — every turn has an agent.
	if len(got) != 3 {
		t.Fatalf("want 3 agent buckets, got %d: %+v", len(got), got)
	}
	// Sorted by spend desc → builder leads.
	if got[0].AgentID != "builder" {
		t.Fatalf("biggest spender should lead: %+v", got)
	}
	approx(t, got[0].Fraction, 0.80) // 0.80 of 1.00
	approx(t, got[1].Fraction, 0.20)
	approx(t, got[0].Credits, 80)
}

func TestAgentSharesDeterministicTieBreak(t *testing.T) {
	// Equal spend ties break by agent id, so the order is deterministic.
	recs := []SpendRecord{{AgentID: "zeta", USD: 0.5}, {AgentID: "alpha", USD: 0.5}}
	got := AgentShares(recs)
	if len(got) != 2 || got[0].AgentID != "alpha" || got[1].AgentID != "zeta" {
		t.Fatalf("ties should break by agent id: %+v", got)
	}
}

func TestWorkflowSharesExcludeNonWorkflowSpend(t *testing.T) {
	recs := []SpendRecord{
		{WorkflowID: "ship", USD: 0.30},
		{WorkflowID: "review", USD: 0.10},
		{WorkflowID: "ship", USD: 0.10},
		{WorkflowID: "", USD: 5.00}, // plain chat spend — excluded from the workflow view
	}
	got := WorkflowShares(recs)
	if len(got) != 2 {
		t.Fatalf("non-workflow spend must be excluded: got %d buckets %+v", len(got), got)
	}
	if got[0].WorkflowID != "ship" {
		t.Fatalf("biggest workflow should lead: %+v", got)
	}
	// Fractions are relative to workflow-attributed spend (0.40 + 0.10 = 0.50), not
	// the 5.50 grand total — so ship is 0.40/0.50 = 0.80.
	approx(t, got[0].Fraction, 0.80)
	approx(t, got[1].Fraction, 0.20)
}

func TestWorkflowSharesEmpty(t *testing.T) {
	// Records that are all plain chat (no workflow) yield no workflow shares.
	if got := WorkflowShares([]SpendRecord{{USD: 1}}); len(got) != 0 {
		t.Fatalf("all-chat records should yield no workflow shares, got %+v", got)
	}
}

func TestWriteCSV(t *testing.T) {
	recs := []SpendRecord{
		{At: day("2026-06-05T10:00:00Z"), SessionID: "s1", Model: "gpt-5", InputTokens: 1200, CachedTokens: 200, OutputTokens: 340, USD: 0.5, AIU: 0.012},
	}
	var buf bytes.Buffer
	if err := WriteCSV(&buf, recs); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatalf("CSV is not parseable: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want header + 1 row, got %d", len(rows))
	}
	wantHeader := []string{"at", "session", "model", "input", "cached", "output", "usd", "credits", "aiu"}
	for i, h := range wantHeader {
		if rows[0][i] != h {
			t.Fatalf("header[%d] = %q, want %q", i, rows[0][i], h)
		}
	}
	if rows[1][2] != "gpt-5" || rows[1][3] != "1200" {
		t.Fatalf("data row wrong: %+v", rows[1])
	}
	if rows[1][7] != "50" { // credits = 0.5 / 0.01
		t.Fatalf("credits column = %q, want 50", rows[1][7])
	}
}

func TestWriteCSVAppendsAttributionColumns(t *testing.T) {
	// The attribution columns are appended at the end so the pre-v2 column order is
	// unchanged (backward-compatible header — CONTRACTS §3).
	recs := []SpendRecord{
		{At: day("2026-06-05T10:00:00Z"), Model: "gpt-5", USD: 0.5, AgentID: "builder", WorkflowID: "ship", LaneIndex: 1},
	}
	var buf bytes.Buffer
	if err := WriteCSV(&buf, recs); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatalf("CSV is not parseable: %v", err)
	}
	want := []string{"at", "session", "model", "input", "cached", "output", "usd", "credits", "aiu", "agent", "workflow", "lane"}
	if len(rows[0]) != len(want) {
		t.Fatalf("header width = %d, want %d: %+v", len(rows[0]), len(want), rows[0])
	}
	for i, h := range want {
		if rows[0][i] != h {
			t.Fatalf("header[%d] = %q, want %q", i, rows[0][i], h)
		}
	}
	if rows[1][9] != "builder" || rows[1][10] != "ship" || rows[1][11] != "1" {
		t.Fatalf("attribution columns wrong: agent=%q workflow=%q lane=%q", rows[1][9], rows[1][10], rows[1][11])
	}
}

func TestMonthToDate(t *testing.T) {
	now := day("2026-06-15T12:00:00Z")
	tests := []struct {
		name    string
		records []SpendRecord
		wantUSD float64
	}{
		{
			name:    "empty ledger sums to zero",
			records: nil,
			wantUSD: 0,
		},
		{
			name: "single month sums every turn",
			records: []SpendRecord{
				{At: day("2026-06-01T00:00:00Z"), USD: 0.10},
				{At: day("2026-06-09T10:00:00Z"), USD: 0.20},
				{At: day("2026-06-15T11:59:59Z"), USD: 0.30},
			},
			wantUSD: 0.60,
		},
		{
			name: "month boundary is inclusive at both edges (UTC)",
			records: []SpendRecord{
				{At: day("2026-06-01T00:00:00Z"), USD: 0.10}, // first instant of the month
				{At: day("2026-06-30T23:59:59Z"), USD: 0.25}, // last instant of the month
			},
			wantUSD: 0.35,
		},
		{
			name: "prior and following months are excluded",
			records: []SpendRecord{
				{At: day("2026-05-31T23:59:59Z"), USD: 1.00}, // prior month
				{At: day("2026-06-10T10:00:00Z"), USD: 0.40}, // in month
				{At: day("2026-07-01T00:00:00Z"), USD: 2.00}, // following month
				{At: day("2025-06-15T10:00:00Z"), USD: 5.00}, // same month, prior year
			},
			wantUSD: 0.40,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := MonthToDate(tc.records, now)
			approx(t, got.USD(), tc.wantUSD)
			approx(t, got.Credits(), tc.wantUSD/USDPerCredit)
		})
	}
}
