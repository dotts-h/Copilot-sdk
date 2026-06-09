package telemetry

import "testing"

func TestModelBreakdownsAggregatesTokensFromLedger(t *testing.T) {
	// The ledger already carries per-turn token counts; the breakdown sums them
	// per model (the live meter showed 0/0/0 because it was never replayed —
	// issue 0058). Two gpt-5 turns + one claude turn.
	recs := []SpendRecord{
		{Model: "gpt-5", InputTokens: 1000, CachedTokens: 200, OutputTokens: 300, USD: 0.50},
		{Model: "gpt-5", InputTokens: 500, CachedTokens: 100, OutputTokens: 150, USD: 0.25},
		{Model: "claude-sonnet-4-6", InputTokens: 800, CachedTokens: 0, OutputTokens: 400, USD: 0.10},
	}
	got := ModelBreakdowns(recs)
	if len(got) != 2 {
		t.Fatalf("want 2 models, got %d: %+v", len(got), got)
	}
	// Sorted by USD desc → gpt-5 leads (0.75 > 0.10).
	g := got[0]
	if g.Model != "gpt-5" {
		t.Fatalf("biggest spender should lead, got %q", g.Model)
	}
	if g.InputTokens != 1500 || g.CachedTokens != 300 || g.OutputTokens != 450 {
		t.Fatalf("gpt-5 token totals wrong: in=%d cached=%d out=%d", g.InputTokens, g.CachedTokens, g.OutputTokens)
	}
	if g.Turns != 2 {
		t.Fatalf("gpt-5 should fold 2 turns, got %d", g.Turns)
	}
	approx(t, g.USD(), 0.75)
	approx(t, g.Credits(), 75) // 0.75 / 0.01
	// The non-zero token counts are the whole point — the table must not read 0/0/0.
	if g.InputTokens == 0 && g.CachedTokens == 0 && g.OutputTokens == 0 {
		t.Fatal("breakdown must surface the ledger's token counts, not 0/0/0")
	}
}

func TestModelBreakdownsEmpty(t *testing.T) {
	if got := ModelBreakdowns(nil); len(got) != 0 {
		t.Fatalf("nil records should yield no breakdown, got %+v", got)
	}
}

func TestModelBreakdownsDeterministicTieBreak(t *testing.T) {
	// Equal USD totals tie-break by model name ascending, so the order is stable.
	recs := []SpendRecord{
		{Model: "zeta", InputTokens: 10, USD: 0.10},
		{Model: "alpha", InputTokens: 20, USD: 0.10},
	}
	got := ModelBreakdowns(recs)
	if len(got) != 2 || got[0].Model != "alpha" || got[1].Model != "zeta" {
		t.Fatalf("equal-spend models should sort by name asc, got %+v", got)
	}
}
