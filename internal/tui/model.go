package tui

import (
	"context"
	"fmt"
	"os"
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
	// Resume, when true, resumes the last session on launch instead of creating
	// a fresh one.
	Resume bool
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

	// form is the active modal editor (nil when not editing).
	form *forgeForm

	// permQueue holds pending tool-permission prompts (FIFO); the head is shown.
	permQueue []copilot.PermissionRequest

	// pendingAttachments are file/image paths attached to the next prompt.
	pendingAttachments []string

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
	start := m.createSessionCmd()
	if m.deps.Resume {
		start = resumeSession(m.deps.Client, "")
	}
	return tea.Batch(
		start,
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

// Update is the central reducer. It routes input and navigation, re-arms the
// agent event pump, and delegates session/agent-stream reduction to
// applyAgentMsg (events.go), keeping each concern in one place.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.onResize(msg), nil

	case tea.KeyMsg:
		return m.onKey(msg)

	case copilotEventMsg:
		// Decode the normalized event and re-arm the listener so the client's
		// event channel becomes an endless stream of messages.
		cmds := []tea.Cmd{listenForEvents(m.deps.Client)}
		if next := decodeEvent(msg.ev); next != nil {
			m2, cmd := m.Update(next)
			m = m2.(Model)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		return m, tea.Batch(cmds...)
	}

	// Session-lifecycle and agent-stream messages are reduced in events.go.
	if mm, cmd, handled := m.applyAgentMsg(msg); handled {
		return mm, cmd
	}

	// Anything else: delegate to the focused input on the chat page.
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

// onKey routes key presses: an open form captures everything; otherwise global
// navigation first, then page-specific.
func (m Model) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		m.quitting = true
		return m, tea.Quit
	}
	if m.form != nil {
		return m.onFormKey(msg), nil
	}

	switch msg.String() {
	case "tab":
		m.page = (m.page + 1) % numPages
		m.syncFocus()
		return m, nil
	case "shift+tab":
		m.page = (m.page - 1 + numPages) % numPages
		m.syncFocus()
		return m, nil
	}

	// A pending permission prompt captures y/n decisions on the chat page.
	if len(m.permQueue) > 0 && m.page == PageChat {
		switch msg.String() {
		case "y", "Y", "enter":
			return m.decidePermission(true)
		case "n", "N", "esc":
			return m.decidePermission(false)
		}
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

// decidePermission answers the head-of-queue permission request and advances.
func (m Model) decidePermission(approve bool) (tea.Model, tea.Cmd) {
	req := m.permQueue[0]
	m.permQueue = m.permQueue[1:]
	verb := "approved"
	if !approve {
		verb = "rejected"
	}
	m.chat.addSystem("permission " + verb + ": " + req.Detail)
	m.status = verb
	m.refreshViewport()
	return m, respondPermission(m.deps.Client, req.ID, approve)
}

// onFormKey drives the modal forge editor.
func (m Model) onFormKey(msg tea.KeyMsg) Model {
	switch msg.String() {
	case "esc":
		m.form = nil
		m.status = "edit cancelled"
		return m
	case "tab", "down":
		m.form.moveFocus(1)
		return m
	case "shift+tab", "up":
		m.form.moveFocus(-1)
		return m
	case "enter":
		if err := m.applyForm(); err != nil {
			m.errText = err.Error()
		} else {
			m.form = nil
		}
		return m
	}
	// Forward to the focused text input.
	var cmd tea.Cmd
	f := &m.form.fields[m.form.focus]
	f.input, cmd = f.input.Update(msg)
	_ = cmd
	return m
}

// applyForm builds the entity from the form and commits it to the forge,
// restoring prior state if the save fails validation.
func (m *Model) applyForm() error {
	entity, err := m.form.build()
	if err != nil {
		return err
	}
	f := m.deps.Forge
	idx := m.form.editIndex

	// Snapshot for rollback.
	restore := func() {}
	switch v := entity.(type) {
	case ctxforge.Skill:
		if idx < 0 {
			if err := f.AddSkill(v); err != nil {
				return err
			}
			restore = func() { f.Skills = f.Skills[:len(f.Skills)-1] }
		} else {
			old := f.Skills[idx]
			f.Skills[idx] = v
			restore = func() { f.Skills[idx] = old }
		}
	case ctxforge.Instruction:
		if idx < 0 {
			f.Instructions = append(f.Instructions, v)
			restore = func() { f.Instructions = f.Instructions[:len(f.Instructions)-1] }
		} else {
			old := f.Instructions[idx]
			f.Instructions[idx] = v
			restore = func() { f.Instructions[idx] = old }
		}
	case ctxforge.Agent:
		if idx < 0 {
			f.Agents = append(f.Agents, v)
			restore = func() { f.Agents = f.Agents[:len(f.Agents)-1] }
		} else {
			old := f.Agents[idx]
			f.Agents[idx] = v
			restore = func() { f.Agents[idx] = old }
		}
	}
	if err := f.Save(); err != nil {
		restore()
		return err
	}
	m.status = "saved"
	return nil
}

func (m Model) onChatKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		text := strings.TrimSpace(m.input.Value())
		if text == "" {
			return m, nil
		}
		if strings.HasPrefix(text, "/") {
			return m.handleSlash(text)
		}
		if !m.ready || m.sessionID == "" {
			return m, nil
		}
		att := m.pendingAttachments
		shown := text
		if len(att) > 0 {
			shown = text + "  " + m.styles.Dim.Render(fmt.Sprintf("[+%d attachment(s)]", len(att)))
		}
		m.chat.addUser(shown)
		m.input.Reset()
		m.pendingAttachments = nil
		m.thinking = true
		m.status = "thinking…"
		m.errText = ""
		m.refreshViewport()
		return m, sendPrompt(m.deps.Client, m.sessionID, text, att)
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

// handleSlash interprets a /command typed in the chat composer. It always
// resets the input and never sends the literal text to the agent.
func (m Model) handleSlash(text string) (tea.Model, tea.Cmd) {
	m.input.Reset()
	m.errText = ""
	parts := strings.Fields(text)
	cmd := strings.ToLower(parts[0])
	arg := strings.TrimSpace(strings.TrimPrefix(text, parts[0]))

	switch cmd {
	case "/help":
		m.page = PageHelp
	case "/clear":
		m.chat = chatState{}
		m.refreshViewport()
		m.status = "transcript cleared"
	case "/cost", "/telemetry":
		m.page = PageTelemetry
	case "/skills":
		m.page = PageSkills
	case "/agents":
		m.page = PageAgents
	case "/settings":
		m.page = PageSettings
	case "/attach":
		if arg == "" {
			m.chat.addSystem("usage: /attach <path>")
		} else if info, err := os.Stat(arg); err != nil || info.IsDir() {
			m.chat.addSystem("cannot attach " + arg + " (not a readable file)")
		} else {
			m.pendingAttachments = append(m.pendingAttachments, arg)
			m.chat.addSystem(fmt.Sprintf("attached %s (%d pending)", arg, len(m.pendingAttachments)))
		}
		m.refreshViewport()
	case "/resume":
		m.chat.addSystem("resuming session…")
		m.refreshViewport()
		return m, resumeSession(m.deps.Client, arg)
	case "/model":
		if arg == "" {
			m.chat.addSystem("usage: /model <name>")
		} else {
			m.deps.Config.DefaultModel = arg
			_ = m.deps.Config.Save()
			m.chat.addSystem("model set to " + arg + " — starting a new session")
			m.refreshViewport()
			return m, m.createSessionCmd()
		}
	case "/agent":
		if arg == "" {
			m.deps.Config.DefaultAgent = ""
		} else if m.deps.Forge.Agent(arg) == nil {
			m.chat.addSystem("unknown agent " + arg)
			m.refreshViewport()
			return m, nil
		} else {
			m.deps.Config.DefaultAgent = arg
		}
		_ = m.deps.Config.Save()
		m.chat.addSystem("agent set to " + def(m.deps.Config.DefaultAgent, "none") + " — starting a new session")
		m.refreshViewport()
		return m, m.createSessionCmd()
	default:
		m.chat.addSystem("unknown command " + cmd + " (try /help)")
		m.refreshViewport()
	}
	return m, nil
}

// onListKey handles selection, toggling, and CRUD on the list pages.
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
	case "a":
		m.openForm(-1)
		return m
	case "e":
		if n > 0 {
			m.openForm(cur)
		}
		return m
	case "d":
		m.deleteCurrent()
		if n > 0 {
			n--
		}
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

// openForm opens the add (index < 0) or edit form for the current page.
func (m *Model) openForm(index int) {
	f := m.deps.Forge
	switch m.page {
	case PageSkills:
		if index >= 0 && index < len(f.Skills) {
			m.form = newSkillForm(&f.Skills[index], index)
		} else {
			m.form = newSkillForm(nil, -1)
		}
	case PageInstructions:
		if index >= 0 && index < len(f.Instructions) {
			m.form = newInstructionForm(&f.Instructions[index], index)
		} else {
			m.form = newInstructionForm(nil, -1)
		}
	case PageAgents:
		if index >= 0 && index < len(f.Agents) {
			m.form = newAgentForm(&f.Agents[index], index)
		} else {
			m.form = newAgentForm(nil, -1)
		}
	}
	m.errText = ""
}

// deleteCurrent removes the selected entity and persists.
func (m *Model) deleteCurrent() {
	f := m.deps.Forge
	cur := m.cursor[m.page]
	switch m.page {
	case PageSkills:
		if cur >= 0 && cur < len(f.Skills) {
			f.Skills = append(f.Skills[:cur], f.Skills[cur+1:]...)
		}
	case PageInstructions:
		if cur >= 0 && cur < len(f.Instructions) {
			f.Instructions = append(f.Instructions[:cur], f.Instructions[cur+1:]...)
		}
	case PageAgents:
		if cur >= 0 && cur < len(f.Agents) {
			removed := f.Agents[cur].ID
			f.Agents = append(f.Agents[:cur], f.Agents[cur+1:]...)
			if m.deps.Config.DefaultAgent == removed {
				m.deps.Config.DefaultAgent = ""
				_ = m.deps.Config.Save()
			}
		}
	default:
		return
	}
	m.persistForge()
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
	if m.form != nil {
		return m.viewForm()
	}
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
