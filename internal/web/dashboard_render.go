// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package web

import (
	"fmt"
	"html/template"
	"math"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// This file assembles the Telemetry KPI-dashboard view (V23, ADR-0027): a row of
// big-number cards (total spend, turns, avg cost/turn, daily burn rate), each
// with a period-over-period delta and a sparkline, plus the cumulative trend band
// and the spend-vs-budget bullet. The numbers come from the pure telemetry reader
// (telemetry.Dashboard); the SVGs from the pure builders (svg.go); this layer
// only joins them into the template data. No new route, no schema change —
// telemetryPartial computes it and the existing ?window= htmx swap re-renders it.

// dashboardView builds the KPI-card + chart view data for the Telemetry page from
// the persisted ledger over the chosen window (now-relative). Returns nil when no
// ledger is wired or it holds no records, so the template hides the whole block.
// now is threaded in (not read off the clock) so the window split and forecast
// are deterministic per render and unit-testable.
func (s *Server) dashboardView(window int, now time.Time) map[string]any {
	if s.spend == nil {
		return nil
	}
	records := s.spend.Records()
	if len(records) == 0 {
		return nil
	}
	dash := telemetry.Dashboard(records, now, window)
	cur, prior := dash.Current, dash.Prior

	// Per-day sparkline series, one value per window day (zero-filled).
	credits := make([]float64, len(dash.Series))
	turns := make([]float64, len(dash.Series))
	avg := make([]float64, len(dash.Series))
	for i, p := range dash.Series {
		credits[i] = p.Credits
		turns[i] = float64(p.Turns)
		if p.Turns > 0 {
			avg[i] = p.Credits / float64(p.Turns)
		}
	}

	// The card set + per-metric "higher-is-worse" flag (ADR-0027): spend, avg
	// cost/turn, and burn rate are worse when they rise (▲ → warn); more turns is
	// activity, not waste (▲ → good) — so ▲ is not blanket-green.
	cards := []map[string]any{
		kpiCard("Total spend", telemetry.FormatCredits(cur.Credits), credits,
			telemetry.ChangePct(prior.Credits, cur.Credits), true, window),
		kpiCard("Turns", fmt.Sprintf("%d", cur.Turns), turns,
			telemetry.ChangePct(float64(prior.Turns), float64(cur.Turns)), false, window),
		kpiCard("Avg cost/turn", telemetry.FormatCredits(cur.AvgCostPerTurn()), avg,
			telemetry.ChangePct(prior.AvgCostPerTurn(), cur.AvgCostPerTurn()), true, window),
		kpiCard("Burn rate", telemetry.FormatCredits(cur.DailyRate())+"/day", credits,
			telemetry.ChangePct(prior.DailyRate(), cur.DailyRate()), true, window),
	}
	view := map[string]any{"Cards": cards, "Window": window}

	// Trend band: cumulative actuals (solid area) + a dashed burn-rate forecast
	// continuing at the window's daily rate over the days left this month.
	cum := make([]float64, len(credits))
	run := 0.0
	for i, v := range credits {
		run += v
		cum[i] = run
	}
	forecastDays := daysLeftInMonth(now)
	if forecastDays > window {
		forecastDays = window
	}
	if forecastDays < 1 {
		forecastDays = 1
	}
	rate := cur.DailyRate()
	fc := make([]float64, forecastDays)
	frun := run
	for i := range fc {
		frun += rate
		fc[i] = frun
	}
	bandLabel := fmt.Sprintf("Cumulative spend over the last %d days — %s to date — with a dashed burn-rate forecast",
		window, telemetry.FormatCredits(run))
	view["Band"] = template.HTML(trendBandSVG(cum, fc, bandLabel)) //nolint:gosec // pure builder; label escaped via esc()

	// Bullet: month-to-date spend against the monthly budget, with a target marker
	// at the projected month-end spend at the current pace. Only when a budget is set.
	budget := s.budget()
	if budget.AllowanceCredits > 0 {
		// Month-to-date off the SAME threaded `now` as the forecast horizon, so a
		// render near a month boundary can't combine one month's spend with another
		// month's days-remaining (the purity this function promises).
		mtd := s.budgets().MonthToDate(now).Credits()
		projected := mtd + rate*float64(daysLeftInMonth(now))
		scaleMax := math.Max(budget.AllowanceCredits, math.Max(projected, mtd))
		over := projected > budget.AllowanceCredits
		bulletLabel := fmt.Sprintf("Spend %s of a %s monthly budget; projected %s by month-end at the current pace",
			telemetry.FormatCredits(mtd), telemetry.FormatCredits(budget.AllowanceCredits), telemetry.FormatCredits(projected))
		view["Bullet"] = template.HTML(bulletSVG(mtd, projected, scaleMax, over, bulletLabel)) //nolint:gosec // pure builder; label escaped
		view["BulletLabel"] = "Spend vs budget"
	}
	return view
}

// kpiCard assembles one big-number card: a label, the formatted value, a
// sparkline over the metric's daily series, and the period-over-period delta
// badge. higherIsWorse flips the delta coloring (a rise in spend is warn, a rise
// in turns is good). The series label names the metric + window for the svg's
// accessible name.
func kpiCard(label, value string, series []float64, delta telemetry.Delta, higherIsWorse bool, window int) map[string]any {
	sparkLabel := fmt.Sprintf("%s trend over the last %d days", label, window)
	return map[string]any{
		"Label": label,
		"Value": value,
		"Spark": template.HTML(sparklineSVG(series, sparkLabel)), //nolint:gosec // pure builder; label escaped
		"Delta": deltaView(delta, higherIsWorse),
	}
}

// deltaView maps a telemetry.Delta + the metric's higher-is-worse flag to the
// badge's glyph, a stable direction class (up/down/flat), a tone class
// (good/warn/neutral), and the display text. A "new" metric (no prior baseline)
// and an unchanged metric are neutral — neither favorable nor not. The tone
// flips on favorability: a rise is favorable iff the metric is higher-is-better.
func deltaView(d telemetry.Delta, higherIsWorse bool) map[string]any {
	glyph, dir := "→", "flat"
	switch d.Dir {
	case 1:
		glyph, dir = "▲", "up"
	case -1:
		glyph, dir = "▼", "down"
	}
	var text, tone string
	switch {
	case !d.HasPrior && d.Dir == 0:
		text, tone = "—", "neutral" // no spend either period
	case !d.HasPrior:
		text, tone = "new", "neutral" // no prior baseline to judge favorability
	case d.Dir == 0:
		text, tone = "0%", "neutral"
	default:
		text = fmt.Sprintf("%.0f%%", math.Abs(d.Fraction)*100)
		if (d.Dir == 1) == higherIsWorse {
			tone = "warn" // rose & higher-is-worse, or fell & higher-is-better
		} else {
			tone = "good"
		}
	}
	return map[string]any{"Glyph": glyph, "Dir": dir, "Tone": tone, "Text": text}
}
