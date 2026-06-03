package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// applyAgentMsg reduces session-lifecycle and agent-stream messages (the events
// that originate from the copilot.Client, plus the session-ready signal) into
// model state. The boolean reports whether msg was one of those messages; when
// false, Update falls through to input/navigation handling.
//
// Keeping this reducer separate from Update isolates the agent event loop — the
// streaming transcript, tool timeline, telemetry, and permission queue — from
// the UI's input and page-navigation concerns.
func (m Model) applyAgentMsg(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case sessionReadyMsg:
		m.sessionID = msg.sessionID
		m.ready = true
		m.status = "session " + short(msg.sessionID)
		m.chat.addSystem("Connected. Session " + short(msg.sessionID) + " ready.")
		m.refreshViewport()

	case streamDeltaMsg:
		m.thinking = true
		m.chat.appendDelta(msg.text)
		m.refreshViewport()

	case reasoningDeltaMsg:
		m.thinking = true
		m.chat.appendReasoning(msg.text)
		m.refreshViewport()

	case assistantDoneMsg:
		m.chat.finish(msg.content)
		m.thinking = false
		m.status = "ready"
		m.refreshViewport()

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

	case permMsg:
		m.permQueue = append(m.permQueue, msg.req)
		m.page = PageChat
		m.status = "permission requested"
		m.chat.addSystem("⚠ permission: " + msg.req.Detail)
		m.refreshViewport()

	case toolStartMsg:
		m.thinking = true
		m.chat.toolStart(msg.id, msg.name, msg.args)
		m.status = "running " + msg.name
		m.refreshViewport()

	case toolProgressMsg:
		m.chat.toolProgress(msg.id, msg.text)
		m.refreshViewport()

	case toolEndMsg:
		m.chat.toolEnd(msg.id, msg.result, msg.success)
		if active := m.chat.activeTools(); len(active) > 0 {
			m.status = "running " + active[len(active)-1]
		} else {
			m.status = "thinking…"
		}
		m.refreshViewport()

	case errMsg:
		if msg.err != nil {
			m.errText = msg.err.Error()
			m.chat.addSystem("⚠ " + msg.err.Error())
			m.refreshViewport()
		}

	default:
		return m, nil, false
	}
	return m, nil, true
}
