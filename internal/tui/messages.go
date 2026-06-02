package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dotts-h/copilot-sdk/internal/copilot"
)

// These tea.Msg types carry asynchronous results into the Bubble Tea update
// loop. Keeping them as plain structs makes Update fully unit-testable.

// sessionReadyMsg signals a session was created.
type sessionReadyMsg struct{ sessionID string }

// errMsg carries a recoverable error to display.
type errMsg struct{ err error }

// copilotEventMsg wraps a normalized client event.
type copilotEventMsg struct{ ev copilot.Event }

// streamDeltaMsg is a chunk of streamed assistant text.
type streamDeltaMsg struct{ text string }

// assistantDoneMsg marks the assistant turn complete.
type assistantDoneMsg struct{ content string }

// usageMsg carries token accounting for the telemetry meter.
type usageMsg struct{ usage copilot.UsageData }

// permMsg carries a tool-permission request awaiting a decision.
type permMsg struct{ req copilot.PermissionRequest }

// toolMsg reports a tool execution boundary.
type toolMsg struct {
	name  string
	start bool
}

// listenForEvents returns a command that waits for the next event on the client
// stream and re-arms itself, turning the channel into a stream of tea.Msgs.
func listenForEvents(c copilot.Client) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-c.Events()
		if !ok {
			return nil
		}
		return copilotEventMsg{ev: ev}
	}
}

// decodeEvent translates a normalized client event into a specific tea.Msg.
func decodeEvent(ev copilot.Event) tea.Msg {
	switch ev.Type {
	case copilot.EvMessageDelta, copilot.EvReasoningDelta:
		return streamDeltaMsg{text: ev.Text}
	case copilot.EvMessage:
		return assistantDoneMsg{content: ev.Text}
	case copilot.EvUsage:
		return usageMsg{usage: ev.Usage}
	case copilot.EvToolStart:
		return toolMsg{name: ev.Tool, start: true}
	case copilot.EvToolEnd:
		return toolMsg{name: ev.Tool, start: false}
	case copilot.EvIdle:
		return assistantDoneMsg{}
	case copilot.EvPermission:
		if ev.Permission != nil {
			return permMsg{req: *ev.Permission}
		}
		return nil
	case copilot.EvError:
		return errMsg{err: ev.Err}
	default:
		return nil
	}
}

// respondPermission returns a command that answers a permission request.
func respondPermission(c copilot.Client, id string, approve bool) tea.Cmd {
	return func() tea.Msg {
		if err := c.Respond(id, approve); err != nil {
			return errMsg{err: err}
		}
		return nil
	}
}

// sendPrompt returns a command that submits a prompt to the session.
func sendPrompt(c copilot.Client, sessionID, prompt string) tea.Cmd {
	return func() tea.Msg {
		if err := c.Send(context.Background(), sessionID, prompt); err != nil {
			return errMsg{err: err}
		}
		return nil
	}
}
