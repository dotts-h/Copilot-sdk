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
	b.WriteString(`<section class="page help"><h2>Help</h2>`)
	b.WriteString(`<p class="dim">my-orchestra is a cost-aware coding companion. Chat streams live; ` +
		`every tool call, the reasoning, and the credit spend are shown as they happen.</p>`)

	b.WriteString(`<h3>Composer commands</h3>`)
	b.WriteString(`<p class="dim">Type these in the chat composer instead of a prompt.</p>`)
	b.WriteString(`<table class="kv">`)
	b.WriteString(cmd("/model [name]", "Switch the model in place (restarts the session); no name shows the current one."))
	b.WriteString(cmd("/agent [id|none]", "Activate a forge agent (applies its model + reasoning) or clear it."))
	b.WriteString(cmd("/clear", "Reset the conversation and start a fresh session."))
	b.WriteString(cmd("/cost", "Show credit usage and refresh the cost meter."))
	b.WriteString(cmd("/attach <path>", "Queue a file to send with your next message."))
	b.WriteString(cmd("/chat … /settings", "Jump to a page (chat, telemetry, skills, instructions, agents, settings)."))
	b.WriteString(cmd("/help", "List the commands in the timeline."))
	b.WriteString(`</table>`)

	b.WriteString(`<h3>Panels</h3><table class="kv">`)
	rows := [][2]string{
		{"Chat", "Stream prompts and replies; approve tool permissions, answer the agent's questions (ask_user), and review its plans (approve or request changes) — all inline; abort an in-flight turn with ⏹ stop. " +
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

// chatPartial renders the chat page: timeline (from state), pending permission
// forms, status line, and the composer.
func (s *Server) chatPartial() string {
	s.mu.Lock()
	timeline := renderTimelineInner(&s.state)
	var perms strings.Builder
	for _, p := range s.perms {
		perms.WriteString(renderPermForm(p.ID, p.Detail))
	}
	var asks strings.Builder
	for _, q := range s.inputs {
		asks.WriteString(renderAskForm(q))
	}
	var plans strings.Builder
	for _, p := range s.plans {
		plans.WriteString(renderPlanForm(p))
	}
	ctx := renderCtx(s.ctxCurrent, s.ctxLimit, s.compacting)
	subagents := renderSubagents(s.subagents)
	s.mu.Unlock()

	return `<section class="chat">` +
		`<div id="timeline" class="timeline">` + timeline + `</div>` +
		`<div id="perms" class="perms">` + perms.String() + `</div>` +
		`<div id="asks" class="asks">` + asks.String() + `</div>` +
		`<div id="plans" class="plans">` + plans.String() + `</div>` +
		`<div class="composer-bar">` +
		`<div id="subagents" class="subagents">` + subagents + `</div>` +
		`<div class="status-row">` +
		`<div id="status" class="status"></div>` +
		`<div id="ctx" class="ctx">` + ctx + `</div>` +
		`</div>` +
		`<div id="cmd-menu" class="cmd-menu-wrap"></div>` +
		`<form id="composer" hx-post="/send" hx-swap="none" ` +
		`hx-on::after-request="this.reset(); document.getElementById('cmd-menu').innerHTML=''; this.querySelector('input').focus()">` +
		`<input type="text" name="prompt" autocomplete="off" autofocus placeholder="Ask my-orchestra…  (/help for commands)" ` +
		`hx-get="/commands" hx-trigger="keyup changed delay:40ms" hx-target="#cmd-menu" hx-swap="innerHTML">` +
		`<button type="submit">Send</button>` +
		`</form></div></section>`
}

func (s *Server) telemetryPartial() string {
	totals := s.meter.Totals()
	in, cached, out := s.meter.TotalTokens()
	budget := telemetry.Budget{AllowanceCredits: s.allowance}
	frac := budget.FractionUsed(totals.Credits())

	var b strings.Builder
	b.WriteString(`<section class="page"><h2>Credits &amp; Token Telemetry</h2>`)
	b.WriteString(`<table class="kv">`)
	row := func(k, v string) { fmt.Fprintf(&b, `<tr><th>%s</th><td>%s</td></tr>`, k, v) }
	row("Total cost", fmt.Sprintf("%s (%s)", esc(telemetry.FormatCredits(totals.Credits())), esc(telemetry.FormatUSD(totals.USD()))))
	row("Monthly budget", fmt.Sprintf("%.2f of %.0f cr", totals.Credits(), budget.AllowanceCredits))
	row("Remaining", esc(telemetry.FormatCredits(budget.Remaining(totals.Credits()))))
	if aiu := s.meter.ReportedAIU(); aiu > 0 {
		row("GitHub-reported cost", fmt.Sprintf("%.4f AIU", aiu))
	}
	row("Tokens", fmt.Sprintf("input %d · cached %d · output %d", in, cached, out))
	b.WriteString(`</table>`)
	pct := frac * 100
	if pct > 100 {
		pct = 100
	}
	fmt.Fprintf(&b, `<div class="meter wide"><span class="meter-fill" style="width:%.1f%%"></span></div>`, pct)

	b.WriteString(`<h3>Per-model breakdown</h3><table class="grid">`)
	b.WriteString(`<tr><th>model</th><th>in</th><th>cached</th><th>out</th><th>credits</th><th>usd</th></tr>`)
	rows := s.meter.ByModel()
	if len(rows) == 0 {
		b.WriteString(`<tr><td colspan="6" class="dim">no usage yet — send a prompt on the Chat page</td></tr>`)
	}
	for _, r := range rows {
		fmt.Fprintf(&b, `<tr><td>%s</td><td>%d</td><td>%d</td><td>%d</td><td>%.2f</td><td>%s</td></tr>`,
			esc(r.Model), r.InputTokens, r.CachedTokens, r.OutputTokens, r.Credits(), esc(telemetry.FormatUSD(r.USD())))
	}
	b.WriteString(`</table></section>`)
	return b.String()
}

func (s *Server) skillsPartial() string {
	var b strings.Builder
	b.WriteString(`<section class="page"><h2>Skills</h2>`)
	b.WriteString(addButton("skills", "skill"))
	if len(s.forge.Skills) == 0 {
		b.WriteString(`<p class="dim">no skills yet — add them to the forge (forge.json)</p>`)
	}
	b.WriteString(`<ul class="rows">`)
	for _, sk := range s.forge.Skills {
		b.WriteString(toggleRow("skills", sk.ID, sk.Enabled, sk.Name, sk.Description))
	}
	b.WriteString(`</ul></section>`)
	return b.String()
}

func (s *Server) instructionsPartial() string {
	var b strings.Builder
	b.WriteString(`<section class="page"><h2>Instructions</h2>`)
	b.WriteString(addButton("instructions", "instruction"))
	if len(s.forge.Instructions) == 0 {
		b.WriteString(`<p class="dim">no instructions yet</p>`)
	}
	b.WriteString(`<ul class="rows">`)
	for _, ins := range s.forge.Instructions {
		label := fmt.Sprintf("%s (p%d)", ins.Title, ins.Priority)
		b.WriteString(toggleRow("instructions", ins.ID, ins.Enabled, label, ins.Body))
	}
	b.WriteString(`</ul></section>`)
	return b.String()
}

func (s *Server) agentsPartial() string {
	var b strings.Builder
	b.WriteString(`<section class="page"><h2>Agents</h2>`)
	b.WriteString(addButton("agents", "agent"))
	if len(s.forge.Agents) == 0 {
		b.WriteString(`<p class="dim">no agents yet</p>`)
	}
	b.WriteString(`<ul class="rows">`)
	for _, a := range s.forge.Agents {
		active := a.ID == s.config.DefaultAgent
		desc := fmt.Sprintf("%s · %s · %s", a.Model, def(a.ReasoningEffort, "medium"), a.Description)
		mark := `<button class="toggle" hx-post="/agents/` + esc(a.ID) + `/select" hx-target="#main" hx-swap="innerHTML">` +
			markGlyph(active) + `</button>`
		edit := `<button class="edit" hx-get="/agents/` + esc(a.ID) + `/edit" hx-target="#main" hx-swap="innerHTML">edit</button>`
		del := `<button class="del" hx-post="/agents/` + esc(a.ID) + `/delete" hx-target="#main" hx-swap="innerHTML" hx-confirm="Delete agent ` + esc(a.Name) + `?">✕</button>`
		fmt.Fprintf(&b, `<li class="row %s">%s<div class="row-text"><span class="row-name">%s</span><span class="row-desc">%s</span></div>%s%s</li>`,
			activeClass(active), mark, esc(a.Name), esc(desc), edit, del)
	}
	b.WriteString(`</ul></section>`)
	return b.String()
}

// modelsPartial renders the model picker: every model the account can use, with
// the current one marked and the rest offering a one-click switch (POST
// /models/{id}/select, which restarts the session — workstream 3). It degrades
// to a notice when the runtime can't list models.
func (s *Server) modelsPartial() string {
	models, err := s.client.ListModels(context.Background())

	s.mu.Lock()
	current := s.spec.Model
	s.mu.Unlock()

	var b strings.Builder
	b.WriteString(`<section class="page"><h2>Models</h2>`)
	if err != nil {
		b.WriteString(`<p class="dim">models unavailable: ` + esc(err.Error()) + `</p></section>`)
		return b.String()
	}
	if len(models) == 0 {
		b.WriteString(`<p class="dim">no models available</p></section>`)
		return b.String()
	}
	b.WriteString(`<p class="dim">selecting a model sets it as default and restarts the session on your next prompt</p>`)
	b.WriteString(`<ul class="rows">`)
	for _, m := range models {
		active := m.ID == current
		desc := m.ID
		if efforts := strings.Join(m.SupportedReasoningEfforts, ", "); efforts != "" {
			desc += " · reasoning: " + efforts
		}
		var action string
		if active {
			action = `<button class="toggle" disabled>` + markGlyph(true) + `</button>`
		} else {
			action = `<button class="toggle" hx-post="/models/` + esc(m.ID) + `/select" hx-target="#main" hx-swap="innerHTML">use</button>`
		}
		fmt.Fprintf(&b, `<li class="row %s">%s<div class="row-text"><span class="row-name">%s</span><span class="row-desc">%s</span></div></li>`,
			activeClass(active), action, esc(def(m.Name, m.ID)), esc(desc))
	}
	b.WriteString(`</ul></section>`)
	return b.String()
}

func (s *Server) settingsPartial() string {
	c := s.config
	rows := [][2]string{
		{"Default model", c.DefaultModel},
		{"Default agent", def(c.DefaultAgent, "(none)")},
		{"Reasoning effort", def(c.ReasoningEffort, "medium")},
		{"Theme", string(c.Theme)},
		{"Streaming", onoff(c.Streaming)},
		{"Auto-approve tools", onoff(c.AutoApproveTools)},
		{"Auth", authState(c)},
		{"Monthly credit budget", fmt.Sprintf("%.0f cr", c.Telemetry.MonthlyCreditAllowance)},
		{"Warn at", fmt.Sprintf("%.0f%%", c.Telemetry.WarnFraction*100)},
		{"OTLP endpoint", def(c.Telemetry.OTLPEndpoint, "(off)")},
		{"Runtime", "github/copilot-sdk/go (copilot CLI)"},
		{"Forge dir", c.ForgeDir},
	}
	var b strings.Builder
	b.WriteString(`<section class="page"><h2>Settings</h2>`)
	b.WriteString(`<p class="dim">edit forge.json / config.json; live-applied on next session</p>`)
	b.WriteString(`<table class="kv">`)
	for _, r := range rows {
		fmt.Fprintf(&b, `<tr><th>%s</th><td>%s</td></tr>`, esc(r[0]), esc(r[1]))
	}
	b.WriteString(`</table></section>`)
	return b.String()
}

// toggleRow renders a list row with an enable/disable toggle and a delete button
// for the skills/instructions pages.
func toggleRow(kind, id string, on bool, name, desc string) string {
	eid := esc(id)
	toggle := `<button class="toggle" hx-post="/` + kind + `/` + eid + `/toggle" hx-target="#main" hx-swap="innerHTML">` + markGlyph(on) + `</button>`
	edit := `<button class="edit" hx-get="/` + kind + `/` + eid + `/edit" hx-target="#main" hx-swap="innerHTML">edit</button>`
	del := `<button class="del" hx-post="/` + kind + `/` + eid + `/delete" hx-target="#main" hx-swap="innerHTML" hx-confirm="Delete ` + esc(name) + `?">✕</button>`
	return fmt.Sprintf(`<li class="row %s">%s<div class="row-text"><span class="row-name">%s</span><span class="row-desc">%s</span></div>%s%s</li>`,
		activeClass(on), toggle, esc(name), esc(truncate(desc, 80)), edit, del)
}

// addButton renders the "+ Add" control that loads a blank create form for the
// given list page into #main. kind is the route slug, noun the singular label.
func addButton(kind, noun string) string {
	return `<button class="add" hx-get="/` + kind + `/new" hx-target="#main" hx-swap="innerHTML">+ Add ` + esc(noun) + `</button>`
}

func markGlyph(on bool) string {
	if on {
		return `<span class="on">[x]</span>`
	}
	return `<span class="off">[ ]</span>`
}

func activeClass(on bool) string {
	if on {
		return "on"
	}
	return ""
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
