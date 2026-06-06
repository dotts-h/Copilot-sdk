package telemetry

import (
	"testing"
	"time"
)

// dayTotal builds a DayTotal the way DailyTotals would, with Credits derived from
// USD, so a test can express spend in either unit and trust both fields agree.
func dayTotal(d string, credits float64, turns int) DayTotal {
	return DayTotal{Day: d, USD: credits * USDPerCredit, Credits: credits, Count: turns}
}

func TestForecast(t *testing.T) {
	now := day("2026-06-15T12:00:00Z")
	const allowance = 100.0

	tests := []struct {
		name       string
		daily      []DayTotal
		budget     Budget
		wantStatus ProjectionStatus
		// the fields below are checked only when wantStatus == ProjectionOK
		wantDaily   float64
		wantPerTurn float64
		wantUsed    float64
		wantRemain  float64
		wantDays    float64
		wantTurns   float64
		wantExhaust string // YYYY-MM-DD
	}{
		{
			name:       "no allowance configured cannot project",
			daily:      []DayTotal{dayTotal("2026-06-15", 5, 2)},
			budget:     Budget{AllowanceCredits: 0},
			wantStatus: ProjectionNoBudget,
		},
		{
			name:       "empty ledger is idle",
			daily:      nil,
			budget:     Budget{AllowanceCredits: allowance},
			wantStatus: ProjectionIdle,
		},
		{
			name:       "spend only older than the window is idle",
			daily:      []DayTotal{dayTotal("2026-06-01", 50, 10)},
			budget:     Budget{AllowanceCredits: allowance},
			wantStatus: ProjectionIdle,
		},
		{
			name:       "month-to-date already at the allowance is exhausted",
			daily:      sevenFlatDays(5, 2), // 7×5 = 35 cr this month
			budget:     Budget{AllowanceCredits: 20},
			wantStatus: ProjectionExhausted,
		},
		{
			name:        "flat week projects a steady rate",
			daily:       sevenFlatDays(5, 2), // 09..15, 5 cr & 2 turns each
			budget:      Budget{AllowanceCredits: allowance},
			wantStatus:  ProjectionOK,
			wantDaily:   5,   // 35 cr / 7 days
			wantPerTurn: 2.5, // 35 cr / 14 turns
			wantUsed:    35,  // all in June
			wantRemain:  65,  // 100 - 35
			wantDays:    13,  // 65 / 5
			wantTurns:   26,  // 65 / 2.5
			wantExhaust: "2026-06-28",
		},
		{
			name:        "single-day series still projects from the one day",
			daily:       []DayTotal{dayTotal("2026-06-15", 10, 4)},
			budget:      Budget{AllowanceCredits: allowance},
			wantStatus:  ProjectionOK,
			wantDaily:   10,  // 10 cr / 1 observed day
			wantPerTurn: 2.5, // 10 cr / 4 turns
			wantUsed:    10,
			wantRemain:  90,
			wantDays:    9, // 90 / 10
			wantTurns:   36,
			wantExhaust: "2026-06-24",
		},
		{
			name: "older in-month spend counts toward used but idle days inside the window drag the rate down",
			daily: []DayTotal{
				dayTotal("2026-06-08", 20, 5), // in month, before the 7-day window
				dayTotal("2026-06-15", 7, 7),  // in window
			},
			budget:      Budget{AllowanceCredits: allowance},
			wantStatus:  ProjectionOK,
			wantDaily:   1,  // 7 cr / 7 elapsed days (08 predates the window → idle 09..14 count)
			wantPerTurn: 1,  // 7 cr / 7 turns
			wantUsed:    27, // 20 + 7, both in June
			wantRemain:  73,
			wantDays:    73,
			wantTurns:   73,
			wantExhaust: "2026-08-27",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := Forecast(tc.daily, tc.budget, now)
			if p.Status != tc.wantStatus {
				t.Fatalf("status = %v, want %v (%+v)", p.Status, tc.wantStatus, p)
			}
			if tc.wantStatus != ProjectionOK {
				return
			}
			approx(t, p.DailyRate, tc.wantDaily)
			approx(t, p.PerTurnRate, tc.wantPerTurn)
			approx(t, p.UsedCredits, tc.wantUsed)
			approx(t, p.RemainingCredits, tc.wantRemain)
			approx(t, p.DaysToCap, tc.wantDays)
			approx(t, p.TurnsToCap, tc.wantTurns)
			if got := p.ExhaustionDate.UTC().Format("2006-01-02"); got != tc.wantExhaust {
				t.Errorf("exhaustion date = %s, want %s", got, tc.wantExhaust)
			}
		})
	}
}

// TestForecastDeterministic guards the package-wide purity invariant: identical
// inputs always yield an identical projection.
func TestForecastDeterministic(t *testing.T) {
	now := day("2026-06-15T12:00:00Z")
	daily := sevenFlatDays(5, 2)
	b := Budget{AllowanceCredits: 100}
	a := Forecast(daily, b, now)
	c := Forecast(daily, b, now)
	if a != c {
		t.Fatalf("Forecast is not deterministic: %+v vs %+v", a, c)
	}
}

// sevenFlatDays builds 2026-06-09..2026-06-15 (the trailing 7-day window ending
// on the 15th), each day spending the same credits over the same turn count.
func sevenFlatDays(credits float64, turns int) []DayTotal {
	out := make([]DayTotal, 0, 7)
	for d := 9; d <= 15; d++ {
		out = append(out, dayTotal(time.Date(2026, 6, d, 0, 0, 0, 0, time.UTC).Format("2006-01-02"), credits, turns))
	}
	return out
}
