package web

import (
	"html/template"
	"strings"
	"testing"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// fixedNow is a stable clock for the dashboard view tests so the current/prior
// window split is deterministic.
var fixedNow = time.Date(2026, time.June, 15, 12, 0, 0, 0, time.UTC)

// seedDashboard appends spend across the current and prior 14-day windows so the
// KPI cards have a real period-over-period delta. Current (last 14 days) spends
// more than the prior window, so spend/turns rise.
func seedDashboard(t *testing.T, store *telemetry.SpendStore) {
	t.Helper()
	day := func(d int) time.Time { return fixedNow.AddDate(0, 0, -d) }
	for _, r := range []telemetry.SpendRecord{
		{At: day(0), Model: "gpt-5", USD: 0.10},  // current window
		{At: day(2), Model: "gpt-5", USD: 0.20},  // current
		{At: day(5), Model: "gpt-5", USD: 0.15},  // current
		{At: day(18), Model: "gpt-5", USD: 0.05}, // prior window (14..27 days ago)
		{At: day(20), Model: "gpt-5", USD: 0.04}, // prior
	} {
		if err := store.Append(r); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDashboardViewBuildsFourCards(t *testing.T) {
	s, store := newSpendServer(t)
	s.allowance = 1500 // a budget so the spend-vs-budget bullet renders
	seedDashboard(t, store)

	view := s.dashboardView(14, fixedNow)
	if view == nil {
		t.Fatal("dashboardView returned nil with seeded spend")
	}
	cards, ok := view["Cards"].([]map[string]any)
	if !ok || len(cards) != 4 {
		t.Fatalf("want 4 KPI cards, got %#v", view["Cards"])
	}
	wantLabels := []string{"Total spend", "Turns", "Avg cost/turn", "Burn rate"}
	for i, want := range wantLabels {
		if cards[i]["Label"] != want {
			t.Errorf("card %d label = %v, want %q", i, cards[i]["Label"], want)
		}
		if _, ok := cards[i]["Value"].(string); !ok {
			t.Errorf("card %d missing a string Value", i)
		}
		if _, ok := cards[i]["Spark"].(template.HTML); !ok {
			t.Errorf("card %d missing a Spark svg", i)
		}
	}

	// Total spend rose vs the prior window and spend is higher-is-worse, so the
	// delta is an "up" direction with a "warn" tone (▲ is not green for spend).
	spendDelta := cards[0]["Delta"].(map[string]any)
	if spendDelta["Dir"] != "up" {
		t.Errorf("spend delta Dir = %v, want up", spendDelta["Dir"])
	}
	if spendDelta["Tone"] != "warn" {
		t.Errorf("spend delta Tone = %v, want warn (spend ▲ is bad)", spendDelta["Tone"])
	}

	// Turns also rose, but more turns is not "worse", so the same up direction is
	// a "good" tone — the per-metric higher-is-worse flag, not a blanket green/red.
	turnsDelta := cards[1]["Delta"].(map[string]any)
	if turnsDelta["Dir"] != "up" || turnsDelta["Tone"] != "good" {
		t.Errorf("turns delta = %v, want up/good", turnsDelta)
	}

	// The two charts render.
	if _, ok := view["Band"].(template.HTML); !ok {
		t.Error("dashboardView missing the trend Band svg")
	}
	if _, ok := view["Bullet"].(template.HTML); !ok {
		t.Error("dashboardView missing the spend-vs-budget Bullet svg (allowance set)")
	}
}

func TestDashboardViewNilWithoutHistory(t *testing.T) {
	s, _ := newTestServer() // no spend store wired
	if v := s.dashboardView(14, fixedNow); v != nil {
		t.Fatalf("dashboardView should be nil without a ledger, got %#v", v)
	}
}

func TestTelemetryPageRendersKPIDashboard(t *testing.T) {
	s, store := newSpendServer(t)
	s.allowance = 1500
	// Seed relative to the REAL clock telemetryPartial reads (time.Now), so the
	// records land in the current/prior windows the page computes.
	now := time.Now()
	day := func(d int) time.Time { return now.AddDate(0, 0, -d) }
	for _, r := range []telemetry.SpendRecord{
		{At: day(0), Model: "gpt-5", USD: 0.10},
		{At: day(3), Model: "gpt-5", USD: 0.20},
		{At: day(18), Model: "gpt-5", USD: 0.05},
	} {
		if err := store.Append(r); err != nil {
			t.Fatal(err)
		}
	}
	html := s.telemetryPartial(defaultSpendWindow)
	for _, want := range []string{
		`class="kpi"`,            // the KPI card row
		`class="kpi-card glass"`, // a big-number card on the glass border (W4, issue 0066)
		`class="kpi-value"`,      // the big number
		`class="kpi-delta`,       // the Δ badge (direction/tone classes follow)
		`class="spark"`,          // a per-card sparkline svg
		`class="band"`,           // the cumulative trend band svg
		`class="bullet"`,         // the spend-vs-budget bullet svg
		"Total spend",            // a card label
		"Burn rate",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("telemetry page missing dashboard marker %q\n%s", want, html)
		}
	}
}
