package web

import (
	"strings"
	"testing"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// recentDay returns midday on the n-th day before today (UTC), so a forecast test
// can lay down a trailing-window series relative to the real clock the page uses.
func recentDay(n int) time.Time {
	return time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -n).Add(12 * time.Hour)
}

// TestTelemetryPageShowsForecast guards that the Telemetry page surfaces the
// burn-rate projection (A3 / ADR-0019) off the persisted ledger: a steady recent
// spend against an allowance must render the forecast line with a days/date.
func TestTelemetryPageShowsForecast(t *testing.T) {
	s, store := newSpendServer(t)
	s.allowance = 10_000 // plenty of headroom, so the projection is ProjectionOK
	// A week of steady spend ending today: 2 cr/day.
	for n := 0; n < 7; n++ {
		if err := store.Append(telemetry.SpendRecord{At: recentDay(n), Model: "gpt-5", USD: 0.02}); err != nil {
			t.Fatal(err)
		}
	}
	html := s.telemetryPartial(defaultSpendWindow)
	if !strings.Contains(html, "Forecast") {
		t.Fatalf("telemetry page missing the Forecast section:\n%s", html)
	}
	// The OK projection names a day-count to the cap.
	if !strings.Contains(html, "day") {
		t.Errorf("forecast line should project a days-to-cap figure:\n%s", html)
	}
}

// TestTelemetryForecastNoBudgetHint guards the degenerate path: with no allowance
// configured the page must explain there's nothing to project toward rather than
// render a bogus date.
func TestTelemetryForecastNoBudgetHint(t *testing.T) {
	s, store := newSpendServer(t)
	s.allowance = 0 // no budget set
	if err := store.Append(telemetry.SpendRecord{At: recentDay(0), Model: "gpt-5", USD: 0.02}); err != nil {
		t.Fatal(err)
	}
	html := s.telemetryPartial(defaultSpendWindow)
	if !strings.Contains(html, "Forecast") {
		t.Fatalf("telemetry page missing the Forecast section:\n%s", html)
	}
	if !strings.Contains(html, "budget") {
		t.Errorf("no-allowance forecast should hint at setting a budget:\n%s", html)
	}
}

// TestStatlineShowsForecastCell guards the compact statusline cell: an OK
// projection adds a "cap ~Nd" cell read account-wide off the ledger.
func TestStatlineShowsForecastCell(t *testing.T) {
	s, store := newSpendServer(t)
	s.spec.Model = "gpt-5"
	s.allowance = 10_000
	for n := 0; n < 7; n++ {
		if err := store.Append(telemetry.SpendRecord{At: recentDay(n), Model: "gpt-5", USD: 0.02}); err != nil {
			t.Fatal(err)
		}
	}
	got := renderStatline(s)
	if !strings.Contains(got, "stat-forecast") {
		t.Errorf("statusline should show the compact forecast cell:\n%s", got)
	}
}

// TestStatlineNoForecastCellWithoutBudget guards that the compact cell stays
// absent when there's nothing to project (no allowance), keeping the statusline
// uncluttered.
func TestStatlineNoForecastCellWithoutBudget(t *testing.T) {
	s, store := newSpendServer(t)
	s.spec.Model = "gpt-5"
	s.allowance = 0
	if err := store.Append(telemetry.SpendRecord{At: recentDay(0), Model: "gpt-5", USD: 0.02}); err != nil {
		t.Fatal(err)
	}
	if got := renderStatline(s); strings.Contains(got, "stat-forecast") {
		t.Errorf("statusline must not show a forecast cell with no budget:\n%s", got)
	}
}
