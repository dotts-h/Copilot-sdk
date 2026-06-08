package telemetry

import (
	"math"
	"testing"
	"time"
)

// at builds a fixed UTC timestamp at noon on the given calendar day so a record
// lands unambiguously inside one window bucket.
func ts(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 12, 0, 0, 0, time.UTC)
}

func TestDashboardSplitsCurrentAndPriorWindows(t *testing.T) {
	now := ts(2026, time.June, 15)
	// window=7 → current [06-09..06-15], prior [06-02..06-08].
	records := []SpendRecord{
		{At: ts(2026, time.June, 15), USD: 0.10}, // current
		{At: ts(2026, time.June, 10), USD: 0.20}, // current
		{At: ts(2026, time.June, 10), USD: 0.05}, // current (same day)
		{At: ts(2026, time.June, 5), USD: 0.30},  // prior
		{At: ts(2026, time.May, 20), USD: 1.00},  // outside both windows
	}
	d := Dashboard(records, now, 7)

	if d.Window != 7 {
		t.Fatalf("Window = %d, want 7", d.Window)
	}
	// Current: (0.10+0.20+0.05)/0.01 = 35 cr over 3 turns.
	if got := d.Current.Credits; math.Abs(got-35) > 1e-9 {
		t.Errorf("Current.Credits = %v, want 35", got)
	}
	if d.Current.Turns != 3 {
		t.Errorf("Current.Turns = %d, want 3", d.Current.Turns)
	}
	if d.Current.Days != 7 || d.Prior.Days != 7 {
		t.Errorf("window Days = %d/%d, want 7/7", d.Current.Days, d.Prior.Days)
	}
	// Prior: 0.30/0.01 = 30 cr over 1 turn. The 05-20 record is excluded entirely.
	if got := d.Prior.Credits; math.Abs(got-30) > 1e-9 {
		t.Errorf("Prior.Credits = %v, want 30 (05-20 must be excluded)", got)
	}
	if d.Prior.Turns != 1 {
		t.Errorf("Prior.Turns = %d, want 1", d.Prior.Turns)
	}
}

func TestDashboardSeriesIsZeroFilledAndAscending(t *testing.T) {
	now := ts(2026, time.June, 15)
	records := []SpendRecord{
		{At: ts(2026, time.June, 15), USD: 0.10},
		{At: ts(2026, time.June, 10), USD: 0.20},
		{At: ts(2026, time.June, 10), USD: 0.05},
	}
	d := Dashboard(records, now, 7)

	if len(d.Series) != 7 {
		t.Fatalf("Series len = %d, want 7 (one point per window day)", len(d.Series))
	}
	// Ascending, contiguous: first day is now-(window-1), last is today.
	if d.Series[0].Day != "2026-06-09" || d.Series[6].Day != "2026-06-15" {
		t.Fatalf("Series spans %s..%s, want 2026-06-09..2026-06-15", d.Series[0].Day, d.Series[6].Day)
	}
	// 06-10 (index 1) aggregates both turns; idle days are present and zero.
	if got := d.Series[1]; got.Day != "2026-06-10" || math.Abs(got.Credits-25) > 1e-9 || got.Turns != 2 {
		t.Errorf("Series[1] = %+v, want 2026-06-10 / 25cr / 2 turns", got)
	}
	if got := d.Series[2]; got.Credits != 0 || got.Turns != 0 {
		t.Errorf("Series[2] (idle) = %+v, want zero-filled", got)
	}
	if got := d.Series[6]; math.Abs(got.Credits-10) > 1e-9 || got.Turns != 1 {
		t.Errorf("Series[6] = %+v, want 10cr / 1 turn", got)
	}
}

func TestWindowSpendDerivedMetrics(t *testing.T) {
	w := WindowSpend{Credits: 35, Turns: 4, Days: 7}
	if got := w.AvgCostPerTurn(); math.Abs(got-8.75) > 1e-9 {
		t.Errorf("AvgCostPerTurn = %v, want 8.75", got)
	}
	if got := w.DailyRate(); math.Abs(got-5) > 1e-9 {
		t.Errorf("DailyRate = %v, want 5", got)
	}
	// Degenerate: no turns / no days never divides by zero.
	empty := WindowSpend{Credits: 10}
	if empty.AvgCostPerTurn() != 0 || empty.DailyRate() != 0 {
		t.Errorf("zero turns/days must yield 0, got avg=%v rate=%v", empty.AvgCostPerTurn(), empty.DailyRate())
	}
}

func TestChangePctSemantics(t *testing.T) {
	cases := []struct {
		name         string
		prior, cur   float64
		wantDir      int
		wantHasPrior bool
		wantFraction float64
	}{
		{"rose", 20, 30, 1, true, 0.5},
		{"fell", 40, 30, -1, true, -0.25},
		{"flat", 30, 30, 0, true, 0},
		{"new from zero", 0, 12, 1, false, 0},
		{"both zero", 0, 0, 0, false, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := ChangePct(c.prior, c.cur)
			if d.Dir != c.wantDir {
				t.Errorf("Dir = %d, want %d", d.Dir, c.wantDir)
			}
			if d.HasPrior != c.wantHasPrior {
				t.Errorf("HasPrior = %v, want %v", d.HasPrior, c.wantHasPrior)
			}
			if math.Abs(d.Fraction-c.wantFraction) > 1e-9 {
				t.Errorf("Fraction = %v, want %v", d.Fraction, c.wantFraction)
			}
		})
	}
}
