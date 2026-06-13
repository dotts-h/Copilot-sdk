// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package telemetry

import (
	"math"
	"strings"
	"time"
)

// This file adds a pure burn-rate projection over the daily spend series — the
// predictive half of the cost differentiator. The reactive guardrails (soft warn,
// hard cap — ADR-0008) only fire once you are already near the line; Forecast
// answers the question they can't: "at this rate, when do I run out?" It reads the
// same persisted ledger the account-wide rows read — via DailyTotals (ADR-0009)
// and a month-to-date sum consistent with MonthToDate (ADR-0016) — adds no IO, and
// is fully unit-tested. The slope is a trailing-window average (not a regression)
// and every degenerate case is explicit in Status. — ADR-0019.

// forecastWindowDays is the trailing window the burn rate averages over. A week
// smooths weekday/weekend noise while still tracking a genuine change in pace.
const forecastWindowDays = 7

// ProjectionStatus classifies a Forecast so the UI picks the right message
// without re-deriving the conditions. Read it before the numeric fields.
type ProjectionStatus int

const (
	// ProjectionNoBudget: no monthly allowance is configured, so there is no
	// ceiling to project toward.
	ProjectionNoBudget ProjectionStatus = iota
	// ProjectionIdle: no spend in the trailing window, so the allowance is never
	// reached at the current (zero) rate.
	ProjectionIdle
	// ProjectionExhausted: month-to-date spend already meets or exceeds the
	// allowance — the budget is gone.
	ProjectionExhausted
	// ProjectionOK: a positive burn rate and remaining allowance yield a finite
	// projection (the numeric fields are meaningful).
	ProjectionOK
)

// Projection is a deterministic burn-rate forecast: at the recent daily spend
// rate, when the monthly allowance is exhausted. The rate/days/turns/date fields
// are meaningful only when Status == ProjectionOK; UsedCredits/RemainingCredits
// are also set for ProjectionIdle so the UI can explain "no recent spend".
type Projection struct {
	Status ProjectionStatus
	// DailyRate is the trailing-window average spend, credits per day.
	DailyRate float64
	// PerTurnRate is the trailing-window average spend, credits per turn.
	PerTurnRate float64
	// UsedCredits is month-to-date spend (credits) within now's UTC month.
	UsedCredits float64
	// RemainingCredits is allowance minus month-to-date (zero once exhausted).
	RemainingCredits float64
	// DaysToCap is the projected number of days until the allowance is exhausted.
	DaysToCap float64
	// TurnsToCap is the projected number of turns until exhaustion at PerTurnRate.
	TurnsToCap float64
	// ExhaustionDate is the projected UTC day the allowance runs out (now's day
	// plus ⌈DaysToCap⌉). Zero unless Status is ProjectionOK or ProjectionExhausted.
	ExhaustionDate time.Time
}

// Forecast projects when the monthly allowance is exhausted from the daily spend
// series and the budget allowance. Pure: same (daily, budget, now) → same
// Projection, no IO and no clock beyond now.
//
// The burn rate is a trailing-N-day average (ADR-0019): sum the credits spent in
// the last forecastWindowDays UTC days and divide by the elapsed days actually
// observed — the window length, clamped to how long the ledger has existed. So a
// brand-new or single-day ledger isn't divided by a mostly-absent week (which
// would understate the rate to near-zero), while idle days inside an established
// history correctly drag the average down. Month-to-date "used" sums the whole
// current UTC month (independent of the trailing window), matching MonthToDate /
// ADR-0016. Degenerate cases are explicit in Status.
func Forecast(daily []DayTotal, budget Budget, now time.Time) Projection {
	var p Projection
	if budget.AllowanceCredits <= 0 {
		p.Status = ProjectionNoBudget
		return p
	}

	today := now.UTC().Truncate(24 * time.Hour)
	monthPrefix := today.Format("2006-01")
	windowKey := today.AddDate(0, 0, -(forecastWindowDays - 1)).Format("2006-01-02")

	var monthCredits, windowCredits float64
	var windowTurns int
	firstEver := "" // earliest day key in the whole series
	for _, d := range daily {
		if strings.HasPrefix(d.Day, monthPrefix) {
			monthCredits += d.Credits
		}
		if d.Day >= windowKey {
			windowCredits += d.Credits
			windowTurns += d.Count
		}
		if firstEver == "" || d.Day < firstEver {
			firstEver = d.Day
		}
	}

	p.UsedCredits = monthCredits
	remaining := budget.AllowanceCredits - monthCredits
	if remaining <= 0 {
		p.Status = ProjectionExhausted
		p.ExhaustionDate = today
		return p
	}
	p.RemainingCredits = remaining

	if windowCredits <= 0 {
		p.Status = ProjectionIdle
		return p
	}

	// Elapsed observed days: the window, clamped so a history shorter than the
	// window isn't divided by mostly-absent days. Idle days *inside* an
	// established history (firstEver predates the window) are intentionally kept.
	spanStart := today.AddDate(0, 0, -(forecastWindowDays - 1))
	if firstEver > windowKey {
		if t, err := time.Parse("2006-01-02", firstEver); err == nil {
			spanStart = t
		}
	}
	spanDays := int(today.Sub(spanStart).Hours()/24) + 1
	if spanDays < 1 {
		spanDays = 1
	}

	p.DailyRate = windowCredits / float64(spanDays)
	if windowTurns > 0 {
		p.PerTurnRate = windowCredits / float64(windowTurns)
	}
	p.DaysToCap = remaining / p.DailyRate
	if p.PerTurnRate > 0 {
		p.TurnsToCap = remaining / p.PerTurnRate
	}
	p.ExhaustionDate = today.AddDate(0, 0, int(math.Ceil(p.DaysToCap)))
	p.Status = ProjectionOK
	return p
}
