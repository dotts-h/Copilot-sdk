package web

import (
	"context"
	"fmt"
	"strings"

	"github.com/dotts-h/copilot-sdk/internal/config"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// This file renders the top-level page partials served by GET /page/{name} and
// swapped into #main. They mirror the TUI's per-page views (internal/tui/views.go)
// as server-rendered HTML.

// pageNames is the nav order.
var pageNames = []struct{ slug, label string }{
	{"chat", "Chat"},
	{"telemetry", "Telemetry"},
	{"skills", "Skills"},
	{"instructions", "Instructions"},
	{"agents", "Agents"},
	{"models", "Models"},
	{"settings", "Settings"},
	{"help", "Help"},
}

// renderPage returns the partial for a nav slug, or chat for unknown slugs.
func (s *Server) renderPage(slug string) string {
	switch slug {
	case "telemetry":
		return s.telemetryPartial()
	case "skills":
		return s.skillsPartial()
	case "instructions":
		return s.instructionsPartial()
	case "agents":
		return s.agentsPartial()
	case "models":
		return s.modelsPartial()
	case "settings":
		return s.settingsPartial()
	case "help":
		return helpPartial()
	default:
		return s.chatPartial()
	}
}

// helpPartial renders the static Help/reference page: how the panels work and
// the full set of composer slash commands. It is the discoverability surface
// behind /help in the composer.
func helpPartial() string {
	cmd := func(name, desc string) string {
		return `<tr><th><code>` + esc(name) + `</code></th><td>` + esc(desc) + `</td></tr>`
	}
	var b strings.Builder
	b.WriteString(`<section class="page help" tabindex="0"><h2>Help</h2>`)
	b.WriteString(`<p class="dim">my-orchestra is a cost-aware coding companion. Chat streams live; ` +
		`every tool call, the reasoning, and the credit spend are shown as they happen.</p>`)

	b.WriteString(`<h3>Composer commands</h3>`)
	b.WriteString(`<p class="dim">Type these in the chat composer instead of a prompt.</p>`)
	b.WriteString(`<table class="kv">`)
	b.WriteString(cmd("/model [name]", "Switch the model in place (restarts the session); no name shows the current one."))
	b.WriteString(cmd("/agent [id|none]", "Activate a forge agent (applies its model + reasoning) or clear it."))
	b.WriteString(cmd("/plan [on|off]", "Toggle plan mode — the agent drafts a plan you approve or revise inline before it acts."))
	b.WriteString(cmd("/clear", "Reset the conversation and start a fresh session."))
	b.WriteString(cmd("/cost", "Show credit usage and refresh the cost meter."))
	b.WriteString(cmd("/attach <path>", "Queue a file to send with your next message."))
	b.WriteString(cmd("/chat … /settings", "Jump to a page (chat, telemetry, skills, instructions, agents, settings)."))
	b.WriteString(cmd("/help", "List the commands in the timeline."))
	b.WriteString(`</table>`)

	b.WriteString(`<h3>Panels</h3><table class="kv">`)
	rows := [][2]string{
		{"Chat", "Stream prompts and replies; approve tool permissions, answer the agent's questions (ask_user), fill schema-driven forms from MCP servers (elicitation), and review its plans (approve or request changes) — all inline; abort an in-flight turn with ⏹ stop. " +
			"Type ahead while a turn runs — extra prompts queue and send automatically when the turn ends."},
		{"Telemetry", "Live credit/token spend, per-model breakdown, and your monthly budget."},
		{"Skills", "Reusable prompt fragments; toggle which are active for the session."},
		{"Instructions", "Always-on guidance, ordered by priority."},
		{"Agents", "Named model + reasoning + skill presets; select one as the default."},
		{"Models", "Pick the model the session uses; selecting one restarts the session on your next prompt."},
		{"Settings", "Read-only view of config.json / forge.json; edit those files to change them."},
	}
	for _, r := range rows {
		fmt.Fprintf(&b, `<tr><th>%s</th><td>%s</td></tr>`, esc(r[0]), esc(r[1]))
	}
	b.WriteString(`</table></section>`)
	return b.String()
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
		perms.WriteString(renderPermForm(p.ID, p.Detail))
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
	s.mu.Unlock()

	return frag("chatPage", map[string]any{
		"Timeline": trusted(timeline), "Perms": trusted(perms.String()),
		"Asks": trusted(asks.String()), "Plans": trusted(plans.String()),
		"Elicits": trusted(elicits.String()), "Subagents": trusted(subagents),
		"Ctx": trusted(ctx),
	})
}

func (s *Server) telemetryPartial() string {
	totals := s.meter.Totals()
	in, cached, out := s.meter.TotalTokens()
	budget := telemetry.Budget{AllowanceCredits: s.allowance}
	frac := budget.FractionUsed(totals.Credits())

	rows := [][2]string{
		{"Total cost", fmt.Sprintf("%s (%s)", telemetry.FormatCredits(totals.Credits()), telemetry.FormatUSD(totals.USD()))},
		{"Monthly budget", fmt.Sprintf("%.2f of %.0f cr", totals.Credits(), budget.AllowanceCredits)},
		{"Remaining", telemetry.FormatCredits(budget.Remaining(totals.Credits()))},
	}
	if aiu := s.meter.ReportedAIU(); aiu > 0 {
		rows = append(rows, [2]string{"GitHub-reported cost", fmt.Sprintf("%.4f AIU", aiu)})
	}
	rows = append(rows, [2]string{"Tokens", fmt.Sprintf("input %d · cached %d · output %d", in, cached, out)})

	pct := frac * 100
	if pct > 100 {
		pct = 100
	}
	models := make([]map[string]any, 0)
	for _, r := range s.meter.ByModel() {
		models = append(models, map[string]any{
			"Model": r.Model, "In": r.InputTokens, "Cached": r.CachedTokens, "Out": r.OutputTokens,
			"Credits": fmt.Sprintf("%.2f", r.Credits()), "USD": telemetry.FormatUSD(r.USD()),
		})
	}
	return frag("telemetryPage", map[string]any{
		"Rows": rows, "Width": fmt.Sprintf("%.1f%%", pct), "Models": models,
	})
}

func (s *Server) skillsPartial() string {
	s.hub.forgeMu.Lock()
	defer s.hub.forgeMu.Unlock()
	rows := make([]map[string]any, 0, len(s.forge.Skills))
	for _, sk := range s.forge.Skills {
		rows = append(rows, map[string]any{
			"Kind": "skills", "ID": sk.ID, "On": sk.Enabled, "Name": sk.Name, "Desc": truncate(sk.Description, 80),
		})
	}
	return frag("skillsPage", map[string]any{"Add": addData("skills", "skill"), "Rows": rows})
}

func (s *Server) instructionsPartial() string {
	s.hub.forgeMu.Lock()
	defer s.hub.forgeMu.Unlock()
	rows := make([]map[string]any, 0, len(s.forge.Instructions))
	for _, ins := range s.forge.Instructions {
		rows = append(rows, map[string]any{
			"Kind": "instructions", "ID": ins.ID, "On": ins.Enabled,
			"Name": fmt.Sprintf("%s (p%d)", ins.Title, ins.Priority), "Desc": truncate(ins.Body, 80),
		})
	}
	return frag("instructionsPage", map[string]any{"Add": addData("instructions", "instruction"), "Rows": rows})
}

func (s *Server) agentsPartial() string {
	s.hub.forgeMu.Lock()
	defer s.hub.forgeMu.Unlock()
	rows := make([]map[string]any, 0, len(s.forge.Agents))
	for _, a := range s.forge.Agents {
		desc := fmt.Sprintf("%s · %s · %s", a.Model, def(a.ReasoningEffort, "medium"), a.Description)
		rows = append(rows, map[string]any{
			"ID": a.ID, "Active": a.ID == s.config.DefaultAgent, "Name": a.Name, "Desc": desc,
		})
	}
	return frag("agentsPage", map[string]any{"Add": addData("agents", "agent"), "Rows": rows})
}

// addData is the "+ Add" button's template data (route slug + singular noun).
func addData(kind, noun string) map[string]any { return map[string]any{"Kind": kind, "Noun": noun} }

// modelsPartial renders the model picker: every model the account can use, with
// the current one marked and the rest offering a one-click switch (POST
// /models/{id}/select, which restarts the session — workstream 3). It degrades
// to a notice when the runtime can't list models.
func (s *Server) modelsPartial() string {
	models, err := s.client.ListModels(context.Background())
	if err != nil {
		return frag("modelsPage", map[string]any{"Err": err.Error()})
	}

	s.mu.Lock()
	current := s.spec.Model
	s.mu.Unlock()

	rows := make([]map[string]any, 0, len(models))
	for _, m := range models {
		desc := m.ID
		if efforts := strings.Join(m.SupportedReasoningEfforts, ", "); efforts != "" {
			desc += " · reasoning: " + efforts
		}
		rows = append(rows, map[string]any{
			"ID": m.ID, "Active": m.ID == current, "Name": def(m.Name, m.ID), "Desc": desc,
		})
	}
	return frag("modelsPage", map[string]any{"Rows": rows})
}

func (s *Server) settingsPartial() string {
	s.hub.forgeMu.Lock()
	defer s.hub.forgeMu.Unlock()
	c := s.config
	rows := [][2]string{
		{"Default model", c.DefaultModel},
		{"Default agent", def(c.DefaultAgent, "(none)")},
		{"Reasoning effort", def(c.ReasoningEffort, "medium")},
		{"Streaming", onoff(c.Streaming)},
		{"Auto-approve tools", onoff(c.AutoApproveTools)},
		{"Auth", authState(c)},
		{"Monthly credit budget", fmt.Sprintf("%.0f cr", c.Telemetry.MonthlyCreditAllowance)},
		{"Warn at", fmt.Sprintf("%.0f%%", c.Telemetry.WarnFraction*100)},
		{"OTLP endpoint", def(c.Telemetry.OTLPEndpoint, "(off)")},
		{"Runtime", "github/copilot-sdk/go (copilot CLI)"},
		{"Forge dir", c.ForgeDir},
	}
	return frag("settingsPage", map[string]any{"Rows": rows})
}

func onoff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func def(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// authState mirrors internal/tui/views.go: explicit token when configured,
// otherwise the logged-in copilot CLI session.
func authState(c *config.Config) string {
	if c.GitHubTokenEnv == "" {
		return "logged-in copilot CLI session"
	}
	if c.GitHubToken() != "" {
		return fmt.Sprintf("token from $%s (set)", c.GitHubTokenEnv)
	}
	return fmt.Sprintf("token from $%s (unset → logged-in CLI)", c.GitHubTokenEnv)
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
