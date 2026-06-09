package telemetry

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// ADR-0034: the ledger gains additive cache-write + reasoning token fields so the
// all-time per-model breakdown shows them from history. The fields are
// backward-readable (older records without them read back 0, like the v2 tags).

func TestSpendRecordCacheWriteReasoningJSONBackwardReadable(t *testing.T) {
	// A new record round-trips both counts.
	rec := SpendRecord{Model: "gpt-5", InputTokens: 100, CacheWriteTokens: 80, ReasoningTokens: 40, USD: 0.5}
	b, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	var back SpendRecord
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.CacheWriteTokens != 80 || back.ReasoningTokens != 40 {
		t.Fatalf("round-trip lost the counts: cw=%d reasoning=%d", back.CacheWriteTokens, back.ReasoningTokens)
	}
	// A pre-0059 record (no cw/reasoning keys) reads back zero, not an error.
	var old SpendRecord
	if err := json.Unmarshal([]byte(`{"model":"gpt-5","in":100,"out":50,"usd":0.5}`), &old); err != nil {
		t.Fatalf("legacy record must still parse: %v", err)
	}
	if old.CacheWriteTokens != 0 || old.ReasoningTokens != 0 {
		t.Fatalf("legacy record should read back zero extras, got cw=%d reasoning=%d", old.CacheWriteTokens, old.ReasoningTokens)
	}
}

func TestModelBreakdownsAggregatesCacheWriteAndReasoning(t *testing.T) {
	recs := []SpendRecord{
		{Model: "gpt-5", InputTokens: 1000, OutputTokens: 300, CacheWriteTokens: 500, ReasoningTokens: 120, USD: 0.50},
		{Model: "gpt-5", InputTokens: 500, OutputTokens: 150, CacheWriteTokens: 200, ReasoningTokens: 80, USD: 0.25},
	}
	got := ModelBreakdowns(recs)
	if len(got) != 1 {
		t.Fatalf("want 1 model, got %d", len(got))
	}
	if got[0].CacheWriteTokens != 700 {
		t.Errorf("cache-write should sum to 700, got %d", got[0].CacheWriteTokens)
	}
	if got[0].ReasoningTokens != 200 {
		t.Errorf("reasoning should sum to 200, got %d", got[0].ReasoningTokens)
	}
}

func TestWriteCSVIncludesCacheWriteAndReasoning(t *testing.T) {
	var buf bytes.Buffer
	recs := []SpendRecord{{Model: "gpt-5", InputTokens: 100, CacheWriteTokens: 80, ReasoningTokens: 40, USD: 0.5}}
	if err := WriteCSV(&buf, recs); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	header := strings.SplitN(out, "\n", 2)[0]
	// New columns are appended at the end, so the legacy column positions are
	// unchanged (CONTRACTS §3 backward-compatible header bump).
	for _, col := range []string{"cacheWrite", "reasoning"} {
		if !strings.Contains(header, col) {
			t.Errorf("CSV header missing %q column: %q", col, header)
		}
	}
	if !strings.Contains(out, "80") || !strings.Contains(out, "40") {
		t.Errorf("CSV body missing cache-write/reasoning counts: %q", out)
	}
}
