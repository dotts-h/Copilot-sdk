// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package telemetry

import "sort"

// This file adds the per-model TOKEN breakdown computed from the persisted
// ledger — the all-time cousin of the live Meter.ByModel() gauge. The Telemetry
// page's per-model table read the in-process Meter, which is empty until turns
// replay through it (epic 0050, finding 2): it showed 0/0/0 next to real spend
// while the ledger already carried the token counts per turn. ModelBreakdown
// aggregates those counts straight from SpendStore.Records() so the table is
// populated from history (restart-surviving), labelled "all-time" against the
// live meter's "this session". A pure reader over a record slice, a cousin of
// the *Shares readers (history.go) — same determinism, no IO, no schema change.
// — issue 0058.

// ModelBreakdown aggregates one model's token counts and cost over the persisted
// ledger. The ledger stores only each turn's total USD (not the input/cached/
// output USD split — see SpendRecord), so the dollar/credit figures are turn
// totals; the in/cached/output split is exact in TOKENS, which is what the
// per-model table shows. Mirrors the live Meter's ModelTotals accessor shape
// (USD/Credits) so the Telemetry render swaps source cleanly.
type ModelBreakdown struct {
	Model        string
	InputTokens  int64
	CachedTokens int64
	OutputTokens int64
	// CacheWriteTokens is priced into USDTotal (ADR-0034); ReasoningTokens is a
	// display-only subset of OutputTokens (already priced at the output rate).
	CacheWriteTokens int64
	ReasoningTokens  int64
	USDTotal         float64 // sum of each turn's total USD (no per-component split in the ledger)
	Turns            int64
}

// USD returns the model's total dollar cost over the ledger.
func (b ModelBreakdown) USD() float64 { return b.USDTotal }

// Credits expresses the model's cost in GitHub AI Credits (1 cr = $0.01).
func (b ModelBreakdown) Credits() float64 { return b.USDTotal / USDPerCredit }

// ModelBreakdowns totals per-model token counts and cost across the persisted
// ledger, sorted by credits descending (ties broken by model name for
// determinism) so the biggest spenders surface first — matching Meter.ByModel().
// Every record is included (an empty model key reads back as the "" group, like
// ModelShares includeEmpty). Pure: same records → same result, no IO.
func ModelBreakdowns(records []SpendRecord) []ModelBreakdown {
	by := map[string]*ModelBreakdown{}
	for _, r := range records {
		b := by[r.Model]
		if b == nil {
			b = &ModelBreakdown{Model: r.Model}
			by[r.Model] = b
		}
		b.InputTokens += r.InputTokens
		b.CachedTokens += r.CachedTokens
		b.OutputTokens += r.OutputTokens
		b.CacheWriteTokens += r.CacheWriteTokens
		b.ReasoningTokens += r.ReasoningTokens
		b.USDTotal += r.USD
		b.Turns++
	}
	out := make([]ModelBreakdown, 0, len(by))
	for _, b := range by {
		out = append(out, *b)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].USDTotal != out[j].USDTotal {
			return out[i].USDTotal > out[j].USDTotal
		}
		return out[i].Model < out[j].Model
	})
	return out
}
