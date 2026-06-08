package telemetry

import "time"

// This file adds the pure view-model behind the Telemetry KPI dashboard (V23,
// ADR-0027): a window's spend rolled up beside the immediately-preceding
// equal-length window (for period-over-period deltas) and the window's
// zero-filled daily series (the sparklines' source). Like the rest of the
// package it is a pure reader over a record slice — no IO, no clock beyond the
// caller's `now` — so the dashboard's numbers are table-tested in isolation and
// the web layer only builds SVG/markup over these figures. — ADR-0027.

// DayPoint is one calendar day's spend within a dashboard window: its UTC date,
// the credits spent, and the number of metered turns. Unlike DailyTotals (which
// omits idle days), a window series is **zero-filled** — one point per day in the
// window, including days with no spend — so a fixed-width sparkline has an
// evenly-spaced point per day.
type DayPoint struct {
	Day     string
	Credits float64
	Turns   int
}

// WindowSpend is the spend rolled up over one dashboard window (a contiguous run
// of Days UTC days): the total credits and turns, from which the derived KPI
// figures (average cost per turn, daily burn rate) are computed. Days is the
// window length, so DailyRate divides by the whole window (idle days included),
// not only the days that had spend.
type WindowSpend struct {
	Credits float64
	Turns   int
	Days    int
}

// AvgCostPerTurn is the window's mean credits per metered turn, or 0 when no
// turns were recorded (never divides by zero).
func (w WindowSpend) AvgCostPerTurn() float64 {
	if w.Turns <= 0 {
		return 0
	}
	return w.Credits / float64(w.Turns)
}

// DailyRate is the window's mean credits per day across the whole window, or 0
// when the window is empty (never divides by zero).
func (w WindowSpend) DailyRate() float64 {
	if w.Days <= 0 {
		return 0
	}
	return w.Credits / float64(w.Days)
}

// WindowDashboard is the KPI dashboard's pure view-model for one window: the
// current window's spend, the immediately-preceding equal-length window's spend
// (the period-over-period baseline), and the current window's zero-filled daily
// series (the sparklines' source). Pure: same (records, now, window) → same
// result, no IO.
type WindowDashboard struct {
	Window  int
	Current WindowSpend
	Prior   WindowSpend
	Series  []DayPoint // len == Window, ascending (oldest first), zero-filled
}

// Dashboard rolls records into the KPI dashboard view-model for a window ending
// on now's UTC day. The current window is the last `window` UTC days [today-(window-1)
// .. today]; the prior window is the equal-length span immediately before it
// [today-(2*window-1) .. today-window]. Spend outside both windows is ignored.
// The Series is the current window's per-day credits/turns, zero-filled and
// ascending. A window < 1 is clamped to 1. Pure: same inputs → same result.
func Dashboard(records []SpendRecord, now time.Time, window int) WindowDashboard {
	if window < 1 {
		window = 1
	}
	today := now.UTC().Truncate(24 * time.Hour)
	curStart := today.AddDate(0, 0, -(window - 1))
	priorStart := today.AddDate(0, 0, -(2*window - 1))

	type acc struct {
		credits float64
		turns   int
	}
	byDay := make(map[string]*acc)
	cur := WindowSpend{Days: window}
	prior := WindowSpend{Days: window}
	for _, r := range records {
		day := r.At.UTC().Truncate(24 * time.Hour)
		switch {
		case !day.Before(curStart) && !day.After(today):
			cur.Credits += r.Credits()
			cur.Turns++
			k := day.Format("2006-01-02")
			a := byDay[k]
			if a == nil {
				a = &acc{}
				byDay[k] = a
			}
			a.credits += r.Credits()
			a.turns++
		case !day.Before(priorStart) && day.Before(curStart):
			prior.Credits += r.Credits()
			prior.Turns++
		}
	}

	series := make([]DayPoint, 0, window)
	for i := 0; i < window; i++ {
		day := curStart.AddDate(0, 0, i)
		k := day.Format("2006-01-02")
		p := DayPoint{Day: k}
		if a := byDay[k]; a != nil {
			p.Credits = a.credits
			p.Turns = a.turns
		}
		series = append(series, p)
	}
	return WindowDashboard{Window: window, Current: cur, Prior: prior, Series: series}
}

// Delta is a period-over-period change between a prior and a current value: the
// signed fractional change, a discrete direction, and a "no prior baseline" flag
// for the case where the prior period had zero spend (the change is "new", not an
// infinite percentage). The web layer maps Dir + a per-metric "higher-is-worse"
// flag to the ▲/▼ glyph and the good/warn coloring.
type Delta struct {
	// Fraction is the signed change (current-prior)/prior; 0 when there is no
	// prior baseline (HasPrior == false).
	Fraction float64
	// Dir is +1 (rose), -1 (fell), or 0 (unchanged / no movement from zero).
	Dir int
	// HasPrior is false when the prior baseline was non-positive — render the
	// change as "new" rather than a percentage.
	HasPrior bool
}

// ChangePct computes the period-over-period Delta from prior to current. A
// non-positive prior baseline yields HasPrior == false (Dir is +1 when there is
// current spend to surface as "new", else 0). Pure: same inputs → same Delta.
func ChangePct(prior, current float64) Delta {
	const eps = 1e-9
	var d Delta
	if prior <= eps {
		if current > eps {
			d.Dir = 1
		}
		return d
	}
	d.HasPrior = true
	d.Fraction = (current - prior) / prior
	switch {
	case d.Fraction > eps:
		d.Dir = 1
	case d.Fraction < -eps:
		d.Dir = -1
	}
	return d
}
