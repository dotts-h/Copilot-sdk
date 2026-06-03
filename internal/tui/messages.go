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

// streamDeltaMsg is a chunk of streamed assistant message text.
type streamDeltaMsg struct{ text string }

// reasoningDeltaMsg is a chunk of streamed (or a full block of) reasoning text.
type reasoningDeltaMsg struct{ text string }

// assistantDoneMsg marks the assistant turn complete.
type assistantDoneMsg struct{ content string }

// usageMsg carries token accounting for the telemetry meter.
type usageMsg struct{ usage copilot.UsageData }

// permMsg carries a tool-permission request awaiting a decision.
type permMsg struct{ req copilot.PermissionRequest }

// toolStartMsg reports a tool execution beginning, with its summarized args.
type toolStartMsg struct{ id, name, args string }

// toolProgressMsg carries a running tool's latest progress message.
type toolProgressMsg struct{ id, text string }

// toolEndMsg reports a tool execution completing with its result and outcome.
type toolEndMsg struct {
	id      string
	result  string
	success bool
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
	case copilot.EvMessageDelta:
		return streamDeltaMsg{text: ev.Text}
	case copilot.EvReasoningDelta, copilot.EvReasoning:
		return reasoningDeltaMsg{text: ev.Text}
	case copilot.EvMessage:
		return assistantDoneMsg{content: ev.Text}
	case copilot.EvUsage:
		return usageMsg{usage: ev.Usage}
	case copilot.EvToolStart:
		if ev.ToolCall != nil {
			return toolStartMsg{id: ev.ToolCall.ID, name: ev.ToolCall.Name, args: ev.ToolCall.Args}
		}
		return toolStartMsg{name: ev.Tool}
	case copilot.EvToolProgress:
		if ev.ToolCall != nil {
			return toolProgressMsg{id: ev.ToolCall.ID, text: ev.ToolCall.Progress}
		}
		return nil
	case copilot.EvToolEnd:
		if ev.ToolCall != nil {
			return toolEndMsg{id: ev.ToolCall.ID, result: ev.ToolCall.Result, success: ev.ToolCall.Success}
		}
		return toolEndMsg{success: true}
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

// sendPrompt returns a command that submits a prompt (with optional attachment
// paths) to the session.
func sendPrompt(c copilot.Client, sessionID, prompt string, attachments []string) tea.Cmd {
	return func() tea.Msg {
		if err := c.Send(context.Background(), sessionID, prompt, attachments); err != nil {
			return errMsg{err: err}
		}
		return nil
	}
}

// resumeSession returns a command that resumes a session (or the last one when
// id is empty) and reports it ready.
func resumeSession(c copilot.Client, id string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		if id == "" {
			last, err := c.LastSessionID(ctx)
			if err != nil {
				return errMsg{err: err}
			}
			if last == "" {
				return errMsg{err: errString("no previous session to resume")}
			}
			id = last
		}
		resumed, err := c.ResumeSession(ctx, id)
		if err != nil {
			return errMsg{err: err}
		}
		return sessionReadyMsg{sessionID: resumed}
	}
}

// errString is a minimal error type for synthesized messages.
type errString string

func (e errString) Error() string { return string(e) }
