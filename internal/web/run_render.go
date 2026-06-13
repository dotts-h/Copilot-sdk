// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package web

import (
	"html/template"
	"strings"

	"github.com/dotts-h/copilot-sdk/internal/convo"
	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// This file renders a run engine's lanes (run_engine.go) into the workflow run
// panel and owns the status→glyph/string vocabulary shared by the live lanes and
// the persisted Runs history, so the two can never drift apart. See ADR-0013.

// lanesFrag builds the workflow-lanes SSE fragment. Caller holds s.mu.
func (s *Server) lanesFrag() fragment {
	return fragment{Event: "lanes", HTML: renderLanes(s.run)}
}

// renderLanes renders the workflow run panel: a header plus one card per lane
// (status glyph, step/agent, collapsible output, cost/detail). Empty when no run
// is active, so the region is ambient like the sub-agent strip.
func renderLanes(run *workflowRun) string {
	if run == nil {
		return ""
	}
	lanes := make([]map[string]any, len(run.lanes))
	for i, l := range run.lanes {
		glyph, state := laneGlyph(l.status)
		lanes[i] = map[string]any{
			"Step": l.Index + 1, "Agent": l.AgentName, "Glyph": glyph, "State": state,
			"Output": l.text, "HasOutput": strings.TrimSpace(l.text) != "",
			"Detail": l.detail, "HasDetail": l.detail != "",
			"Tools": laneToolsHTML(l.tools), "HasTools": len(l.tools) > 0,
			"Perms": lanePermsHTML(l.perms), "HasPerms": len(l.perms) > 0,
		}
	}
	return frag("workflowLanes", map[string]any{
		"Name": run.name, "Mode": run.mode, "Running": !run.done, "Lanes": lanes,
	})
}

// laneToolsHTML renders a lane's own tool-execution timeline by reusing the chat
// tool card (so a sub-run's tools look identical to a chat turn's). The result is
// a composed-from-escaped-fragments HTML string — its args/results pass through
// the same richtext escaping as the chat timeline (ADR-0001).
func laneToolsHTML(tools []*convo.ToolView) template.HTML {
	var b strings.Builder
	for _, tv := range tools {
		b.WriteString(renderToolCard(tv))
	}
	return trusted(b.String())
}

// lanePermsHTML renders a lane's pending inline permission requests by reusing the
// chat permission form (the compact form or the diff review lane), so a lane's
// permissions are answerable in place via the same /perm/{id} flow (ADR-0012).
func lanePermsHTML(perms []copilot.PermissionRequest) template.HTML {
	var b strings.Builder
	for _, p := range perms {
		b.WriteString(renderPermForm(p))
	}
	return trusted(b.String())
}

// laneGlyph maps a live lane status to its glyph and CSS state class. It routes
// through the status-string vocabulary (laneStatusName → glyphFor) so a live lane and
// a persisted run-history lane (which only has the string) can never drift apart.
func laneGlyph(st laneStatus) (glyph, state string) {
	return glyphFor(laneStatusName(st))
}

// glyphFor is the single source of truth mapping a lane status string to its glyph and
// CSS state class — shared by live lanes (laneGlyph) and the persisted Runs history
// (runs.go), so the two render identically.
func glyphFor(status string) (glyph, state string) {
	switch status {
	case "running":
		return "◐", "running"
	case "input-required":
		return "◑", "input-required"
	case "done":
		return "✓", "done"
	case "failed":
		return "✗", "failed"
	case "skipped":
		return "⊘", "skipped"
	default:
		return "○", "pending"
	}
}

// laneStatusName is the persisted/string form of a settled lane status. It is total
// (an unsettled lane — which a finished run never has — reads as "pending").
func laneStatusName(st laneStatus) string {
	switch st {
	case laneRunning:
		return "running"
	case laneInputRequired:
		return "input-required"
	case laneDone:
		return "done"
	case laneFailed:
		return "failed"
	case laneSkipped:
		return "skipped"
	default:
		return "pending"
	}
}

// costDetail formats a finished lane's metered cost for its summary line.
func (l *lane) costDetail() string {
	if l.credits <= 0 {
		return "done"
	}
	return telemetry.FormatCredits(l.credits)
}
