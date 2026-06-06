package web

import (
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// This file renders the Runs page: the persisted workflow-run history (ADR-0022). A
// run is the product's unit of orchestration, recorded once on completion by
// workflow.go (recordRun). The page is a read-only view over telemetry.RunStore,
// resolving agent ids to display names like the Telemetry cost breakdowns.

// runsPartial renders the Runs page: the persisted run history, most recent first.
// Each run shows its name, mode, outcome, when it ran, total metered cost, and a
// per-lane breakdown (agent, status glyph, credits). Empty when no run store is wired
// or none have run yet.
func (s *Server) runsPartial() string {
	rows := []map[string]any{}
	if s.runs != nil {
		records := s.runs.Records()
		s.hub.forgeMu.Lock()
		for i := len(records) - 1; i >= 0; i-- { // newest first
			rows = append(rows, s.runRow(records[i]))
		}
		s.hub.forgeMu.Unlock()
	}
	return frag("runsPage", map[string]any{"Rows": rows})
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
	return map[string]any{
		"Name": r.Name, "Mode": r.Mode, "Glyph": glyph, "State": state, "Outcome": r.Outcome,
		"When": humanWhen(r.StartedAt), "Lanes": lanes,
		"Credits": telemetry.FormatCredits(credits), "HasCredits": credits > 0,
	}
}

// runOutcomeGlyph maps a run's overall outcome to its glyph and CSS state class.
func runOutcomeGlyph(outcome string) (glyph, state string) {
	if outcome == "failed" {
		return "✗", "failed"
	}
	return "✓", "done"
}
