package telemetry

import (
	"math"
	"sort"
)

// This file joins the ledger's two cost figures — the price-book estimate and
// GitHub's reported AIU (ADR-0033) — per model, so the estimate-vs-reported gap
// is visible instead of silent (issue 0060). It is the cost cousin of the
// ledger⋈runs reconciliation (reconcile.go): same pure cross-figure join, same
// |delta|-first ordering, but within ONE store — each SpendRecord carries both
// sides. Only REPORTED turns are compared: an unreported turn has no
// authoritative figure to drift from, so adding its estimate to either side
// would manufacture divergence; it is counted (UnreportedTurns) but never
// joined. A pure reader over a record slice — no IO, no schema change.

// ModelDrift reconciles one model's price-book estimate against GitHub's
// reported cost over the turns the runtime reported. Delta is EstimateCredits −
// ReportedCredits — zero when the price book matches GitHub's bill, negative
// when it under-estimates real spend (the epic-0050 audit finding), positive
// when it over-estimates. UnreportedTurns counts the model's turns that carried
// no reported figure (excluded from the join) so a surface can show how much of
// the ledger the comparison actually covers.
type ModelDrift struct {
	Model           string
	EstimateCredits float64 // price-book estimate over the model's reported turns only
	ReportedCredits float64 // GitHub's reported AIU over the same turns (1 AIU = 1 cr)
	Delta           float64 // EstimateCredits − ReportedCredits (0 = the price book agrees)
	ReportedTurns   int     // turns joined (both figures present)
	UnreportedTurns int     // turns with no reported figure — counted, never compared
}

// Drifted reports whether the delta's magnitude clears epsilon — a real
// divergence rather than floating-point noise from summing two independent
// measurements. The caller owns epsilon (the web layer ties it to the display
// width, like the sibling reconcile rows).
func (d ModelDrift) Drifted(epsilon float64) bool { return math.Abs(d.Delta) >= epsilon }

// ModelDrifts joins the price-book estimate to the reported cost per model over
// the reported turns (HasReported) of the given records. A model with no
// reported turn has nothing to compare and yields no row. Ledger estimates sum
// USD then convert once (mirroring shareBy / WorkflowReconcile) so the figure
// matches the shares it sits beside. Sorted by absolute delta DESCENDING — the
// biggest mis-pricing reads first — with ties broken by estimate descending,
// then model ascending: a total order over the unique model key for full
// determinism regardless of input order. Empty input → empty slice. Pure: same
// records → same result.
func ModelDrifts(records []SpendRecord) []ModelDrift {
	by := map[string]*ModelDrift{}
	for _, r := range records {
		d := by[r.Model]
		if d == nil {
			d = &ModelDrift{Model: r.Model}
			by[r.Model] = d
		}
		if !r.HasReported() {
			d.UnreportedTurns++
			continue
		}
		d.EstimateCredits += r.USD / USDPerCredit
		d.ReportedCredits += ReportedCredits(r.AIU)
		d.ReportedTurns++
	}
	out := make([]ModelDrift, 0, len(by))
	for _, d := range by {
		if d.ReportedTurns == 0 {
			continue // no reported figure to drift from
		}
		d.Delta = d.EstimateCredits - d.ReportedCredits
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool {
		di, dj := math.Abs(out[i].Delta), math.Abs(out[j].Delta)
		if di != dj {
			return di > dj
		}
		if out[i].EstimateCredits != out[j].EstimateCredits {
			return out[i].EstimateCredits > out[j].EstimateCredits
		}
		return out[i].Model < out[j].Model
	})
	return out
}
