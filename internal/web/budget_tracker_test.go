package web

import (
	"testing"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// budgetTracker is the extracted cost-accounting module (a deep module behind a
// narrow interface): it answers Budget / MonthToDate / Forecast off the persisted
// ledger, independent of the Server. These tests exercise it in isolation — the
// testability win of the extraction.

func TestBudgetTrackerReadsOffTheLedger(t *testing.T) {
	store, err := telemetry.LoadSpendStore("") // ephemeral
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	// Two turns this month ($0.80), one last month ($9.00) — MonthToDate counts only
	// the current UTC month (ADR-0016), so the prior month is excluded.
	for _, r := range []telemetry.SpendRecord{
		{At: now.AddDate(0, 0, -1), Model: "gpt-5", USD: 0.50},
		{At: now.AddDate(0, 0, -2), Model: "gpt-5", USD: 0.30},
		{At: now.AddDate(0, -1, 0), Model: "gpt-5", USD: 9.00},
	} {
		if err := store.Append(r); err != nil {
			t.Fatal(err)
		}
	}
	bt := budgetTracker{allowance: 500, warnFraction: 0.8, hardCap: 1000, spend: store}

	if got := bt.Budget(); got.AllowanceCredits != 500 || got.WarnFraction != 0.8 || got.HardCapCredits != 1000 {
		t.Fatalf("Budget() = %+v, want the cached knobs {500, 0.8, 1000}", got)
	}
	if c := bt.MonthToDate(now).Credits(); c < 79.9 || c > 80.1 {
		t.Fatalf("MonthToDate = %v cr, want ~80 (this month's $0.80 only, ADR-0016)", c)
	}
	if _, ok := bt.Forecast(now); !ok {
		t.Fatal("Forecast should be available when the ledger holds records")
	}
}

func TestBudgetTrackerNilLedgerIsZero(t *testing.T) {
	// No ledger wired (the offline demo / a test without one): accounting reads zero
	// and the forecast is unavailable, but the budget knobs still surface.
	bt := budgetTracker{allowance: 500, spend: nil}
	if c := bt.MonthToDate(time.Now()).Credits(); c != 0 {
		t.Fatalf("nil-ledger MonthToDate = %v cr, want 0", c)
	}
	if _, ok := bt.Forecast(time.Now()); ok {
		t.Fatal("nil-ledger Forecast should be unavailable (ok=false)")
	}
	if bt.Budget().AllowanceCredits != 500 {
		t.Fatal("Budget should reflect the knobs even with no ledger")
	}
}
