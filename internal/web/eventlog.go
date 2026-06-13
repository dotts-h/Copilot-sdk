// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

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
// Lock order: this method takes s.mu to snapshot the run ID, lane attribution,
// and log pointer (creating the log lazily when the run ID changes). It releases
// s.mu before the background goroutine does any IO. The AppendOnlyStore[RunEvent]
// is goroutine-safe for concurrent Append calls. This respects the forgeMu → s.mu
// order: no forgeMu is needed here.
func (s *Server) appendRunEvent(e copilot.Event) {
	if s.eventLogDir == "" {
		return // event log disabled — no-op
	}
	// A sub-agent-tagged event is routed only to the registry (ADR-0040/ADR-0041);
	// it is not a run-level event, so we don't log it here.
	if e.AgentID != "" {
		return
	}

	// Snapshot under s.mu: get the active run's ID, lane attribution, and the
	// log pointer, creating the log if the run id changed (new run).
	// We do NOT skip run.done here: the terminal event (EvIdle / EvError that
	// flips done) is already recorded in handleEvent before appendRunEvent is
	// called, so run.done==true at this point means we are logging that final
	// event — it belongs in the log.
	s.mu.Lock()
	run := s.run
	if run == nil {
		// No active run — nothing to log.
		s.mu.Unlock()
		return
	}
	runID := run.runID
	// Resolve lane attribution while holding s.mu, so laneFor reads lane.sessionID
	// and lane.status consistently (both are mutated under s.mu by launchLane and
	// the run state machine). This keeps laneFor off any concurrent mutation path.
	laneIndex := -1 // -1 = unattributed (no lane resolved for this event)
	if l := run.laneFor(e.SessionID); l != nil {
		laneIndex = l.Index
	}
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

	ev := normalizeRunEvent(e, runID, laneIndex, s.meter.PriceBook())

	// Append in a goroutine so the pump never blocks on disk IO.
	go func() {
		if err := log.Append(ev); err != nil {
			s.logger.Printf("append run event %q: %v", runID, err)
		}
	}()
}

// normalizeRunEvent maps a copilot.Event to a telemetry.RunEvent for the log.
// It extracts the fields relevant for replay/audit, keyed by runID and
// the pre-resolved laneIndex (computed under s.mu by the caller). Pure: no IO,
// no locking.
//
// laneIndex is -1 when the event could not be attributed to any lane (no lane
// resolved), which round-trips as -1 in JSON. Lane 0 is stored as laneIndex=0
// so lane 0 events are distinguishable from unattributed events.
//
// pb is the meter's price book, used only to price an EvUsage turn whose
// authoritative cost the runtime did not report — passing data (not a live meter)
// keeps the function pure.
func normalizeRunEvent(e copilot.Event, runID string, laneIndex int, pb *telemetry.PriceBook) telemetry.RunEvent {
	ev := telemetry.RunEvent{
		At:        time.Now(),
		RunID:     runID,
		Type:      eventTypeName(e.Type),
		LaneIndex: laneIndex,
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
	case copilot.EvUsage:
		// Price the turn at time of use so the timeline carries the credits the meter
		// actually computed — never recomputed from a later price book (issue 0092).
		ev.TokensIn = e.Usage.InputTokens
		ev.TokensOut = e.Usage.OutputTokens
		ev.Credits = usageCredits(pb, e.Usage)
	case copilot.EvError:
		if e.Err != nil {
			ev.Err = e.Err.Error()
		}
	}
	return ev
}

// usageCredits computes the credits to stamp into an EvUsage run event: the
// price-book estimate for the turn, frozen at log time so a later price-book change
// (or live repricing) can't reprice history (issue 0092). It deliberately mirrors
// the figure recordUsage adds to the lane (run_adapter.go EvUsage: `l.credits +=
// cost.Credits()`, the estimate from telemetry.Price) — NOT the reported-AIU
// authoritative basis — so the logged per-turn credits sum back to RunRecord.Credits
// on the SAME basis, and the inspector's per-run cross-check ambers only on a genuine
// log↔record drift, never on the estimate-vs-reported gap that the Telemetry page's
// ModelDrift table already surfaces (ADR-0033). Pure: no IO, no lock.
func usageCredits(pb *telemetry.PriceBook, u copilot.UsageData) float64 {
	return telemetry.Price(pb, telemetry.Usage{
		Model:            u.Model,
		InputTokens:      u.InputTokens,
		CachedTokens:     u.CachedTokens,
		CacheWriteTokens: u.CacheWriteTokens,
		OutputTokens:     u.OutputTokens,
		ReasoningTokens:  u.ReasoningTokens,
	}).Credits()
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
	case copilot.EvUserInput:
		return "EvUserInput"
	case copilot.EvPlanReview:
		return "EvPlanReview"
	case copilot.EvPlanChanged:
		return "EvPlanChanged"
	case copilot.EvElicitation:
		return "EvElicitation"
	case copilot.EvUserMessage:
		return "EvUserMessage"
	default:
		return "EvUnknown"
	}
}
