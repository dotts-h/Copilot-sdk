package web

import (
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

func (s *Server) telemetryPartial(window int) string {
	// Account-wide budget accounting reads month-to-date from the persisted ledger
	// so "remaining this month" survives a restart (ADR-0016). The live token split
	// and per-model table below stay on the in-process meter — one source per
	// surface (see REGRESSIONS "two meters", now three sources).
	month := s.monthToDate()
	in, cached, out := s.meter.TotalTokens()
	budget := s.budget()
	frac := budget.FractionUsed(month.Credits())

	rows := [][2]string{
		{"Total cost", fmt.Sprintf("%s (%s)", telemetry.FormatCredits(month.Credits()), telemetry.FormatUSD(month.USD()))},
		{"Monthly budget", fmt.Sprintf("%.2f of %.0f cr", month.Credits(), budget.AllowanceCredits)},
		{"Remaining", telemetry.FormatCredits(budget.Remaining(month.Credits()))},
	}
	if s.hardCap > 0 {
		rows = append(rows, [2]string{"Hard cap", fmt.Sprintf("%.0f cr — a turn projected to exceed it pauses for confirmation", s.hardCap)})
	}
	if aiu := s.meter.ReportedAIU(); aiu > 0 {
		rows = append(rows, [2]string{"GitHub-reported cost", fmt.Sprintf("%.4f AIU", aiu)})
	}
	rows = append(rows, [2]string{"Tokens (this session)", fmt.Sprintf("input %d · cached %d · output %d", in, cached, out)})

	pct := frac * 100
	if pct > 100 {
		pct = 100
	}
	// The per-model token table reads the PERSISTED LEDGER (all-time, restart-
	// surviving), not the live in-process meter — which is empty until turns replay
	// through it, so the table read 0/0/0 next to real spend (epic 0050 finding 2 /
	// issue 0058). The ledger's SpendRecords already carry the per-turn token
	// counts; ModelBreakdowns aggregates them. The live meter remains the "this
	// session" source for the Tokens row above.
	models := make([]map[string]any, 0)
	if s.spend != nil {
		for _, r := range telemetry.ModelBreakdowns(s.spend.Records()) {
			models = append(models, map[string]any{
				"Model": r.Model, "In": r.InputTokens, "Cached": r.CachedTokens, "Out": r.OutputTokens,
				"CacheWrite": r.CacheWriteTokens, "Reasoning": r.ReasoningTokens,
				"Credits": fmt.Sprintf("%.2f", r.Credits()), "USD": telemetry.FormatUSD(r.USD()),
			})
		}
	}
	days, shares, hasHistory := s.spendTrend(window)
	now := time.Now()
	agents, workflows := s.spendShares(now)
	var forecast map[string]any
	if fc, ok := s.forecast(now); ok {
		forecast = forecastView(fc, budget.AllowanceCredits, now)
	}
	windows := make([]map[string]any, 0, len(spendWindows))
	for _, w := range spendWindows {
		windows = append(windows, map[string]any{"Value": w, "Active": w == window})
	}
	return frag("telemetryPage", map[string]any{
		"Rows": rows, "Width": fmt.Sprintf("%.1f%%", pct), "Models": models,
		"Dashboard": s.dashboardView(window, now),
		"Days":      days, "Shares": shares, "HasHistory": hasHistory,
		"AgentShares": agents, "WorkflowShares": workflows,
		"Reconcile":     s.workflowReconcile(),
		"LaneReconcile": s.laneReconcile(),
		"Drift":         s.estimateDrift(),
		"Forecast":      forecast, "Windows": windows,
	})
}

// spendTrend builds the persisted-ledger trend data for the Telemetry page: the
// per-day spend over the chosen window (most recent last, each bar scaled to the
// busiest day in view) and each model's share of all-time spend. The window (days)
// is the selector value, clamped upstream to spendWindows. Empty when no ledger is
// wired or none yet.
func (s *Server) spendTrend(window int) (days, shares []map[string]any, hasHistory bool) {
	days, shares = []map[string]any{}, []map[string]any{}
	if s.spend == nil {
		return days, shares, false
	}
	records := s.spend.Records()
	if len(records) == 0 {
		return days, shares, false
	}

	daily := telemetry.DailyTotals(records)
	// Slice to the chosen window FIRST, so a long history stays scannable and the
	// max below is computed over what's shown — never over full history (the
	// REGRESSIONS #14 invariant: an off-window peak must not shrink visible bars).
	if window > 0 && len(daily) > window {
		daily = daily[len(daily)-window:]
	}
	// Scale bars to the busiest day *in view*, so the visible window always uses
	// the full width even when an off-screen older day spent more.
	var maxUSD float64
	for _, d := range daily {
		if d.USD > maxUSD {
			maxUSD = d.USD
		}
	}
	for _, d := range daily {
		w := 0.0
		if maxUSD > 0 {
			w = d.USD / maxUSD * 100
		}
		days = append(days, map[string]any{
			"Day": d.Day, "Credits": fmt.Sprintf("%.2f", d.Credits),
			"USD": telemetry.FormatUSD(d.USD), "Turns": d.Count,
			"Width": fmt.Sprintf("%.1f%%", w),
		})
	}

	for _, m := range telemetry.ModelShares(records) {
		shares = append(shares, map[string]any{
			"Model": m.Model, "Credits": fmt.Sprintf("%.2f", m.Credits),
			"Pct": fmt.Sprintf("%.0f", m.Fraction*100), "Width": fmt.Sprintf("%.1f%%", m.Fraction*100),
		})
	}
	return days, shares, true
}

// spendShares builds the per-agent and per-workflow cost breakdowns for the
// Telemetry page from the persisted ledger — the orchestration-aware "which agent
// / which workflow burned my budget" view (ADR-0018) — each row joined to its burn
// TRAJECTORY (F3): the same per-bucket Forecast slope that AgentShares/WorkflowShares
// can't answer. Agent/workflow ids are resolved to display names under forgeMu
// (falling back to the raw id when a persona was renamed or deleted; an empty agent
// id is the built-in chat). The trajectory is keyed by the raw id (BucketForecasts'
// key), joined before the id is resolved to a label. One `now` is threaded through
// both the per-bucket Forecast and the month projection (the ADR-0019 single-`now`
// gotcha, per bucket). Both lists are empty when no ledger is wired or it holds no
// relevant records.
func (s *Server) spendShares(now time.Time) (agents, workflows []map[string]any) {
	agents, workflows = []map[string]any{}, []map[string]any{}
	if s.spend == nil {
		return agents, workflows
	}
	records := s.spend.Records()
	if len(records) == 0 {
		return agents, workflows
	}
	budget := s.budget()
	agentTraj := bucketTrajectories(telemetry.BucketForecasts(records, budget, now, agentKey, true), now)
	workflowTraj := bucketTrajectories(telemetry.BucketForecasts(records, budget, now, workflowKey, false), now)
	s.hub.forgeMu.Lock()
	defer s.hub.forgeMu.Unlock()
	for _, a := range telemetry.AgentShares(records) {
		row := shareRow(s.agentLabel(a.AgentID), a.Credits, a.Fraction)
		row["Traj"] = agentTraj[a.AgentID]
		agents = append(agents, row)
	}
	for _, w := range telemetry.WorkflowShares(records) {
		row := shareRow(s.workflowLabel(w.WorkflowID), w.Credits, w.Fraction)
		row["Traj"] = workflowTraj[w.WorkflowID]
		workflows = append(workflows, row)
	}
	return agents, workflows
}

// agentKey / workflowKey are the bucket keys BucketForecasts projects per (the same
// keyOf the *Shares readers bucket by), so the trajectory join lines up with the
// share rows exactly.
func agentKey(r telemetry.SpendRecord) string { return r.AgentID }

func workflowKey(r telemetry.SpendRecord) string { return r.WorkflowID }

// reconcileEpsilon is the smallest delta the reconciliation treats as a real
// discrepancy: a delta whose magnitude is below it rounds to "0.00 cr" at the
// two-decimal display width, so it's floating-point noise from summing two
// independent measurements, not a divergence worth ambering. Tied to the display
// width so the amber flag matches what the cell shows.
const reconcileEpsilon = 0.005

// workflowReconcile builds the per-workflow "ledger vs runs" reconciliation rows for
// the Telemetry page (V15): each workflow's spend as the ledger attributes it
// (WorkflowShares grain) beside what its recorded runs metered (RunAggregates grain),
// and the delta between them — so orchestrated spend is reconcilable across the two
// persisted stores, not just accountable on each. A non-trivial delta is ambered (a
// turn metered outside a recorded run, or a run metered under a different
// attribution). Workflow ids resolve to display names under forgeMu. Empty unless
// BOTH a spend store and a run store are wired (reconciliation needs two sides) and
// at least one workflow appears in either.
func (s *Server) workflowReconcile() []map[string]any {
	rows := []map[string]any{}
	if s.spend == nil || s.runs == nil {
		return rows
	}
	recon := telemetry.WorkflowReconcile(s.spend.Records(), s.runs.Records())
	if len(recon) == 0 {
		return rows
	}
	s.hub.forgeMu.Lock()
	defer s.hub.forgeMu.Unlock()
	for _, r := range recon {
		rows = append(rows, s.reconcileRow(r))
	}
	return rows
}

// reconcileRow builds the template shape for one "Ledger vs runs" comparison row: the
// workflow's display name, its ledger credits, its recorded-run credits, and the
// delta (ambered when its magnitude clears reconcileEpsilon — i.e. it displays as a
// non-zero figure). Workflow ids resolve via workflowLabel — caller holds forgeMu.
func (s *Server) reconcileRow(r telemetry.WorkflowRecon) map[string]any {
	return map[string]any{
		"Workflow":      s.workflowLabel(r.WorkflowID),
		"LedgerCredits": telemetry.FormatCredits(r.LedgerCredits),
		"RunCredits":    telemetry.FormatCredits(r.RunCredits),
		"Delta":         telemetry.FormatCredits(r.Delta),
		"Amber":         math.Abs(r.Delta) >= reconcileEpsilon,
	}
}

// laneReconcile builds the per-lane "Ledger vs runs by lane" rows for the Telemetry page
// (V16) — the per-workflow reconciliation one grain finer: each (workflow, lane)'s ledger
// spend (lane-tagged SpendRecords) beside what its recorded run lane metered, and the
// delta — so a divergence the per-workflow row only totals is locatable at the exact step.
// A non-trivial delta is ambered. Workflow ids resolve to display names under forgeMu.
// Empty unless BOTH a spend store and a run store are wired (reconciliation needs two
// sides) and at least one lane appears with non-zero credits in either.
func (s *Server) laneReconcile() []map[string]any {
	rows := []map[string]any{}
	if s.spend == nil || s.runs == nil {
		return rows
	}
	recon := telemetry.LaneReconcile(s.spend.Records(), s.runs.Records())
	if len(recon) == 0 {
		return rows
	}
	s.hub.forgeMu.Lock()
	defer s.hub.forgeMu.Unlock()
	for _, r := range recon {
		rows = append(rows, s.laneReconcileRow(r))
	}
	return rows
}

// laneReconcileRow builds the template shape for one "Ledger vs runs by lane" row: the
// lane named "<workflow> · step <n>" (n = lane index + 1, mirroring the Runs page's "Cost
// by lane"), its ledger credits, its recorded-run credits, and the delta (ambered when its
// magnitude clears reconcileEpsilon). Workflow ids resolve via workflowLabel — caller holds
// forgeMu.
func (s *Server) laneReconcileRow(r telemetry.LaneRecon) map[string]any {
	return map[string]any{
		"Lane":          fmt.Sprintf("%s · step %d", s.workflowLabel(r.WorkflowID), r.LaneIndex+1),
		"LedgerCredits": telemetry.FormatCredits(r.LedgerCredits),
		"RunCredits":    telemetry.FormatCredits(r.RunCredits),
		"Delta":         telemetry.FormatCredits(r.Delta),
		"Amber":         math.Abs(r.Delta) >= reconcileEpsilon,
	}
}

// estimateDrift builds the per-model "Estimate vs reported" drift rows for the
// Telemetry page (issue 0060) — the cost cousin of the ledger⋈runs reconciliation,
// joining the price-book estimate to GitHub's reported cost over the ledger's
// REPORTED turns (ModelDrifts). The delta is ambered when its magnitude clears
// reconcileEpsilon (it displays as a non-zero figure); the coverage cell counts the
// turns joined and the est-only turns the comparison can't cover. Empty when no
// ledger is wired or no turn was ever reported (nothing to compare).
func (s *Server) estimateDrift() []map[string]any {
	rows := []map[string]any{}
	if s.spend == nil {
		return rows
	}
	for _, d := range telemetry.ModelDrifts(s.spend.Records()) {
		coverage := fmt.Sprintf("%d reported", d.ReportedTurns)
		if d.UnreportedTurns > 0 {
			coverage += fmt.Sprintf(" · %d est-only", d.UnreportedTurns)
		}
		rows = append(rows, map[string]any{
			"Model":    d.Model,
			"Estimate": telemetry.FormatCredits(d.EstimateCredits),
			"Reported": telemetry.FormatCredits(d.ReportedCredits),
			"Delta":    telemetry.FormatCredits(d.Delta),
			"Amber":    d.Drifted(reconcileEpsilon),
			"Coverage": coverage,
		})
	}
	return rows
}

// bucketTrajectories renders each bucket's burn-trajectory sentence keyed by the
// bucket's raw id, so spendShares can join it onto the matching share row before
// the id is resolved to a display label. An empty string (no-budget bucket) joins
// as no trajectory cell.
func bucketTrajectories(bs []telemetry.BucketProjection, now time.Time) map[string]string {
	out := make(map[string]string, len(bs))
	for _, b := range bs {
		out[b.Key] = bucketTrajectoryText(b.Projection, now)
	}
	return out
}

// bucketTrajectoryText turns a per-bucket Projection into its trajectory sentence
// (F3). The honest per-bucket framing is rate + month projection, NOT a per-bucket
// exhaustion date: a bucket has no own allowance, so the account-wide DaysToCap/
// ExhaustionDate fields are deliberately not surfaced. Each degenerate Status gets
// its own sentence (or none) rather than a bogus figure, mirroring forecastView:
//   - OK: the bucket's recent rate + where it lands this month at that pace;
//   - Idle: no recent spend to project from;
//   - NoBudget / Exhausted: empty — no per-bucket trajectory line.
func bucketTrajectoryText(p telemetry.Projection, now time.Time) string {
	switch p.Status {
	case telemetry.ProjectionOK:
		// Project where this bucket lands by month-end at its recent pace: what it has
		// already spent this month plus the rate over the days still to come. No
		// per-bucket cap — just the trajectory.
		month := p.UsedCredits + p.DailyRate*float64(daysLeftInMonth(now))
		return fmt.Sprintf("at ~%s/day, on pace for ~%s this month",
			telemetry.FormatCredits(p.DailyRate), telemetry.FormatCredits(month))
	case telemetry.ProjectionIdle:
		return "idle — no recent spend to project a rate from"
	default: // ProjectionNoBudget, ProjectionExhausted — no per-bucket trajectory line
		return ""
	}
}

// daysLeftInMonth counts the whole UTC days remaining after now's day through the
// end of its calendar month (zero on the last day) — the horizon the per-bucket
// month projection extrapolates the rate over. Pure: same now → same count.
func daysLeftInMonth(now time.Time) int {
	t := now.UTC()
	y, m, d := t.Date()
	firstNextMonth := time.Date(y, m+1, 1, 0, 0, 0, 0, time.UTC)
	daysInMonth := firstNextMonth.AddDate(0, 0, -1).Day()
	return daysInMonth - d
}

// shareRow is the template shape shared by every spend-breakdown bar (per-model,
// per-agent, per-workflow): a label, a credit total, and a fraction rendered as a
// percentage and a bar width.
func shareRow(label string, credits, fraction float64) map[string]any {
	return map[string]any{
		"Label": label, "Credits": fmt.Sprintf("%.2f", credits),
		"Pct": fmt.Sprintf("%.0f", fraction*100), "Width": fmt.Sprintf("%.1f%%", fraction*100),
	}
}

// forecastView turns a burn-rate Projection into the Telemetry-page line (A3 /
// ADR-0019): a human sentence plus a Warn flag that ambers the line when the
// projected exhaustion falls within the current month (on track to blow this
// month's budget). Each degenerate Status gets its own explanatory sentence
// rather than a bogus date.
func forecastView(fc telemetry.Projection, allowance float64, now time.Time) map[string]any {
	switch fc.Status {
	case telemetry.ProjectionNoBudget:
		return map[string]any{"Text": "Set a monthly budget to see a burn-rate forecast.", "Warn": false}
	case telemetry.ProjectionIdle:
		return map[string]any{"Text": "No recent spend to project a burn rate from.", "Warn": false}
	case telemetry.ProjectionExhausted:
		return map[string]any{"Text": "This month's budget is already exhausted.", "Warn": true}
	default:
		// Ceil so the "~N days" count matches ExhaustionDate (= today + ⌈DaysToCap⌉);
		// rounding the count independently could print "~9 days" beside a +10-day date.
		days := int(math.Ceil(fc.DaysToCap))
		turns := int(math.Round(fc.TurnsToCap))
		text := fmt.Sprintf("At ~%s/day, your %.0f cr budget runs out around %s (~%d %s, ~%d %s).",
			telemetry.FormatCredits(fc.DailyRate), allowance,
			fc.ExhaustionDate.UTC().Format("2006-01-02"),
			days, plural(days, "day", "days"), turns, plural(turns, "turn", "turns"))
		return map[string]any{"Text": text, "Warn": forecastSoon(fc.ExhaustionDate, now)}
	}
}

// forecastSoon reports whether a projected exhaustion date falls on or before the
// last instant of now's UTC month — i.e. the burn rate is on track to spend the
// monthly allowance before it resets. Pure: same inputs → same answer.
func forecastSoon(exhaust, now time.Time) bool {
	if exhaust.IsZero() {
		return false
	}
	y, m, _ := now.UTC().Date()
	endOfMonth := time.Date(y, m+1, 1, 0, 0, 0, 0, time.UTC).Add(-time.Nanosecond)
	return !exhaust.After(endOfMonth)
}

// plural picks the singular or plural noun for n.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// handleSpendExport streams the full persisted spend ledger as a CSV download,
// so spend can be analysed outside the app (the trend view's accountable-ledger
// promise). Empty (header only) when no ledger is wired.
func (s *Server) handleSpendExport(w http.ResponseWriter, r *http.Request) {
	var records []telemetry.SpendRecord
	if s.spend != nil {
		records = s.spend.Records()
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="my-orchestra-spend.csv"`)
	if err := telemetry.WriteCSV(w, records); err != nil {
		s.logger.Printf("export spend csv: %v", err)
	}
}

// handleReconcileExport streams the cross-store reconciliation as a CSV download (V17) —
// the convergence's export sibling of handleSpendExport/handleRunsExport — so the ledger-
// vs-runs divergence the Telemetry page surfaces can be analysed outside the app. One file
// carries both grains (per-workflow then per-(workflow, lane)); empty (header only) when no
// run store is wired or there is nothing to reconcile.
func (s *Server) handleReconcileExport(w http.ResponseWriter, r *http.Request) {
	var spend []telemetry.SpendRecord
	if s.spend != nil {
		spend = s.spend.Records()
	}
	var runs []telemetry.RunRecord
	if s.runs != nil {
		runs = s.runs.Records()
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="my-orchestra-reconcile.csv"`)
	if err := telemetry.WriteReconcileCSV(w, spend, runs); err != nil {
		s.logger.Printf("export reconcile csv: %v", err)
	}
}
