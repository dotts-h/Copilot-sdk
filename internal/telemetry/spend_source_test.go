package telemetry

import (
	"testing"
	"time"
)

// TestActualCredits table-tests the estimate-vs-reported selection at the heart of
// ADR-0033: ReportedAIU (1 AIU = 1 credit) is the actual-spend source of truth when
// present; the price-book estimate is the fallback. The boundary is "reported > 0":
// a zero reported figure means the runtime never reported (offline mock / unreported
// turn), so the estimate carries.
func TestActualCredits(t *testing.T) {
	for _, tc := range []struct {
		name           string
		estimate       float64
		reportedAIU    float64
		wantCredits    float64
		wantFromReport bool
	}{
		{"reported present overrides estimate", 1.20, 1.55, 1.55, true},
		{"no reported falls back to estimate", 1.20, 0, 1.20, false},
		{"reported present even when it equals estimate", 1.20, 1.20, 1.20, true},
		{"both zero is still an estimate", 0, 0, 0, false},
		{"reported present beats a zero estimate", 0, 0.42, 0.42, true},
		{"negative reported is ignored (never real spend)", 0.30, -1, 0.30, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ActualCredits(tc.estimate, tc.reportedAIU)
			approx(t, got, tc.wantCredits)
			if HasReported(tc.reportedAIU) != tc.wantFromReport {
				t.Fatalf("HasReported(%v) = %v, want %v", tc.reportedAIU, HasReported(tc.reportedAIU), tc.wantFromReport)
			}
		})
	}
}

// TestMeterActualCreditsPrefersReported asserts the Meter folds the same selection:
// once the runtime reports an AIU, ActualCredits switches from the price-book estimate
// to the reported figure, and HasReported flips true.
func TestMeterActualCreditsPrefersReported(t *testing.T) {
	m := NewMeter(DefaultPriceBook())
	m.Record(Usage{Model: "gpt-5", InputTokens: 1_000_000})
	estimate := m.Totals().Credits()
	if estimate <= 0 {
		t.Fatalf("expected a non-zero price-book estimate, got %v", estimate)
	}
	// No reported AIU yet: actual == estimate, and it is flagged as an estimate.
	if m.HasReported() {
		t.Fatal("HasReported must be false before any AIU is reported")
	}
	approx(t, m.ActualCredits(), estimate)

	// Once the runtime reports, actual spend switches to the reported figure.
	m.RecordReportedAIU(0.42)
	if !m.HasReported() {
		t.Fatal("HasReported must be true once an AIU is reported")
	}
	approx(t, m.ActualCredits(), 0.42)
}

// TestSpendRecordActualVsEstimate asserts a ledger record reports both figures and
// prefers the reported AIU for actual spend, falling back to the estimate-priced USD
// when the runtime never reported.
func TestSpendRecordActualVsEstimate(t *testing.T) {
	reported := SpendRecord{Model: "gpt-5", USD: 0.012, AIU: 1.55}
	approx(t, reported.EstimateCredits(), 1.2) // 0.012 / 0.01
	approx(t, reported.ActualCredits(), 1.55)
	if !reported.HasReported() {
		t.Fatal("a record with a non-zero AIU is reported-backed")
	}

	estimated := SpendRecord{Model: "gpt-5", USD: 0.012, AIU: 0}
	approx(t, estimated.EstimateCredits(), 1.2)
	approx(t, estimated.ActualCredits(), 1.2) // falls back to the estimate
	if estimated.HasReported() {
		t.Fatal("a record with a zero AIU is estimate-backed")
	}
}

// TestMonthToDateActual asserts the account-wide month total can be read on the
// authoritative-cost-first basis: each turn contributes its reported AIU when present
// and its estimate otherwise, and the aggregate flags whether every / any contributing
// turn was reported (so the surface can label a partial month as a mix).
func TestMonthToDateActual(t *testing.T) {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	day := func(d int) time.Time { return time.Date(2026, 6, d, 9, 0, 0, 0, time.UTC) }
	records := []SpendRecord{
		{At: day(3), Model: "gpt-5", USD: 0.012, AIU: 1.55},                                    // reported
		{At: day(5), Model: "gpt-5", USD: 0.020, AIU: 0},                                       // estimate-only
		{At: time.Date(2026, 5, 30, 9, 0, 0, 0, time.UTC), Model: "gpt-5", USD: 0.500, AIU: 9}, // prior month, excluded
	}
	a := MonthToDateActual(records, now)
	// Actual = reported(1.55) + estimate(0.020/0.01 = 2.0) = 3.55.
	approx(t, a.ActualCredits, 3.55)
	// Estimate = (0.012 + 0.020)/0.01 = 3.2.
	approx(t, a.EstimateCredits, 3.2)
	// Reported = just the reported turn.
	approx(t, a.ReportedCredits, 1.55)
	if a.AllReported {
		t.Fatal("month has an estimate-only turn, so AllReported must be false")
	}
	if !a.AnyReported {
		t.Fatal("month has a reported turn, so AnyReported must be true")
	}
}

// TestMonthToDateActualAllReported asserts AllReported flips true only when every
// in-month turn carried a reported AIU (an empty month reports neither).
func TestMonthToDateActualAllReported(t *testing.T) {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	day := func(d int) time.Time { return time.Date(2026, 6, d, 9, 0, 0, 0, time.UTC) }
	all := MonthToDateActual([]SpendRecord{
		{At: day(3), USD: 0.012, AIU: 1.55},
		{At: day(5), USD: 0.020, AIU: 2.10},
	}, now)
	if !all.AllReported || !all.AnyReported {
		t.Fatalf("every turn reported: want AllReported && AnyReported, got %+v", all)
	}
	empty := MonthToDateActual(nil, now)
	if empty.AllReported || empty.AnyReported {
		t.Fatalf("empty month reports neither, got %+v", empty)
	}
}
