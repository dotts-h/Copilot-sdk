package web

import (
	"github.com/dotts-h/copilot-sdk/internal/convo"
	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// liveKind tracks what the trailing #cur node currently represents in the
// browser, so streamed deltas can be appended in place (the fast path) and a
// full timeline re-render is only emitted when the kind changes or the structure
// does (tool start/end, finish, system note).
type liveKind int

const (
	liveNone liveKind = iota
	liveMessage
	liveReasoning
)

// handleEvent reduces one normalized copilot.Event into convo state and returns
// the SSE fragments to emit. High-frequency message/reasoning deltas append to
// #cur; structural events re-render #timeline; usage updates the cost footer;
// permission requests append an inline form. Mirrors the TUI's applyAgentMsg
// (internal/tui/events.go) but emits HTML instead of mutating a Bubble Tea model.
func (s *Server) handleEvent(e copilot.Event) []fragment {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch e.Type {
	case copilot.EvMessageDelta:
		prev := s.live
		s.state.AppendDelta(e.Text)
		if prev == liveMessage {
			return []fragment{{Event: "delta", HTML: deltaSpan(e.Text)}}
		}
		s.live = liveMessage
		return s.timelineFragments()

	case copilot.EvReasoningDelta:
		prev := s.live
		s.state.AppendReasoning(e.Text)
		if prev == liveReasoning {
			return []fragment{{Event: "delta", HTML: deltaSpan(e.Text)}}
		}
		s.live = liveReasoning
		return s.timelineFragments()

	case copilot.EvReasoning: // full reasoning block (non-streaming)
		s.state.AppendReasoning(e.Text)
		s.live = liveReasoning
		return s.timelineFragments()

	case copilot.EvMessage:
		s.state.Finish(e.Text)
		s.live = liveNone
		return append(s.timelineFragments(), fragment{Event: "status", HTML: "ready"})

	case copilot.EvToolStart:
		args := ""
		if e.ToolCall != nil {
			args = e.ToolCall.Args
		}
		s.state.ToolStart(toolID(e), e.Tool, args)
		s.live = liveNone
		return append(s.timelineFragments(), fragment{Event: "status", HTML: "running " + esc(e.Tool)})

	case copilot.EvToolProgress:
		if e.ToolCall != nil {
			s.state.ToolProgress(e.ToolCall.ID, e.ToolCall.Progress)
		}
		return s.timelineFragments()

	case copilot.EvToolEnd:
		if e.ToolCall != nil {
			s.state.ToolEnd(e.ToolCall.ID, e.ToolCall.Result, e.ToolCall.Success)
		}
		s.live = liveNone
		st := "thinking…"
		if active := s.state.ActiveTools(); len(active) > 0 {
			st = "running " + active[len(active)-1]
		}
		return append(s.timelineFragments(), fragment{Event: "status", HTML: esc(st)})

	case copilot.EvIdle:
		s.state.Finish("")
		s.live = liveNone
		return append(s.timelineFragments(), fragment{Event: "status", HTML: ""})

	case copilot.EvUsage:
		s.meter.Record(telemetry.Usage{
			Model:        e.Usage.Model,
			InputTokens:  e.Usage.InputTokens,
			CachedTokens: e.Usage.CachedTokens,
			OutputTokens: e.Usage.OutputTokens,
		})
		s.meter.RecordReportedAIU(e.Usage.NanoAIU * 1e-9)
		return []fragment{{Event: "cost", HTML: renderCostFooter(s.meter, s.allowance)}}

	case copilot.EvPermission:
		if e.Permission == nil {
			return nil
		}
		s.perms = append(s.perms, *e.Permission)
		return []fragment{
			{Event: "perm", HTML: renderPermForm(e.Permission.ID, e.Permission.Detail)},
			{Event: "status", HTML: "permission requested"},
		}

	case copilot.EvError:
		msg := "unknown error"
		if e.Err != nil {
			msg = e.Err.Error()
		}
		s.state.AddSystem("⚠ " + msg)
		s.live = liveNone
		return s.timelineFragments()

	default:
		return nil
	}
}

// timelineFragments re-renders the whole #timeline and resets the live-kind to
// match the (possibly just-committed) in-flight buffer.
func (s *Server) timelineFragments() []fragment {
	role, text := s.state.Pending()
	if text == "" {
		s.live = liveNone
	} else if role == convo.RoleReasoning {
		s.live = liveReasoning
	} else {
		s.live = liveMessage
	}
	return []fragment{{Event: "timeline", HTML: renderTimelineInner(&s.state)}}
}

// toolID returns a stable id for a tool event, falling back to the tool name.
func toolID(e copilot.Event) string {
	if e.ToolCall != nil && e.ToolCall.ID != "" {
		return e.ToolCall.ID
	}
	return e.Tool
}
