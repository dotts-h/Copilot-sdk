// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package web

import (
	"strings"
	"testing"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// seedLedger attaches an ephemeral (in-memory, dir="") spend ledger to s and
// appends the given records — the proxy for a persisted ledger that outlived the
// process while the in-memory Meter started fresh (ADR-0016 / issue 0014).
func seedLedger(t *testing.T, s *Server, recs ...telemetry.SpendRecord) {
	t.Helper()
	store, err := telemetry.LoadSpendStore("")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range recs {
		if err := store.Append(r); err != nil {
			t.Fatal(err)
		}
	}
	s.spend = store
}

// TestBudgetRowsSurviveFreshMeter is the restart proxy: the account-wide rows
// must reflect this month's persisted spend even though the in-process Meter is
// empty (the state right after a restart). Before ADR-0016 they read the meter
// and reset to zero.
func TestBudgetRowsSurviveFreshMeter(t *testing.T) {
	s, _ := newTestServer()
	s.spec.Model = "gpt-5"
	s.allowance = 200
	// A persisted turn from earlier this month (e.g. a prior process), $1.00 = 100 cr.
	seedLedger(t, s, telemetry.SpendRecord{At: time.Now().UTC(), Model: "gpt-5", USD: 1.00})

	// The Meter is fresh — if any account-wide row read it, it would show 0.
	if got := s.meter.Totals().Credits(); got != 0 {
		t.Fatalf("precondition: meter must be empty (fresh process), got %v cr", got)
	}

	page := s.telemetryPartial(defaultSpendWindow)
	if !strings.Contains(page, "100.00 cr") {
		t.Errorf("Total cost must reflect the persisted ledger (100 cr), not the fresh meter: %q", page)
	}
	if !strings.Contains(page, telemetry.FormatCredits(100)) { // Remaining = 200 - 100
		t.Errorf("Remaining must be ledger-derived (100 cr left of 200): %q", page)
	}
}

// TestCapBaselineReadsLedger guards that the hard-cap projection measures "used"
// from the persisted month-to-date, so a mid-month restart doesn't silently
// re-open a cap the user already approached.
func TestCapBaselineReadsLedger(t *testing.T) {
	s, _ := newTestServer()
	s.spec.Model = "gpt-5"
	s.hardCap = 100
	s.ctxCurrent = 1 // a negligible next-turn estimate, so the baseline dominates

	// Meter empty (fresh process) but the ledger already holds 100 cr this month —
	// any turn on top of it breaches the cap purely from the persisted baseline.
	seedLedger(t, s, telemetry.SpendRecord{At: time.Now().UTC(), Model: "gpt-5", USD: 1.00})

	projected, capped := s.overCap()
	if projected < 100 {
		t.Errorf("projection must include the persisted month-to-date baseline (≥100 cr), got %v", projected)
	}
	if !capped {
		t.Errorf("a turn on top of a 100 cr ledger must breach the 100 cr cap, got projected=%v", projected)
	}
}

// TestRecordUsageHitsBothMetersAndLedger guards the shared recordUsage helper
// (extracted from the EvUsage reducers): a single metered turn must land in the
// account-wide meter, the per-session meter, AND the persisted ledger — drop any
// one and a surface (footer, statusline, or restart-survival) silently drifts.
func TestRecordUsageHitsBothMetersAndLedger(t *testing.T) {
	s, _ := newTestServer()
	s.spec.Model = "gpt-5"
	s.sessionID = "sess-guard"
	seedLedger(t, s) // empty, writable ledger

	_ = s.handleEvent(usageEvent("gpt-5", 80_000))

	if got := s.meter.Count(); got != 1 {
		t.Errorf("account-wide meter must record the turn, count=%d", got)
	}
	if got := s.sessionMeter.Count(); got != 1 {
		t.Errorf("per-session meter must record the turn (statusline), count=%d", got)
	}
	if got := s.spend.Count(); got != 1 {
		t.Errorf("persisted ledger must record the turn (restart survival), count=%d", got)
	}
	recs := s.spend.Records()
	if len(recs) != 1 || recs[0].SessionID != "sess-guard" || recs[0].Model != "gpt-5" {
		t.Errorf("ledger record must carry the turn's session and model: %+v", recs)
	}
}

// TestTelemetryPageStaysAccountWide guards the deliberate split: the Telemetry
// page's account-wide rows aggregate every session via the persisted ledger
// (ADR-0016), not just this conversation's in-process meter.
func TestTelemetryPageStaysAccountWide(t *testing.T) {
	s, _ := newTestServer()
	s.spec.Model = "gpt-5"
	// Account-wide spend from another session, persisted to the shared ledger.
	seedLedger(t, s, telemetry.SpendRecord{At: time.Now().UTC(), SessionID: "other", Model: "gpt-5", USD: 1.00}) // 100 cr

	page := s.telemetryPartial(defaultSpendWindow)
	if !strings.Contains(page, "100.00 cr") {
		t.Errorf("the Telemetry page must report the account-wide ledger total, not just this session: %q", page)
	}
}

// TestCostFooterReadsLedger guards that the topbar cost footer reports
// month-to-date from the ledger (account-wide, restart-surviving), not the live
// meter — same source as the Telemetry rows.
func TestCostFooterReadsLedger(t *testing.T) {
	s, _ := newTestServer()
	s.allowance = 200
	seedLedger(t, s, telemetry.SpendRecord{At: time.Now().UTC(), Model: "gpt-5", USD: 0.50}) // 50 cr

	got := renderCostFooter(s.monthToDate().Credits(), s.budget())
	if !strings.Contains(got, "50.00 cr") {
		t.Errorf("cost footer must report the ledger month-to-date (50 cr): %q", got)
	}
	// A fresh meter would render 0 — make sure that's not what we're seeing.
	if s.meter.Totals().Credits() != 0 {
		t.Fatalf("precondition: meter must be empty, got %v", s.meter.Totals().Credits())
	}
}

// TestCostFooterPrefersReportedSpend asserts the topbar cost footer reports the
// account-wide month total on the authoritative-cost-first basis (ADR-0033): when the
// ledger's in-month turns carry a reported AIU, the footer shows the reported total
// (not the price-book estimate), and flags the figure as reported. An estimate-only
// ledger falls back to the estimate, flagged as such.
func TestCostFooterPrefersReportedSpend(t *testing.T) {
	s, _ := newTestServer()
	s.allowance = 200
	// A turn whose estimate (USD 0.50 = 50 cr) disagrees with GitHub's reported cost
	// (AIU 30 = 30 cr): the footer must trust the reported figure.
	seedLedger(t, s, telemetry.SpendRecord{At: time.Now().UTC(), Model: "gpt-5", USD: 0.50, AIU: 30})

	a := s.monthToDateActual()
	if a.ActualCredits != 30 {
		t.Fatalf("month-to-date actual must prefer the reported AIU (30 cr), got %v", a.ActualCredits)
	}
	got := renderActualCostFooter(a, s.budget())
	if !strings.Contains(got, "30.00 cr") {
		t.Errorf("cost footer must report the reported total (30 cr), not the estimate (50 cr): %q", got)
	}
	if strings.Contains(got, "50.00 cr") {
		t.Errorf("cost footer must not show the estimate when a reported figure is present: %q", got)
	}
}
