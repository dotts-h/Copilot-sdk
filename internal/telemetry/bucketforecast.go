package telemetry

import (
	"sort"
	"time"
)

// This file converges cost prediction (ADR-0019, the account-wide Forecast slope)
// with cost attribution (ADR-0018, the per-agent/workflow spend tags): a bucketed
// burn-rate projection. Where AgentShares/WorkflowShares show each tag's HISTORICAL
// share, BucketForecasts shows each tag's TRAJECTORY — "at this pace, the `review`
// workflow burns ~X cr/day." It is a pure reader over the existing records (no
// schema change, no IO), and it reuses Forecast UNCHANGED per bucket so the three
// ADR-0019 slope gotchas (the elapsed-observed-days denominator clamped to ledger
// age, the ⌈DaysToCap⌉ match, the single `now`) stay single-sourced — F3.
//
// Per-bucket framing: a bucket has no own allowance, so the account-wide Forecast
// is run per bucket only to compute the bucket's OWN slope (DailyRate / UsedCredits
// over its own daily series). The view surfaces that rate plus a month projection,
// not a per-bucket exhaustion date — the honest framing for "this tag's pace," with
// no fabricated per-bucket cap. The Projection's DaysToCap/ExhaustionDate are
// account-wide and intentionally not surfaced per bucket.

// BucketProjection pairs a bucket key (an agent id or workflow id) with the
// burn-rate Projection over that bucket's own slice of the ledger. Credits is the
// bucket's total spend — the deterministic sort key (spend descending, like the
// *Shares readers) and a display figure.
type BucketProjection struct {
	Key        string
	Credits    float64
	Projection Projection
}

// DailyTotalsBy buckets records by keyOf and returns each bucket's own ascending
// daily series — the DailyTotals shape (ADR-0009), one series per bucket. When
// includeEmpty is false, records whose key is "" are skipped entirely (mirroring
// shareBy, so non-workflow chat spend doesn't form a phantom bucket). Pure: same
// records → same map.
func DailyTotalsBy(records []SpendRecord, keyOf func(SpendRecord) string, includeEmpty bool) map[string][]DayTotal {
	groups := map[string][]SpendRecord{}
	for _, r := range records {
		k := keyOf(r)
		if k == "" && !includeEmpty {
			continue
		}
		groups[k] = append(groups[k], r)
	}
	out := make(map[string][]DayTotal, len(groups))
	for k, recs := range groups {
		out[k] = DailyTotals(recs)
	}
	return out
}

// BucketForecasts runs the SAME Forecast slope (ADR-0019) over each agent/workflow
// bucket of the ledger against the account-wide budget, returning a per-bucket
// projection sorted by spend descending then key ascending (a total, deterministic
// order, like AgentShares/WorkflowShares). Each bucket's slope is computed over its
// OWN daily series via DailyTotalsBy — its own rate, its own ledger age — so a
// high-spend bucket projects a steeper rate than a quiet one and a single-day
// bucket clamps to one observed day rather than a mostly-absent week.
//
// The per-bucket Projection's DailyRate/UsedCredits are the bucket's own; its
// account-wide DaysToCap/ExhaustionDate are NOT the per-bucket framing (a bucket
// has no own allowance) — the caller surfaces the rate + a month projection, not a
// per-bucket cap. Forecast is reused unchanged so its slope gotchas stay
// single-sourced. Pure: same (records, budget, now) → same result, no IO.
func BucketForecasts(records []SpendRecord, budget Budget, now time.Time, keyOf func(SpendRecord) string, includeEmpty bool) []BucketProjection {
	byDay := DailyTotalsBy(records, keyOf, includeEmpty)
	out := make([]BucketProjection, 0, len(byDay))
	for k, daily := range byDay {
		var credits float64
		for _, d := range daily {
			credits += d.Credits
		}
		out = append(out, BucketProjection{
			Key:        k,
			Credits:    credits,
			Projection: Forecast(daily, budget, now),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Credits != out[j].Credits {
			return out[i].Credits > out[j].Credits
		}
		return out[i].Key < out[j].Key
	})
	return out
}
