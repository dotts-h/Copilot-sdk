package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/dotts-h/copilot-sdk/internal/config"
	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// Page identifies a top-level screen.
type Page int

const (
	PageChat Page = iota
	PageTelemetry
	PageSkills
	PageInstructions
	PageAgents
	PageSettings
	PageConfig
	PageHelp
	numPages
)

func (p Page) String() string {
	switch p {
	case PageChat:
		return "Chat"
	case PageTelemetry:
		return "Telemetry"
	case PageSkills:
		return "Skills"
	case PageInstructions:
		return "Instructions"
	case PageAgents:
		return "Agents"
	case PageSettings:
		return "Settings"
	case PageConfig:
		return "Config"
	case PageHelp:
		return "Help"
	default:
		return "?"
	}
}

// Deps bundles the collaborators the TUI needs, so tests can inject fakes.
type Deps struct {
	Config *config.Config
	Forge  *ctxforge.Forge
	Client copilot.Client
	Meter  *telemetry.Meter
}

// Model is the root Bubble Tea model.
type Model struct {
	deps   Deps
	styles Styles

	width, height int
	page          Page
	ready         bool // session created
	sized         bool // first WindowSizeMsg received; viewport constructed

	chat      chatState
	input     textarea.Model
	viewport  viewport.Model
	sessionID string
	thinking  bool

	// list cursors per page
	cursor map[Page]int

	status   string
	errText  string
	quitting bool
}

// New builds the root model from its dependencies.
func New(d Deps) Model {
	ta := textarea.New()
	ta.Placeholder = "Ask anything — Enter to send, Tab to switch pages…"
	ta.Prompt = "┃ "
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	ta.SetHeight(3)
	ta.Focus()

	st := NewStyles(DarkPalette())
	return Model{
		deps:   d,
		styles: st,
		input:  ta,
		cursor: map[Page]int{},
		status: "ready",
	}
}

// Init starts the session and the event pump.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.createSessionCmd(),
		listenForEvents(m.deps.Client),
		textarea.Blink,
	)
}

// createSessionCmd compiles forge context and opens a session.
func (m Model) createSessionCmd() tea.Cmd {
	spec := m.buildSpec()
	c := m.deps.Client
	return func() tea.Msg {
		id, err := c.CreateSession(context.Background(), spec)
		if err != nil {
			return errMsg{err: err}
		}
		return sessionReadyMsg{sessionID: id}
	}
}

// buildSpec merges config + forge into a wire SessionSpec.
func (m Model) buildSpec() copilot.SessionSpec {
	spec := copilot.SessionSpec{
		Model:            m.deps.Config.DefaultModel,
		ReasoningEffort:  m.deps.Config.ReasoningEffort,
		Streaming:        m.deps.Config.Streaming,
		AutoApproveTools: m.deps.Config.AutoApproveTools,
	}
	if m.deps.Forge != nil {
		if compiled, err := m.deps.Forge.Compile(m.deps.Config.DefaultAgent); err == nil {
			if compiled.Model != "" {
				spec.Model = compiled.Model
			}
			if compiled.ReasoningEffort != "" {
				spec.ReasoningEffort = compiled.ReasoningEffort
			}
			spec.SystemMessage = compiled.SystemMessage
			for _, s := range compiled.MCPServers {
				spec.MCPServers = append(spec.MCPServers, copilot.MCPServer{
					Name: s.Name, Command: s.Command, Args: s.Args, Env: s.Env,
				})
			}
		}
	}
	return spec
}

// Update is the central reducer.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.onResize(msg), nil

	case tea.KeyMsg:
		return m.onKey(msg)

	case sessionReadyMsg:
		m.sessionID = msg.sessionID
		m.ready = true
		m.status = "session " + short(msg.sessionID)
		m.chat.addSystem("Connected. Session " + short(msg.sessionID) + " ready.")
		m.refreshViewport()
		return m, nil

	case copilotEventMsg:
		// Decode and re-arm the listener.
		next := decodeEvent(msg.ev)
		cmds := []tea.Cmd{listenForEvents(m.deps.Client)}
		if next != nil {
			m2, cmd := m.Update(next)
			mm := m2.(Model)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			return mm, tea.Batch(cmds...)
		}
		return m, tea.Batch(cmds...)

	case streamDeltaMsg:
		m.thinking = true
		m.chat.appendDelta(msg.text)
		m.refreshViewport()
		return m, nil

	case assistantDoneMsg:
		m.chat.finish(msg.content)
		m.thinking = false
		m.status = "ready"
		m.refreshViewport()
		return m, nil

	case usageMsg:
		u := msg.usage
		// OutputTokens already accounts for billable generation including
		// reasoning, so it is recorded as-is (ReasoningTokens is reported
		// separately for visibility, not added, to avoid double-counting).
		m.deps.Meter.Record(telemetry.Usage{
			Model:        u.Model,
			InputTokens:  u.InputTokens,
			CachedTokens: u.CachedTokens,
			OutputTokens: u.OutputTokens,
		})
		// Capture GitHub's authoritative cost (nano-AIU -> AIU) when present.
		m.deps.Meter.RecordReportedAIU(u.NanoAIU * 1e-9)
		return m, nil

	case toolMsg:
		if msg.start {
			m.chat.toolStart(msg.name)
			m.status = "running " + msg.name
		} else {
			m.chat.toolEnd(msg.name)
		}
		return m, nil

	case errMsg:
		if msg.err != nil {
			m.errText = msg.err.Error()
			m.chat.addSystem("⚠ " + msg.err.Error())
			m.refreshViewport()
		}
		return m, nil
	}

	// Delegate to the focused input on the chat page.
	if m.page == PageChat {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) onResize(msg tea.WindowSizeMsg) Model {
	m.width, m.height = msg.Width, msg.Height
	contentH := m.height - 6 // tabs + footer + input
	if contentH < 3 {
		contentH = 3
	}
	if !m.sized {
		m.viewport = viewport.New(msg.Width-2, contentH)
		m.sized = true
	} else {
		m.viewport.Width = msg.Width - 2
		m.viewport.Height = contentH
	}
	m.input.SetWidth(msg.Width - 2)
	m.refreshViewport()
	return m
}

// onKey routes key presses: global navigation first, then page-specific.
func (m Model) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "tab":
		m.page = (m.page + 1) % numPages
		m.syncFocus()
		return m, nil
	case "shift+tab":
		m.page = (m.page - 1 + numPages) % numPages
		m.syncFocus()
		return m, nil
	}

	switch m.page {
	case PageChat:
		return m.onChatKey(msg)
	case PageSkills, PageInstructions, PageAgents:
		return m.onListKey(msg), nil
	default:
		return m, nil
	}
}

func (m Model) onChatKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		text := strings.TrimSpace(m.input.Value())
		if text == "" || !m.ready || m.sessionID == "" {
			return m, nil
		}
		m.chat.addUser(text)
		m.input.Reset()
		m.thinking = true
		m.status = "thinking…"
		m.errText = ""
		m.refreshViewport()
		return m, sendPrompt(m.deps.Client, m.sessionID, text)
	case "ctrl+j":
		m.input.InsertString("\n")
		return m, nil
	case "esc":
		if m.deps.Client != nil && m.sessionID != "" {
			_ = m.deps.Client.Abort(context.Background(), m.sessionID)
			m.status = "aborted"
			m.thinking = false
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// onListKey handles selection + toggling on the list pages.
func (m Model) onListKey(msg tea.KeyMsg) Model {
	n := m.listLen()
	cur := m.cursor[m.page]
	switch msg.String() {
	case "up", "k":
		cur--
	case "down", "j":
		cur++
	case " ", "x", "enter":
		m.toggleCurrent()
	}
	if n == 0 {
		cur = 0
	} else {
		if cur < 0 {
			cur = 0
		}
		if cur >= n {
			cur = n - 1
		}
	}
	m.cursor[m.page] = cur
	return m
}

func (m Model) listLen() int {
	switch m.page {
	case PageSkills:
		return len(m.deps.Forge.Skills)
	case PageInstructions:
		return len(m.deps.Forge.Instructions)
	case PageAgents:
		return len(m.deps.Forge.Agents)
	}
	return 0
}

// toggleCurrent flips the enabled flag of the selected skill/instruction (agents
// are selected as the default agent) and persists the forge.
func (m *Model) toggleCurrent() {
	cur := m.cursor[m.page]
	switch m.page {
	case PageSkills:
		if cur >= 0 && cur < len(m.deps.Forge.Skills) {
			m.deps.Forge.Skills[cur].Enabled = !m.deps.Forge.Skills[cur].Enabled
			m.persistForge()
		}
	case PageInstructions:
		if cur >= 0 && cur < len(m.deps.Forge.Instructions) {
			m.deps.Forge.Instructions[cur].Enabled = !m.deps.Forge.Instructions[cur].Enabled
			m.persistForge()
		}
	case PageAgents:
		if cur >= 0 && cur < len(m.deps.Forge.Agents) {
			id := m.deps.Forge.Agents[cur].ID
			if m.deps.Config.DefaultAgent == id {
				m.deps.Config.DefaultAgent = ""
			} else {
				m.deps.Config.DefaultAgent = id
			}
			_ = m.deps.Config.Save()
			m.status = "default agent: " + def(m.deps.Config.DefaultAgent, "none")
		}
	}
}

func (m *Model) persistForge() {
	if err := m.deps.Forge.Save(); err != nil {
		m.errText = err.Error()
	} else {
		m.status = "forge saved"
	}
}

// syncFocus focuses the input only on the chat page.
func (m *Model) syncFocus() {
	if m.page == PageChat {
		m.input.Focus()
	} else {
		m.input.Blur()
	}
}

func short(s string) string {
	if len(s) <= 12 {
		return s
	}
	return s[:12]
}

func def(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// View renders the whole screen.
func (m Model) View() string {
	if m.quitting {
		return m.styles.Dim.Render("orchestra stopped — credits this session: ") +
			m.styles.Good.Render(telemetry.FormatCredits(m.deps.Meter.Totals().Credits())) + "\n"
	}
	if m.width == 0 {
		return "starting my-orchestra…"
	}
	var b strings.Builder
	b.WriteString(m.renderTabs())
	b.WriteString("\n")
	b.WriteString(m.renderBody())
	b.WriteString("\n")
	b.WriteString(m.renderFooter())
	return b.String()
}

func (m Model) renderTabs() string {
	var tabs []string
	for p := Page(0); p < numPages; p++ {
		label := fmt.Sprintf("%d %s", int(p)+1, p.String())
		if p == m.page {
			tabs = append(tabs, m.styles.TabActive.Render(label))
		} else {
			tabs = append(tabs, m.styles.TabInactive.Render(label))
		}
	}
	title := m.styles.Title.Render("⎈ my-orchestra ")
	return title + strings.Join(tabs, " ")
}

func (m Model) renderBody() string {
	switch m.page {
	case PageChat:
		return m.viewChat()
	case PageTelemetry:
		return m.viewTelemetry()
	case PageSkills, PageInstructions, PageAgents:
		return m.viewList()
	case PageSettings:
		return m.viewSettings()
	case PageConfig:
		return m.viewConfig()
	case PageHelp:
		return m.viewHelp()
	}
	return ""
}

func (m Model) renderFooter() string {
	totals := m.deps.Meter.Totals()
	budget := telemetry.Budget{AllowanceCredits: m.deps.Config.Telemetry.MonthlyCreditAllowance}
	frac := budget.FractionUsed(totals.Credits())
	usage := fmt.Sprintf("%s  %s  used %s",
		m.styles.Key.Render(telemetry.FormatCredits(totals.Credits())),
		bar(m.styles, frac, 12),
		m.styles.Dim.Render(fmt.Sprintf("%.0f%%", frac*100)),
	)
	left := m.styles.Footer.Render("tab: pages • enter: send • ctrl+c: quit")
	mid := m.styles.Footer.Render(" • " + m.status)
	if m.errText != "" {
		mid = m.styles.Bad.Render(" • " + m.errText)
	}
	line := left + mid
	pad := m.width - lipglossWidth(line) - lipglossWidth(usage)
	if pad < 1 {
		pad = 1
	}
	return line + strings.Repeat(" ", pad) + usage
}
