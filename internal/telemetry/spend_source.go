// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package telemetry

import "time"

// The authoritative-cost-first source hierarchy (ADR-0033).
//
// GitHub's runtime reports each turn's authoritative cost as a ReportedAIU figure.
// An AI unit (AIU) and an AI Credit are the same unit — GitHub's usage-based meter
// denominates both in the credit whose USD value is USDPerCredit — so a reported AIU
// IS the actual credit spend, at parity (1 AIU = 1 credit). The static price book is
// demoted to an *estimate*: it powers the pre-flight composer and the burn-rate
// forecast, and it is the offline fallback when the runtime never reports (the mock,
// or an unreported turn). It is never the source of truth for actual spend.
//
// The selection below is the single seam every "what did this actually cost" read
// flows through, so estimate and reported can't silently swap roles. Keep it pure.

// HasReported reports whether a reported-AIU figure represents real reported spend.
// The boundary is strictly positive: a zero AIU means the runtime never reported
// (offline mock / unreported turn), and a negative value is never real spend, so both
// fall back to the estimate.
func HasReported(reportedAIU float64) bool { return reportedAIU > 0 }

// ReportedCredits converts a reported AIU figure to credits. AIU and credit are the
// same unit (ADR-0033), so the conversion is identity — the named function exists so
// the parity is stated once and callers read intent, not a bare value.
func ReportedCredits(reportedAIU float64) float64 { return reportedAIU }

// ActualCredits picks the actual per-turn (or aggregate) spend on the
// authoritative-cost-first basis: the reported AIU when the runtime reported one
// (HasReported), else the price-book estimate. Pure: same inputs → same answer.
func ActualCredits(estimateCredits, reportedAIU float64) float64 {
	if HasReported(reportedAIU) {
		return ReportedCredits(reportedAIU)
	}
	return estimateCredits
}

// ActualSpend is the account-wide month total read on the authoritative-cost-first
// basis (MonthToDateActual): the actual credits (reported-where-present, estimate
// elsewhere) alongside the pure estimate and pure reported sums, plus whether every /
// any in-window turn carried a reported figure — so a surface can label the total as
// reported, estimated, or a mix without re-walking the ledger.
type ActualSpend struct {
	// ActualCredits is the authoritative-cost-first total: each turn's reported AIU
	// when present, its estimate otherwise.
	ActualCredits float64
	// EstimateCredits is the pure price-book total across every turn (the figure the
	// pre-ADR-0033 meter reported as "spend").
	EstimateCredits float64
	// ReportedCredits sums only the turns the runtime reported (AIU → credits).
	ReportedCredits float64
	// AnyReported / AllReported describe the reported coverage of the window: AllReported
	// is true only when every contributing turn was reported (and the window is non-empty),
	// so the total is fully authoritative; AnyReported is true when at least one was, so it
	// is at least partly authoritative. Both false → a pure estimate (offline / empty).
	AnyReported bool
	AllReported bool
}

// MonthToDateActual sums now's UTC calendar month on the authoritative-cost-first
// basis: each in-month turn contributes its reported AIU when present and its
// estimate-priced credits otherwise, and the result also carries the pure estimate
// and reported sub-totals plus the reported-coverage flags. Pure: same records + now
// → same result, no IO. It mirrors MonthToDate's window so the cost footer can switch
// to actual spend without changing which turns it counts. — ADR-0033.
func MonthToDateActual(records []SpendRecord, now time.Time) ActualSpend {
	y, m, _ := now.UTC().Date()
	var a ActualSpend
	a.AllReported = true // vacuously true until an estimate-only turn is seen
	counted := 0
	for _, r := range records {
		ry, rm, _ := r.At.UTC().Date()
		if ry != y || rm != m {
			continue
		}
		counted++
		a.EstimateCredits += r.EstimateCredits()
		a.ActualCredits += r.ActualCredits()
		if r.HasReported() {
			a.ReportedCredits += ReportedCredits(r.AIU)
			a.AnyReported = true
		} else {
			a.AllReported = false
		}
	}
	if counted == 0 {
		a.AllReported = false // an empty window is neither all- nor any-reported
	}
	return a
}
