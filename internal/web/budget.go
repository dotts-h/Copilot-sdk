package web

import (
	"time"

	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// budgetTracker is the session's account-wide cost-accounting module: it owns the
// budget knobs (allowance, warn fraction, hard cap) and answers every budget question
// off the PERSISTED ledger (ADR-0016) behind a narrow interface, so callers never
// reach into the meter/ledger/config to do accounting themselves. It is independent
// of the web layer (no Server/forge deps) and unit-tested in isolation; the gate
// lifecycle (budgetGate/handleBudget) and fragment rendering stay on Server. The
// knobs are a cache refreshed from config on a settings save (refreshBudget); spend
// is the persisted ledger (nil in the offline demo / a test without one).
type budgetTracker struct {
	allowance    float64
	warnFraction float64
	hardCap      float64
	spend        *telemetry.SpendStore
}

// Budget is the telemetry budget built from this session's cached knobs.
func (b budgetTracker) Budget() telemetry.Budget {
	return telemetry.Budget{
		AllowanceCredits: b.allowance,
		WarnFraction:     b.warnFraction,
		HardCapCredits:   b.hardCap,
	}
}

// MonthToDate is the account-wide spend recorded within now's UTC calendar month,
// read off the persisted ledger so the budget rows, the cost footer, and the hard-cap
// baseline survive a restart (ADR-0016). Zero when no ledger is wired. Distinct from
// the per-session statusline (sessionMeter) and the live token split (process meter):
// one source per surface (REGRESSIONS "two meters").
func (b budgetTracker) MonthToDate(now time.Time) telemetry.Cost {
	if b.spend == nil {
		return telemetry.Cost{}
	}
	return telemetry.MonthToDate(b.spend.Records(), now)
}

// Forecast projects when the monthly allowance is exhausted at the recent burn rate,
// account-wide off the same ledger (a pure telemetry.Forecast over the daily series —
// ADR-0019). ok is false when no ledger is wired or it holds no records, so callers
// can hide the surface entirely (distinct from a wired-but-degenerate projection,
// which Projection.Status carries).
func (b budgetTracker) Forecast(now time.Time) (telemetry.Projection, bool) {
	if b.spend == nil {
		return telemetry.Projection{}, false
	}
	records := b.spend.Records()
	if len(records) == 0 {
		return telemetry.Projection{}, false
	}
	return telemetry.Forecast(telemetry.DailyTotals(records), b.Budget(), now), true
}

// budgets snapshots this session's cached knobs + ledger into a budgetTracker. The
// knobs are read without locking, exactly as the prior inline accessors did (a turn
// path already holds s.mu; a render path tolerates a benign stale read).
func (s *Server) budgets() budgetTracker {
	return budgetTracker{
		allowance:    s.allowance,
		warnFraction: s.warnFraction,
		hardCap:      s.hardCap,
		spend:        s.spend,
	}
}

// budget builds the telemetry budget from this session's cached knobs. Kept as a
// Server method (a thin delegator to budgetTracker) so its many call sites — the cost
// footer, the Telemetry rows, the cap check — are unchanged.
func (s *Server) budget() telemetry.Budget { return s.budgets().Budget() }

// monthToDate returns the account-wide spend in the current UTC month off the ledger.
func (s *Server) monthToDate() telemetry.Cost { return s.budgets().MonthToDate(time.Now()) }

// forecast projects allowance exhaustion at the recent burn rate (account-wide).
func (s *Server) forecast(now time.Time) (telemetry.Projection, bool) {
	return s.budgets().Forecast(now)
}

// overCap reports the projected credit spend of the next turn (persisted month-to-date
// plus the live pre-flight estimate of resending the context) and whether it would
// breach the hard cap. The "used" baseline is the persisted month-to-date, not the
// in-process meter, so a mid-month restart doesn't silently re-open a cap the user
// already approached (ADR-0016); the next-turn estimate still comes from the live
// meter. Caller must hold s.mu.
func (s *Server) overCap() (projected float64, capped bool) {
	bt := s.budgets()
	b := bt.Budget()
	if b.HardCapCredits <= 0 {
		return 0, false
	}
	projected = bt.MonthToDate(time.Now()).Credits() + s.meter.EstimateTurn(s.spec.Model, s.ctxCurrent).Credits()
	return projected, b.CapExceeded(projected)
}
