package tui

import (
	"context"
	"encoding/json"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dotts-h/copilot-sdk/internal/copilot"
)

// These tea.Msg types carry asynchronous results into the Bubble Tea update
// loop. Keeping them as plain structs makes Update fully unit-testable.

// sessionReadyMsg signals a session was created.
type sessionReadyMsg struct{ sessionID string }

// errMsg carries a recoverable error to display.
type errMsg struct{ err error }

// copilotEventMsg wraps a decoded sidecar event.
type copilotEventMsg struct{ ev copilot.Event }

// streamDeltaMsg is a chunk of streamed assistant text.
type streamDeltaMsg struct{ text string }

// assistantDoneMsg marks the assistant turn complete.
type assistantDoneMsg struct{ content string }

// usageMsg carries token accounting for the telemetry meter.
type usageMsg struct{ usage copilot.UsageData }

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

// decodeEvent translates a raw sidecar event into a specific tea.Msg.
func decodeEvent(ev copilot.Event) tea.Msg {
	switch ev.Type {
	case copilot.EventMessageDelta:
		var d copilot.DeltaData
		_ = json.Unmarshal(ev.Data, &d)
		return streamDeltaMsg{text: d.DeltaContent}
	case copilot.EventMessage:
		var d copilot.MessageData
		_ = json.Unmarshal(ev.Data, &d)
		return assistantDoneMsg{content: d.Content}
	case copilot.EventUsage:
		var u copilot.UsageData
		_ = json.Unmarshal(ev.Data, &u)
		return usageMsg{usage: u}
	case copilot.EventToolStart:
		var d copilot.ToolData
		_ = json.Unmarshal(ev.Data, &d)
		return toolMsg{name: d.ToolName, start: true}
	case copilot.EventToolComplete:
		var d copilot.ToolData
		_ = json.Unmarshal(ev.Data, &d)
		return toolMsg{name: d.ToolName, start: false}
	case copilot.EventIdle:
		return assistantDoneMsg{}
	case copilot.EventError:
		var d copilot.ErrorData
		_ = json.Unmarshal(ev.Data, &d)
		return errMsg{err: errString(d.Message)}
	default:
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

// errString is a tiny error type for decoded error messages.
type errString string

func (e errString) Error() string { return string(e) }
