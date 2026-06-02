package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

func lipglossWidth(s string) int { return lipgloss.Width(s) }

// refreshViewport re-renders the transcript into the scroll viewport and pins
// it to the bottom so the latest output is visible.
func (m *Model) refreshViewport() {
	if m.viewport.Width == 0 {
		return
	}
	m.viewport.SetContent(m.renderTranscript())
	m.viewport.GotoBottom()
}

func (m Model) renderTranscript() string {
	var b strings.Builder
	for _, t := range m.chat.transcript() {
		switch t.Role {
		case RoleUser:
			b.WriteString(m.styles.User.Render("you") + "\n")
			b.WriteString(indent(t.Text, "  ") + "\n\n")
		case RoleAgent:
			b.WriteString(m.styles.Agent.Render("orchestra") + "\n")
			b.WriteString(indent(t.Text, "  ") + "\n\n")
		case RoleSystem:
			b.WriteString(m.styles.Dim.Render("· "+t.Text) + "\n\n")
		case RoleTool:
			b.WriteString(m.styles.Tool.Render("⚙ "+t.Text) + "\n\n")
		}
	}
	if len(m.chat.tools) > 0 {
		b.WriteString(m.styles.Tool.Render("⚙ running: "+strings.Join(m.chat.tools, ", ")) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) viewChat() string {
	body := m.viewport.View()
	if len(m.permQueue) > 0 {
		req := m.permQueue[0]
		extra := ""
		if n := len(m.permQueue) - 1; n > 0 {
			extra = m.styles.Dim.Render(fmt.Sprintf("  (+%d more)", n))
		}
		prompt := m.styles.Warn.Render("⚠ allow ") + m.styles.Key.Render(req.Detail) +
			m.styles.Warn.Render("?  ") + m.styles.Good.Render("[y]es") + " / " +
			m.styles.Bad.Render("[n]o") + extra
		return body + "\n" + prompt + "\n" + m.input.View()
	}
	thinking := ""
	if m.thinking {
		thinking = m.styles.Dim.Render("  ● orchestra is working…")
	}
	return body + "\n" + thinking + "\n" + m.input.View()
}

func (m Model) viewTelemetry() string {
	var b strings.Builder
	totals := m.deps.Meter.Totals()
	in, cached, out := m.deps.Meter.TotalTokens()
	budget := telemetry.Budget{AllowanceCredits: m.deps.Config.Telemetry.MonthlyCreditAllowance}
	frac := budget.FractionUsed(totals.Credits())

	b.WriteString(m.styles.Title.Render("Credits & Token Telemetry") + "\n\n")
	b.WriteString(fmt.Sprintf("  Total cost     %s   (%s)\n",
		m.styles.Key.Render(telemetry.FormatCredits(totals.Credits())),
		m.styles.Dim.Render(telemetry.FormatUSD(totals.USD()))))
	b.WriteString(fmt.Sprintf("  Monthly budget %s of %.0f cr  %s\n",
		m.styles.Key.Render(fmt.Sprintf("%.2f", totals.Credits())),
		budget.AllowanceCredits, bar(m.styles, frac, 24)))
	b.WriteString(fmt.Sprintf("  Remaining      %s\n\n",
		creditColor(m.styles, budget.Remaining(totals.Credits()))))

	b.WriteString(m.styles.Dim.Render(fmt.Sprintf(
		"  Tokens  input %d   cached %d   output %d   (every category GitHub meters)\n",
		in, cached, out)))
	if aiu := m.deps.Meter.ReportedAIU(); aiu > 0 {
		b.WriteString(fmt.Sprintf("  GitHub-reported cost  %s\n",
			m.styles.Key.Render(fmt.Sprintf("%.4f AIU", aiu))))
	} else {
		b.WriteString(m.styles.Dim.Render("  GitHub-reported cost  (awaiting first metered call)\n"))
	}
	b.WriteString("\n")

	b.WriteString(m.styles.Title.Render("  Per-model breakdown") + "\n")
	b.WriteString(m.styles.Dim.Render(fmt.Sprintf("  %-22s %8s %8s %8s %10s %10s\n",
		"model", "in", "cached", "out", "credits", "usd")))
	rows := m.deps.Meter.ByModel()
	if len(rows) == 0 {
		b.WriteString(m.styles.Dim.Render("  (no usage yet — send a prompt on the Chat page)\n"))
	}
	for _, r := range rows {
		b.WriteString(fmt.Sprintf("  %-22s %8d %8d %8d %10s %10s\n",
			truncate(r.Model, 22), r.InputTokens, r.CachedTokens, r.OutputTokens,
			fmt.Sprintf("%.2f", r.Credits()), telemetry.FormatUSD(r.USD())))
	}
	return b.String()
}

func creditColor(s Styles, credits float64) string {
	str := telemetry.FormatCredits(credits)
	if credits < 0 {
		return s.Bad.Render(str + " (over budget)")
	}
	return s.Good.Render(str)
}

func (m Model) viewList() string {
	var b strings.Builder
	cur := m.cursor[m.page]
	switch m.page {
	case PageSkills:
		b.WriteString(m.styles.Title.Render("Skills") +
			m.styles.Dim.Render("   space: toggle • a: add • e: edit • d: delete") + "\n\n")
		if len(m.deps.Forge.Skills) == 0 {
			b.WriteString(m.styles.Dim.Render("  no skills yet — add them to the forge (forge.json)\n"))
		}
		for i, s := range m.deps.Forge.Skills {
			b.WriteString(rowToggle(m.styles, i == cur, s.Enabled, s.Name, s.Description))
		}
	case PageInstructions:
		b.WriteString(m.styles.Title.Render("Instructions") +
			m.styles.Dim.Render("   space: toggle • a: add • e: edit • d: delete") + "\n\n")
		if len(m.deps.Forge.Instructions) == 0 {
			b.WriteString(m.styles.Dim.Render("  no instructions yet\n"))
		}
		for i, ins := range m.deps.Forge.Instructions {
			label := fmt.Sprintf("%s (p%d)", ins.Title, ins.Priority)
			b.WriteString(rowToggle(m.styles, i == cur, ins.Enabled, label, truncate(ins.Body, 48)))
		}
	case PageAgents:
		b.WriteString(m.styles.Title.Render("Agents") +
			m.styles.Dim.Render("   enter: set default • a: add • e: edit • d: delete") + "\n\n")
		if len(m.deps.Forge.Agents) == 0 {
			b.WriteString(m.styles.Dim.Render("  no agents yet\n"))
		}
		for i, a := range m.deps.Forge.Agents {
			active := a.ID == m.deps.Config.DefaultAgent
			desc := fmt.Sprintf("%s · %s · %s", a.Model, def(a.ReasoningEffort, "medium"), a.Description)
			b.WriteString(rowToggle(m.styles, i == cur, active, a.Name, desc))
		}
	}
	return b.String()
}

func (m Model) viewForm() string {
	f := m.form
	var b strings.Builder
	b.WriteString(m.styles.Title.Render(f.title()) +
		m.styles.Dim.Render("   tab/↑↓: move • enter: save • esc: cancel") + "\n\n")
	for i := range f.fields {
		fld := &f.fields[i]
		marker := "  "
		labelStyle := m.styles.Dim
		if i == f.focus {
			marker = m.styles.Selected.Render("›") + " "
			labelStyle = m.styles.Key
		}
		req := ""
		if fld.required {
			req = m.styles.Bad.Render("*")
		}
		b.WriteString(fmt.Sprintf("%s%s%s\n", marker, labelStyle.Render(fld.label), req))
		b.WriteString("    " + fld.input.View() + "\n")
		if fld.help != "" {
			b.WriteString("    " + m.styles.Dim.Render(fld.help) + "\n")
		}
		b.WriteString("\n")
	}
	if m.errText != "" {
		b.WriteString("  " + m.styles.Bad.Render("⚠ "+m.errText) + "\n")
	}
	return b.String()
}

func rowToggle(s Styles, selected, on bool, name, desc string) string {
	mark := s.Dim.Render("[ ]")
	if on {
		mark = s.Good.Render("[x]")
	}
	line := fmt.Sprintf("%s %s  %s", mark, name, s.Dim.Render(desc))
	if selected {
		return "  " + s.Selected.Render("›") + " " + line + "\n"
	}
	return "    " + line + "\n"
}

func (m Model) viewSettings() string {
	c := m.deps.Config
	var b strings.Builder
	b.WriteString(m.styles.Title.Render("Settings") +
		m.styles.Dim.Render("   edit forge.json / config.json; live-applied on next session") + "\n\n")
	rows := [][2]string{
		{"Default model", c.DefaultModel},
		{"Default agent", def(c.DefaultAgent, "(none)")},
		{"Reasoning effort", def(c.ReasoningEffort, "medium")},
		{"Theme", string(c.Theme)},
		{"Streaming", yesno(c.Streaming)},
		{"Auto-approve tools", yesno(c.AutoApproveTools)},
		{"GitHub token env", c.GitHubTokenEnv + tokenState(c)},
		{"Monthly credit budget", fmt.Sprintf("%.0f cr", c.Telemetry.MonthlyCreditAllowance)},
		{"Warn at", fmt.Sprintf("%.0f%%", c.Telemetry.WarnFraction*100)},
		{"OTLP endpoint", def(c.Telemetry.OTLPEndpoint, "(off)")},
		{"Runtime", "github/copilot-sdk/go (copilot CLI)"},
		{"Forge dir", c.ForgeDir},
	}
	for _, r := range rows {
		b.WriteString(fmt.Sprintf("  %-22s %s\n", m.styles.Dim.Render(r[0]), r[1]))
	}
	return b.String()
}

func tokenState(c interface{ GitHubToken() string }) string {
	if c.GitHubToken() != "" {
		return "  (set)"
	}
	return "  (unset)"
}

func (m Model) viewConfig() string {
	var b strings.Builder
	b.WriteString(m.styles.Title.Render("Config") +
		m.styles.Dim.Render("   key bindings & paths (advanced)") + "\n\n")
	b.WriteString(m.styles.Dim.Render("  Config dir: ") + m.deps.Config.Dir() + "\n\n")
	keys := m.deps.Config.Keybindings
	names := make([]string, 0, len(keys))
	for k := range keys {
		names = append(names, k)
	}
	sort.Strings(names)
	b.WriteString(m.styles.Dim.Render("  Key bindings\n"))
	for _, n := range names {
		b.WriteString(fmt.Sprintf("    %-14s %s\n", n, m.styles.Key.Render(keys[n])))
	}
	return b.String()
}

func (m Model) viewHelp() string {
	var b strings.Builder
	b.WriteString(m.styles.Title.Render("my-orchestra — help") + "\n\n")
	lines := []string{
		"A cost-aware coding TUI fusing the GitHub Copilot CLI and Claude CLI,",
		"powered by the Copilot SDK and the my-ctx forge.",
		"",
		m.styles.Key.Render("tab / shift+tab") + "  cycle pages",
		m.styles.Key.Render("1..8") + "           jump (when not typing)",
		m.styles.Key.Render("enter") + "          send prompt (Chat) / toggle (lists)",
		m.styles.Key.Render("ctrl+j") + "         newline in the composer",
		m.styles.Key.Render("esc") + "            abort the current turn / cancel a form",
		m.styles.Key.Render("a / e / d") + "      add / edit / delete (Skills·Instructions·Agents)",
		m.styles.Key.Render("y / n") + "          approve / reject a tool-permission prompt",
		m.styles.Key.Render("ctrl+c") + "         quit",
		"",
		"Pages:",
		"  Chat         converse with the agent; tokens metered live",
		"  Telemetry    credits & per-model token breakdown vs. budget",
		"  Skills       toggle reusable prompt skills into context",
		"  Instructions toggle system-message rules",
		"  Agents       pick the active agent persona",
		"  Settings     view effective settings",
		"  Config       key bindings & paths",
		"",
		"Slash commands (type in the composer):",
		"  /help /clear /cost /skills /agents /settings",
		"  /model <name>   switch model + restart session",
		"  /agent <id>     switch agent + restart session",
		"  /attach <path>  attach a file/image to the next prompt",
		"  /resume [id]    resume the last (or given) session",
	}
	for _, l := range lines {
		b.WriteString("  " + l + "\n")
	}
	return b.String()
}

func yesno(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func indent(s, pad string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = pad + l
	}
	return strings.Join(lines, "\n")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}
