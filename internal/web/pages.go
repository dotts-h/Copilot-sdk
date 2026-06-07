package web

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// This file renders the top-level page partials served by GET /page/{name} and
// swapped into #main. They mirror the TUI's per-page views (internal/tui/views.go)
// as server-rendered HTML.

// pageNames is the nav order.
var pageNames = []struct{ slug, label string }{
	{"chat", "Chat"},
	{"sessions", "Sessions"},
	{"telemetry", "Telemetry"},
	{"skills", "Skills"},
	{"instructions", "Instructions"},
	{"agents", "Agents"},
	{"workflows", "Workflows"},
	{"runs", "Runs"},
	{"mcp", "MCP"},
	{"snippets", "Snippets"},
	{"models", "Models"},
	{"settings", "Settings"},
	{"help", "Help"},
}

// spendWindows is the allowed set of Telemetry "spend over time" trend windows
// (days); defaultSpendWindow is the fallback for a missing/out-of-range value and
// the historical behavior. The maxUSD scaling stays window-local (REGRESSIONS #14).
var spendWindows = []int{14, 30, 90}

const defaultSpendWindow = 14

// clampWindow parses a ?window= value to one of spendWindows, falling back to
// defaultSpendWindow (14) for an empty, unparseable, or out-of-range value.
func clampWindow(raw string) int {
	n, err := strconv.Atoi(raw)
	if err != nil {
		return defaultSpendWindow
	}
	for _, w := range spendWindows {
		if n == w {
			return n
		}
	}
	return defaultSpendWindow
}

// renderPage returns the partial for a nav slug, or chat for unknown slugs. The
// window string is the (already raw) ?window= value, used by the Telemetry trend
// selector and the Runs time-window selector (both clamp it via clampWindow); every
// other page ignores it.
func (s *Server) renderPage(slug, window string) string {
	switch slug {
	case "sessions":
		return s.sessionsPartial()
	case "telemetry":
		return s.telemetryPartial(clampWindow(window))
	case "skills":
		return s.skillsPartial()
	case "instructions":
		return s.instructionsPartial()
	case "agents":
		return s.agentsPartial()
	case "workflows":
		return s.workflowsPartial()
	case "runs":
		return s.runsPartial(clampWindow(window))
	case "mcp":
		return s.mcpServersPartial()
	case "snippets":
		return s.snippetsPartial()
	case "models":
		return s.modelsPartial()
	case "settings":
		return s.settingsPartial()
	case "help":
		return s.helpPartial()
	default:
		return s.chatPartial()
	}
}

// chatPartial renders the chat page. The dynamic regions (timeline, pending
// request forms, sub-agent strip, context meter) are rendered by their own
// renderers and injected into the chatPage template as trusted HTML — they are
// already escaped at the source (renderTurn/renderPermForm/… via richtext/esc).
func (s *Server) chatPartial() string {
	s.mu.Lock()
	timeline := renderTimelineInner(&s.state)
	var perms, asks, plans, elicits strings.Builder
	for _, p := range s.perms {
		perms.WriteString(renderPermForm(p))
	}
	for _, q := range s.inputs {
		asks.WriteString(renderAskForm(q))
	}
	for _, p := range s.plans {
		plans.WriteString(renderPlanForm(p))
	}
	for _, e := range s.elicits {
		elicits.WriteString(renderElicitForm(e))
	}
	ctx := renderCtx(s.ctxCurrent, s.ctxLimit, s.compacting)
	subagents := renderSubagents(s.subagents)
	lanes := renderLanes(s.run)
	statline := renderStatline(s)
	budget := s.renderGate()
	s.mu.Unlock()

	return frag("chatPage", map[string]any{
		"Timeline": trusted(timeline), "Perms": trusted(perms.String()),
		"Asks": trusted(asks.String()), "Plans": trusted(plans.String()),
		"Elicits": trusted(elicits.String()), "Subagents": trusted(subagents),
		"Lanes": trusted(lanes),
		"Ctx":   trusted(ctx), "Statline": trusted(statline), "Budget": trusted(budget),
	})
}

// agentLabel resolves an agent id to its display name for the cost breakdown,
// falling back to the raw id (a since-renamed/deleted agent) and labelling the
// empty-agent bucket as the built-in chat. Caller holds forgeMu.
func (s *Server) agentLabel(id string) string {
	if id == "" {
		return "chat (built-in)"
	}
	if a := s.forge.Agent(id); a != nil {
		return a.Name
	}
	return id
}

// workflowLabel resolves a workflow id to its display name, falling back to the
// raw id when the workflow was renamed or deleted. Caller holds forgeMu.
func (s *Server) workflowLabel(id string) string {
	if w := s.forge.Workflow(id); w != nil {
		return w.Name
	}
	return id
}

// handleRunsExport streams the full persisted workflow-run history as a CSV download —
// the orchestration sibling of handleSpendExport — so runs (including skipped branches,
// which leave no spend record) can be analysed outside the app. Empty (header only) when
// no run store is wired.
func (s *Server) handleRunsExport(w http.ResponseWriter, r *http.Request) {
	var records []telemetry.RunRecord
	if s.runs != nil {
		records = s.runs.Records()
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="my-orchestra-runs.csv"`)
	if err := telemetry.WriteRunsCSV(w, records); err != nil {
		s.logger.Printf("export runs csv: %v", err)
	}
}

// addData is the "+ Add" button's template data (route slug + singular noun).
func addData(kind, noun string) map[string]any { return map[string]any{"Kind": kind, "Noun": noun} }

func def(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// truncate shortens s to at most n runes, appending an ellipsis when it cuts.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}
