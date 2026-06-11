package web

import (
	"time"

	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// appendRunEvent records the event to the active run's per-run event log when
// the event log is enabled (s.eventLogDir != "") and a run is in flight. It is
// called from the hub's pump AFTER sv.handleEvent and sv.broadcast have returned
// — so it is OFF the hot-path critical section (s.mu is never held by the caller).
// The actual disk append runs in a background goroutine, so the pump never blocks
// on IO. — ADR-0048.
//
// Lock order: this method takes s.mu only to snapshot the run ID and get/create
// the log pointer; it never holds s.mu while doing IO (the goroutine does IO
// without any lock). The AppendOnlyStore[RunEvent] is goroutine-safe for concurrent
// Append calls. This respects the forgeMu → s.mu order: no forgeMu is needed here.
func (s *Server) appendRunEvent(e copilot.Event) {
	if s.eventLogDir == "" {
		return // event log disabled — no-op
	}
	// A sub-agent-tagged event is routed only to the registry (ADR-0040/ADR-0041);
	// it is not a run-level event, so we don't log it here.
	if e.AgentID != "" {
		return
	}

	// Snapshot under s.mu: get the active run's ID and the log pointer,
	// creating the log if the run id changed (new run).
	s.mu.Lock()
	run := s.run
	if run == nil || run.done {
		// No active run — nothing to log.
		s.mu.Unlock()
		return
	}
	runID := run.runID
	// Lazily create or re-create the log when the run ID changes (a new run).
	if s.runEventLog == nil || s.runEventLogID != runID {
		log, err := telemetry.LoadRunEventLog(s.eventLogDir, runID)
		if err != nil {
			s.logger.Printf("open run event log %q: %v", runID, err)
			s.mu.Unlock()
			return
		}
		s.runEventLog = log
		s.runEventLogID = runID
	}
	log := s.runEventLog
	s.mu.Unlock()

	// Determine the lane index for the event (for attribution). laneFor is a
	// pure method on the run — safe to call without s.mu, because we already hold
	// a reference to the run and the log; laneFor only reads lane sessionIDs
	// (set before the run starts and never changed while running).
	// NOTE: we do NOT hold s.mu here — the IO must stay off the critical section.
	ev := normalizeRunEvent(e, run, runID)

	// Append in a goroutine so the pump never blocks on disk IO.
	go func() {
		if err := log.Append(ev); err != nil {
			s.logger.Printf("append run event %q: %v", runID, err)
		}
	}()
}

// normalizeRunEvent maps a copilot.Event to a telemetry.RunEvent for the log.
// It extracts the fields relevant for replay/audit, keyed by runID and
// optionally lane index. Pure: no IO, no locking.
func normalizeRunEvent(e copilot.Event, run *workflowRun, runID string) telemetry.RunEvent {
	ev := telemetry.RunEvent{
		At:    time.Now(),
		RunID: runID,
		Type:  eventTypeName(e.Type),
	}

	// Attribute to a lane when we can resolve it from the session id.
	if l := run.laneFor(e.SessionID); l != nil {
		ev.LaneIndex = l.Index
	}

	switch e.Type {
	case copilot.EvMessage, copilot.EvMessageDelta, copilot.EvReasoning, copilot.EvReasoningDelta:
		ev.Text = e.Text
	case copilot.EvToolStart:
		ev.Tool = e.Tool
		if e.ToolCall != nil {
			ev.Args = e.ToolCall.Args
		}
	case copilot.EvToolEnd:
		ev.Tool = e.Tool
		if e.ToolCall != nil {
			ev.Result = e.ToolCall.Result
			ev.Success = e.ToolCall.Success
		}
	case copilot.EvError:
		if e.Err != nil {
			ev.Err = e.Err.Error()
		}
	}
	return ev
}

// eventTypeName converts a copilot.EventType to its stable string name for the log.
// These names are the on-disk contract (pinned by TestRunEventLogOnDiskTagsAreStable).
func eventTypeName(t copilot.EventType) string {
	switch t {
	case copilot.EvMessage:
		return "EvMessage"
	case copilot.EvMessageDelta:
		return "EvMessageDelta"
	case copilot.EvReasoning:
		return "EvReasoning"
	case copilot.EvReasoningDelta:
		return "EvReasoningDelta"
	case copilot.EvToolStart:
		return "EvToolStart"
	case copilot.EvToolProgress:
		return "EvToolProgress"
	case copilot.EvToolEnd:
		return "EvToolEnd"
	case copilot.EvUsage:
		return "EvUsage"
	case copilot.EvIdle:
		return "EvIdle"
	case copilot.EvError:
		return "EvError"
	case copilot.EvPermission:
		return "EvPermission"
	case copilot.EvContextWindow:
		return "EvContextWindow"
	case copilot.EvCompactionStart:
		return "EvCompactionStart"
	case copilot.EvCompactionEnd:
		return "EvCompactionEnd"
	case copilot.EvSubagentStart:
		return "EvSubagentStart"
	case copilot.EvSubagentEnd:
		return "EvSubagentEnd"
	case copilot.EvToolDecision:
		return "EvToolDecision"
	case copilot.EvHookRun:
		return "EvHookRun"
	default:
		return "EvUnknown"
	}
}
