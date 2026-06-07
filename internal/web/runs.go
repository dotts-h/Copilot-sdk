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
// failure rate, average cost, average duration — the cost ⋈ orchestration roll-up,
// ADR-0022/V1) above the persisted run history, most recent first. Each run shows
// its name, mode, outcome, when it ran, how long it took, total metered cost, and a
// per-lane breakdown (agent, status glyph, credits). Both the run rows and the
// summary resolve ids to display names under forgeMu, like the cost breakdowns.
// Empty when no run store is wired or none have run yet.
func (s *Server) runsPartial() string {
	rows := []map[string]any{}
	summary := []map[string]any{}
	if s.runs != nil {
		records := s.runs.Records()
		s.hub.forgeMu.Lock()
		for i := len(records) - 1; i >= 0; i-- { // newest first
			rows = append(rows, s.runRow(records[i]))
		}
		for _, a := range telemetry.RunAggregates(records) {
			summary = append(summary, s.runSummaryRow(a))
		}
		s.hub.forgeMu.Unlock()
	}
	return frag("runsPage", map[string]any{"Rows": rows, "Summary": summary})
}

// runSummaryRow builds the template shape for one per-workflow summary row: the
// workflow's display name, its run/failure counts, failure rate as a percentage,
// and its average cost and duration. Workflow ids resolve to names via
// workflowLabel — caller holds forgeMu.
func (s *Server) runSummaryRow(a telemetry.RunAggregate) map[string]any {
	return map[string]any{
		"Workflow":    s.workflowLabel(a.WorkflowID),
		"Runs":        a.Runs,
		"Failures":    a.Failures,
		"FailPct":     fmt.Sprintf("%.0f", a.FailureRate()*100),
		"HasFailures": a.Failures > 0,
		"AvgCredits":  telemetry.FormatCredits(a.AvgCredits),
		"AvgDuration": humanDuration(a.AvgDuration),
	}
}

// runRow builds the template shape for one persisted run: its header (name, mode,
// outcome glyph, when, total cost) and a per-lane breakdown. Agent ids resolve to
// display names via agentLabel — caller holds forgeMu.
func (s *Server) runRow(r telemetry.RunRecord) map[string]any {
	glyph, state := runOutcomeGlyph(r.Outcome)
	lanes := make([]map[string]any, len(r.Lanes))
	for i, l := range r.Lanes {
		lg, ls := glyphFor(l.Status)
		lanes[i] = map[string]any{
			"Step": l.Index + 1, "Agent": s.agentLabel(l.AgentID),
			"Glyph": lg, "State": ls, "Status": l.Status,
			"Credits": telemetry.FormatCredits(l.Credits), "HasCredits": l.Credits > 0,
		}
	}
	credits := r.Credits()
	dur := r.Duration()
	return map[string]any{
		"Name": r.Name, "Mode": r.Mode, "Glyph": glyph, "State": state, "Outcome": r.Outcome,
		"When": humanWhen(r.StartedAt), "Lanes": lanes,
		"Duration": humanDuration(dur), "HasDuration": dur > 0,
		"Credits": telemetry.FormatCredits(credits), "HasCredits": credits > 0,
	}
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
