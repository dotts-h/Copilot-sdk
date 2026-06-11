package web

import (
	"fmt"
	"strconv"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// This file renders the Runs page: the persisted workflow-run history (ADR-0022). A
// run is the product's unit of orchestration, recorded once on completion by
// workflow.go (recordRun). The page is a read-only view over telemetry.RunStore,
// resolving agent ids to display names like the Telemetry cost breakdowns.

// runsPartial renders the Runs page: a per-workflow summary table (run count,
// failure rate, total & average cost, average duration — the cost ⋈ orchestration
// roll-up, ADR-0022/V1) above the persisted run history, most recent first. Each run shows
// its name, mode, outcome, when it ran, how long it took, total metered cost, and a
// per-lane breakdown (agent, status glyph, credits). Both the run rows and the
// summary resolve ids to display names under forgeMu, like the cost breakdowns.
//
// The history is first sliced to the chosen 14/30/90-day window (V12) — the
// orchestration analog of the Telemetry trend's window selector — so a long history
// stays scannable; the window is clamped upstream (clampWindow) to spendWindows. The
// slice happens BEFORE both the summary roll-up and the history list, so an out-of-window
// run is dropped from both. Empty when no run store is wired or none have run yet.
func (s *Server) runsPartial(window int) string {
	rows := []map[string]any{}
	summary := []map[string]any{}
	laneShares := []map[string]any{}
	if s.runs != nil {
		records := windowRuns(s.runs.Records(), window)
		s.hub.forgeMu.Lock()
		for i := len(records) - 1; i >= 0; i-- { // newest first
			rows = append(rows, s.runRow(records[i], window))
		}
		for _, a := range telemetry.RunAggregates(records) {
			summary = append(summary, s.runSummaryRow(a))
		}
		for _, l := range telemetry.LaneShares(records) {
			laneShares = append(laneShares, s.laneShareRow(l))
		}
		s.hub.forgeMu.Unlock()
	}
	windows := make([]map[string]any, 0, len(spendWindows))
	for _, w := range spendWindows {
		windows = append(windows, map[string]any{"Value": w, "Active": w == window})
	}
	return frag("runsPage", map[string]any{
		"Rows": rows, "Summary": summary, "LaneShares": laneShares, "Windows": windows,
	})
}

// windowRuns slices a run history to the records started within `window` days of the
// most recent run — the Runs-page time-window selector (V12). The cutoff is anchored to
// the latest StartedAt rather than wall-clock now, so the slice is deterministic and a
// long-idle history still shows its most recent window — mirroring spendTrend's
// tail-relative day slice. A non-positive window, an empty history, or a history with no
// usable timestamp (all-zero StartedAt) returns the records unchanged. Pure: same inputs
// → same slice.
func windowRuns(records []telemetry.RunRecord, window int) []telemetry.RunRecord {
	if window <= 0 || len(records) == 0 {
		return records
	}
	var latest time.Time
	for _, r := range records {
		if r.StartedAt.After(latest) {
			latest = r.StartedAt
		}
	}
	if latest.IsZero() {
		return records
	}
	cutoff := latest.AddDate(0, 0, -window)
	out := make([]telemetry.RunRecord, 0, len(records))
	for _, r := range records {
		if !r.StartedAt.Before(cutoff) {
			out = append(out, r)
		}
	}
	return out
}

// runSummaryRow builds the template shape for one per-workflow summary row: the
// workflow's display name, its run/failure counts, failure rate as a percentage,
// and its total & average cost and duration. Total credits — the workflow's
// cumulative orchestrated spend — reads beside the average so a high run count and a
// high per-run cost are distinguishable (V13). Workflow ids resolve to names via
// workflowLabel — caller holds forgeMu.
func (s *Server) runSummaryRow(a telemetry.RunAggregate) map[string]any {
	return map[string]any{
		"Workflow":     s.workflowLabel(a.WorkflowID),
		"Runs":         a.Runs,
		"Failures":     a.Failures,
		"FailPct":      fmt.Sprintf("%.0f", a.FailureRate()*100),
		"HasFailures":  a.Failures > 0,
		"TotalCredits": telemetry.FormatCredits(a.TotalCredits),
		"AvgCredits":   telemetry.FormatCredits(a.AvgCredits),
		"AvgDuration":  humanDuration(a.AvgDuration),
	}
}

// laneShareRow builds the template shape for one "Cost by lane" breakdown row (V14):
// the finest orchestration-attribution grain — which lane in a workflow costs / fails
// most. The lane is named by its workflow, its step (lane index + 1), and its agent;
// the workflow and agent ids resolve to display names via workflowLabel/agentLabel —
// caller holds forgeMu. Pct/Width render the lane's credit fraction as a percentage
// and a meter bar, mirroring the Telemetry cost-share rows.
func (s *Server) laneShareRow(l telemetry.LaneShare) map[string]any {
	return map[string]any{
		"Workflow":    s.workflowLabel(l.WorkflowID),
		"Step":        l.LaneIndex + 1,
		"Agent":       s.agentLabel(l.AgentID),
		"Failures":    l.Failures,
		"HasFailures": l.Failures > 0,
		"Credits":     fmt.Sprintf("%.2f", l.Credits),
		"Pct":         fmt.Sprintf("%.0f", l.Fraction*100),
		"Width":       fmt.Sprintf("%.1f%%", l.Fraction*100),
	}
}

// runRow builds the template shape for one persisted run: its header (name, mode,
// outcome glyph, when, total cost), a per-lane breakdown, and a "Rerun" control. Agent
// ids resolve to display names via agentLabel — caller holds forgeMu.
//
// CanRerun gates the rerun button on the run's workflow still existing in the forge
// (ADR-0023): an orphan run — one whose workflow was renamed or deleted since — has
// nothing to re-execute, so it shows no control. The active window rides along so the
// button's POST carries it, preserving the selection if the rerun is refused (a
// just-deleted workflow or a busy server) and the Runs page re-renders.
func (s *Server) runRow(r telemetry.RunRecord, window int) map[string]any {
	glyph, state := runOutcomeGlyph(r.Outcome)
	lanes := make([]map[string]any, len(r.Lanes))
	for i, l := range r.Lanes {
		lg, ls := glyphFor(l.Status)
		lanes[i] = map[string]any{
			"Step": l.Index + 1, "Agent": s.agentLabel(l.AgentID),
			"Glyph": lg, "State": ls, "Status": l.Status,
			"Credits": telemetry.FormatCredits(l.Credits), "HasCredits": l.Credits > 0,
			// Attention attribution (S6): a lane that parked on a human shows its
			// pause count and the time it waited — where humans were the bottleneck.
			// PausedFor drops a sub-second span (humanDuration rounds it to "0s"): the
			// "⏸ N" count still shows, but "· 0s" would read as broken.
			"HasPauses": l.Pauses > 0, "Pauses": l.Pauses,
			"PausedFor": pausedFor(l.PausedMs),
		}
	}
	credits := r.Credits()
	dur := r.Duration()
	return map[string]any{
		"Name": r.Name, "Mode": r.Mode, "Glyph": glyph, "State": state, "Outcome": r.Outcome,
		"When": humanWhen(r.StartedAt), "Lanes": lanes,
		"Duration": humanDuration(dur), "HasDuration": dur > 0,
		"Credits": telemetry.FormatCredits(credits), "HasCredits": credits > 0,
		"WorkflowID": r.WorkflowID, "CanRerun": s.forge != nil && s.forge.Workflow(r.WorkflowID) != nil,
		"Window": window,
	}
}

// pausedFor renders a lane's total paused wall-clock for the Runs indicator, "" for a
// sub-second span — humanDuration rounds <1s to "0s", which beside the "⏸ N" count
// would read as broken; an empty string lets the template omit the "· …" part so a
// brief pause shows just its count. Pure.
func pausedFor(ms int64) string {
	d := humanDuration(time.Duration(ms) * time.Millisecond)
	if d == "0s" {
		return ""
	}
	return d
}

// humanDuration renders a run's wall-clock span compactly: "" for zero (an
// unfinished/zero-span run, so the cell stays empty), seconds under a minute,
// "Nm Ss" under an hour, "Nh Nm" beyond. Seconds/minutes are dropped when zero so
// a clean span reads "2m", not "2m 0s". The span is rounded to a clean resolution
// up front — to the second below an hour, the minute above — then decomposed, so a
// fractional unit that rounds up to 60 carries into the next unit (59.6s → "1m",
// not "60s") rather than printing an impossible "60s"/"1m 60s".
func humanDuration(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	if d < time.Hour {
		d = d.Round(time.Second)
	} else {
		d = d.Round(time.Minute)
	}
	switch {
	case d < time.Minute:
		return strconv.Itoa(int(d/time.Second)) + "s"
	case d < time.Hour:
		m := d / time.Minute
		if sec := (d % time.Minute) / time.Second; sec != 0 {
			return strconv.Itoa(int(m)) + "m " + strconv.Itoa(int(sec)) + "s"
		}
		return strconv.Itoa(int(m)) + "m"
	default:
		h := d / time.Hour
		if m := (d % time.Hour) / time.Minute; m != 0 {
			return strconv.Itoa(int(h)) + "h " + strconv.Itoa(int(m)) + "m"
		}
		return strconv.Itoa(int(h)) + "h"
	}
}

// runOutcomeGlyph maps a run's overall outcome to its glyph and CSS state class.
func runOutcomeGlyph(outcome string) (glyph, state string) {
	if outcome == "failed" {
		return "✗", "failed"
	}
	return "✓", "done"
}
