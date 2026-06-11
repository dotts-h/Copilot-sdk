package web

import (
	"context"

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
	// Route sub-agent-tagged events to the registry (epic 0069 S2, issue 0071,
	// ADR-0041). The SDK streams a sub-agent's deltas/tools/usage tagged with its
	// instance AgentID; folding them into the root transcript would render a
	// sub-agent's text in the user-facing bubble and meter its spend as the root
	// agent's (the S1 invariant, ADR-0040), so they reach ONLY the registry — its
	// live list is their surface. The guard precedes the lane router below, so a
	// sub-agent running during a workflow still feeds the list. The sub-agent
	// LIFECYCLE events (EvSubagentStart/End, AgentID empty — session-level) are
	// handled in the switch like before.
	if e.AgentID != "" {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.handleSubagentStream(e)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// While a multi-agent workflow run is in flight, its sub-runs' events feed the
	// lanes surface (item 2.1) rather than the main chat transcript.
	if s.run != nil && !s.run.done {
		return s.handleRunEvent(s.run, e)
	}

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
		return append(s.timelineFragments(), s.statusFrag("ready", false))

	case copilot.EvToolStart:
		args := ""
		if e.ToolCall != nil {
			args = e.ToolCall.Args
		}
		s.state.ToolStart(toolID(e), e.Tool, args)
		s.toolsUsed++
		s.live = liveNone
		return append(s.timelineFragments(), s.statusFrag("running "+e.Tool, true), s.statFrag())

	case copilot.EvToolProgress:
		if e.ToolCall != nil {
			s.state.ToolProgress(e.ToolCall.ID, e.ToolCall.Progress)
		}
		return s.timelineFragments()

	case copilot.EvToolDecision:
		// A governance hook auto-approved or denied a call without a gate; record
		// the "why" inline so the decision is explainable (ADR-0031).
		if e.Decision != nil {
			s.state.AddDecision(convo.DecisionView{
				Kind: e.Decision.Kind, HookID: e.Decision.HookID,
				Reason: e.Decision.Reason, Detail: e.Decision.Detail,
			})
		}
		return s.timelineFragments()

	case copilot.EvHookRun:
		// A PostToolUse hook ran an external command; record its bounded, untrusted
		// output inline as display-only telemetry (ADR-0032). It is not a gate.
		if e.HookRun != nil {
			s.state.AddHookRun(convo.HookRunView{
				HookID: e.HookRun.HookID, Command: e.HookRun.Command,
				Output: e.HookRun.Output, ExitCode: e.HookRun.ExitCode,
				TimedOut: e.HookRun.TimedOut, Failed: e.HookRun.Failed,
			})
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
		return append(s.timelineFragments(), s.statusFrag(st, true))

	case copilot.EvIdle:
		s.state.Finish("")
		s.live = liveNone
		// Drain the next queued prompt, if any (its user bubble is already in the
		// transcript — see handleSend). The session stays busy across the drain,
		// and the drained prompt starts a fresh turn for the elapsed timer.
		if len(s.queue) > 0 && s.sessionID != "" {
			next := s.queue[0]
			// Gate the queued turn on the agent leash too: the just-finished turn
			// may have pushed the persona over its cap, so re-check before draining
			// the next one (issue 0072).
			if g := s.pendingLeashGate(next, nil); g != nil {
				s.queue = s.queue[1:]
				s.gate = g
				s.turnStartMs = 0
				return append(s.timelineFragments(),
					s.budgetFrag(), s.statusFrag("agent leash reached — confirm to proceed", false))
			}
			// Gate the queued turn too: type-ahead must not slip an over-budget
			// prompt past the hard cap. The just-finished turn's usage/context is
			// already folded in, so the projection is fresh. Surface the gate over
			// SSE (the body-level `budget` listener) instead of dispatching.
			if projected, capped := s.overCap(); capped {
				s.queue = s.queue[1:]
				s.gate = &budgetGate{prompt: next, projected: projected, cap: s.hardCap}
				s.turnStartMs = 0
				return append(s.timelineFragments(),
					s.budgetFrag(), s.statusFrag("over budget cap — confirm to proceed", false))
			}
			s.queue = s.queue[1:]
			sid := s.sessionID
			s.turnStartMs = nowMs()
			// Drain on a goroutine so the event loop is not blocked; a Send failure
			// here would otherwise leave the turn with no events, so surface it over
			// SSE (the body-level listeners pick up the OOB fragments).
			go func() {
				if err := s.dispatch(context.Background(), sid, next, nil); err != nil {
					s.broadcastSendFailure(err)
				}
			}()
			st := "thinking…"
			if rem := len(s.queue); rem > 0 {
				st = queuedStatus(rem)
			}
			return append(s.timelineFragments(), s.statusFrag(st, true))
		}
		s.busy = false
		s.turnStartMs = 0
		return append(s.timelineFragments(), s.statusFrag("", false))

	case copilot.EvUsage:
		// Attribute the chat turn to the active agent persona (no workflow owns it).
		s.recordUsage(e.Usage, spendTag{agentID: s.agentID})
		return []fragment{
			{Event: "cost", HTML: renderActualCostFooter(s.monthToDateActual(), s.budget())},
			s.statFrag(),
		}

	case copilot.EvPermission:
		if e.Permission == nil {
			return nil
		}
		s.perms = append(s.perms, *e.Permission)
		return []fragment{
			{Event: "perm", HTML: renderPermForm(*e.Permission)},
			s.statusFrag("permission requested", true),
		}

	case copilot.EvUserInput:
		if e.Input == nil {
			return nil
		}
		s.inputs = append(s.inputs, *e.Input)
		return []fragment{
			{Event: "ask", HTML: renderAskForm(*e.Input)},
			s.statusFrag("input requested", true),
		}

	case copilot.EvPlanReview:
		if e.Plan == nil {
			return nil
		}
		s.plans = append(s.plans, *e.Plan)
		return []fragment{
			{Event: "plan", HTML: renderPlanForm(*e.Plan)},
			s.statusFrag("plan ready for review", true),
		}

	case copilot.EvElicitation:
		if e.Elicit == nil {
			return nil
		}
		s.elicits = append(s.elicits, *e.Elicit)
		return []fragment{
			{Event: "elicit", HTML: renderElicitForm(*e.Elicit)},
			s.statusFrag("input requested", true),
		}

	case copilot.EvPlanChanged:
		note := e.Text
		if note == "" {
			note = "plan changed"
		}
		s.state.AddSystem("◷ " + note)
		return s.timelineFragments()

	case copilot.EvSubagentStart:
		if e.Subagent == nil {
			return nil
		}
		sa := *e.Subagent
		s.subreg.Start(sa.ToolCallID, sa.Name, sa.DisplayName, sa.Description, sa.Model)
		s.state.AddSystem("▸ sub-agent " + subagentLabel(sa) + " started")
		return append(s.timelineFragments(), s.subagentsFrag())

	case copilot.EvSubagentEnd:
		if e.Subagent == nil {
			return nil
		}
		s.subreg.End(e.Subagent.ToolCallID, e.Subagent.Success, e.Subagent.Detail, e.Subagent.TotalTokens)
		glyph := "✓"
		if !e.Subagent.Success {
			glyph = "✗"
		}
		note := glyph + " sub-agent " + subagentLabel(*e.Subagent) + " finished"
		if e.Subagent.Detail != "" {
			note += " · " + e.Subagent.Detail
		}
		s.state.AddSystem(note)
		return append(s.timelineFragments(), s.subagentsFrag())

	case copilot.EvContextWindow:
		s.ctxCurrent = e.Context.CurrentTokens
		s.ctxLimit = e.Context.TokenLimit
		return []fragment{s.ctxFrag(), s.statFrag()}

	case copilot.EvCompactionStart:
		s.compacting = true
		s.state.AddSystem("✻ compacting conversation…")
		return append(s.timelineFragments(), s.ctxFrag())

	case copilot.EvCompactionEnd:
		s.compacting = false
		note := e.Text
		if note == "" {
			note = "compacted context"
		}
		s.state.AddSystem("✻ " + note)
		return append(s.timelineFragments(), s.ctxFrag())

	case copilot.EvError:
		msg := "unknown error"
		if e.Err != nil {
			msg = e.Err.Error()
		}
		s.state.AddSystem("⚠ " + msg)
		s.live = liveNone
		// An error ends the turn: clear the busy/queued state and the spinner so the
		// composer is not stuck waiting for events that will never arrive.
		s.busy = false
		s.queue = nil
		s.turnStartMs = 0
		return append(s.timelineFragments(), s.statusFrag("", false))

	default:
		return nil
	}
}

// spendTag attributes a metered turn to the agent persona (and, when a workflow
// run owns the turn, the workflow + lane within it) that incurred it, so the
// ledger answers "which agent / which workflow burned the budget" (ADR-0018). Its
// zero value — no agent, no workflow — is a plain unattributed chat turn.
type spendTag struct {
	agentID    string
	workflowID string
	laneIndex  int
	// subagentID/subagentName attribute the turn to a sub-agent INSTANCE (epic 0069
	// S3, issue 0072). When set, recordUsage prices the turn and appends a tagged
	// ledger record but does NOT fold it into the account-wide or per-session token
	// meters — a sub-agent's tokens are the sub-agent's, never metered as the root's
	// (the S1 invariant, ADR-0040).
	subagentID   string
	subagentName string
}

// recordUsage folds one metered turn into both meters and appends a SpendRecord
// to the persisted ledger, returning the priced cost. It is the single
// spend-recording path shared by the chat reducer (EvUsage above) and the
// workflow-lane reducer (workflow.go handleRunEvent): every turn must land in the
// account-wide meter (live token split), the per-session meter (statusline,
// ADR-0011), AND the ledger (account-wide budget accounting, ADR-0016) — drop any
// one and that surface silently drifts (REGRESSIONS "two meters", now three
// sources). The tag additively attributes the record to the agent/workflow that
// spent it (ADR-0018). Ledger persistence is best-effort: a disk error is logged,
// not surfaced, so the live meters and stream are unaffected. Caller holds s.mu.
func (s *Server) recordUsage(u copilot.UsageData, tag spendTag) telemetry.Cost {
	usage := telemetry.Usage{
		Model:            u.Model,
		InputTokens:      u.InputTokens,
		CachedTokens:     u.CachedTokens,
		CacheWriteTokens: u.CacheWriteTokens,
		OutputTokens:     u.OutputTokens,
		ReasoningTokens:  u.ReasoningTokens,
	}
	aiu := u.NanoAIU * 1e-9
	var cost telemetry.Cost
	if tag.subagentID != "" {
		// A sub-agent's tokens are the sub-agent's, not the root/session's: price the
		// turn for ledger attribution and the live registry row, but DON'T fold it
		// into the account-wide or per-session token meters — those are the root's
		// "this session" gauges and must stay free of sub-agent spend (the S1
		// invariant, ADR-0040). The ledger record below still carries the spend, so it
		// counts toward the account-wide budget and the per-sub-agent breakdown.
		cost = telemetry.Price(s.meter.PriceBook(), usage)
	} else {
		cost = s.meter.Record(usage)
		s.sessionMeter.Record(usage)
		// Fold GitHub's authoritative per-turn cost into BOTH meters: the account-wide
		// one (Telemetry page) and the per-session one (statusline), so each surface
		// prefers reported over estimate on its own scope — ADR-0033.
		s.meter.RecordReportedAIU(aiu)
		s.sessionMeter.RecordReportedAIU(aiu)
	}
	if s.spend != nil {
		rec := telemetry.SpendRecord{
			SessionID:        s.sessionID,
			Model:            u.Model,
			InputTokens:      u.InputTokens,
			CachedTokens:     u.CachedTokens,
			OutputTokens:     u.OutputTokens,
			CacheWriteTokens: u.CacheWriteTokens,
			ReasoningTokens:  u.ReasoningTokens,
			USD:              cost.USD(),
			AIU:              aiu,
			AgentID:          tag.agentID,
			WorkflowID:       tag.workflowID,
			LaneIndex:        tag.laneIndex,
			SubagentID:       tag.subagentID,
			SubagentName:     tag.subagentName,
		}
		if err := s.spend.Append(rec); err != nil {
			s.logger.Printf("persist spend: %v", err)
		}
	}
	// Accumulate the persona's running spend for the budget leash (issue 0072): a
	// persona-tagged turn (root chat or a workflow lane) counts toward its agent's
	// cap; a sub-agent turn (instance-tagged, persona empty) does not — its cap
	// rides S4.
	if tag.subagentID == "" && tag.agentID != "" {
		s.agentCredits[tag.agentID] += cost.Credits()
		s.agentTurns[tag.agentID]++
	}
	return cost
}

// leashFor reports the active persona's snapshot leash and whether it has already
// crossed it this session — the pre-dispatch budget-leash check (issue 0072). It
// reads the leash SNAPSHOT (`s.agentLeash`, captured under forgeMu at agent
// selection) rather than the forge, so it never inverts the forge→s.mu lock order.
// False (never gates) when the persona has no leash or the user raised it this
// session. Caller holds s.mu.
func (s *Server) leashFor(agentID string) (telemetry.Leash, bool) {
	leash := s.agentLeash
	if agentID == "" || s.leashLifted[agentID] || !leash.Active() {
		return leash, false
	}
	return leash, leash.Breached(s.agentCredits[agentID], s.agentTurns[agentID])
}

// handleSubagentStream folds one sub-agent-tagged stream event into the
// registry: a tool start becomes the row's current activity, a tool end or a
// streamed delta returns it to "thinking…", and everything else (usage in
// particular — S3 prices it into Credits) is recorded as an observation only,
// which still performs the ADR-0040 instance↔spawn join and corroborates the
// eventual completion. A fragment is emitted only when the displayed registry
// actually changed, so a delta storm doesn't re-render the list per chunk.
// Caller holds s.mu.
func (s *Server) handleSubagentStream(e copilot.Event) []fragment {
	if e.Type == copilot.EvUsage {
		return s.recordSubagentUsage(e)
	}
	// activityChanged drives the list row's current-activity column (S2); content
	// drives the per-sub-agent transcript the overlay reads (S5). They re-render
	// independent surfaces, so each fragment is emitted only when its own state moved.
	activityChanged, content := false, false
	switch e.Type {
	case copilot.EvToolStart:
		activityChanged = s.subreg.Observe(e.AgentID, e.Tool)
		// Name from e.Tool (canonical, same value the activity column uses — main
		// timeline does likewise), args/id from the ToolCall detail; fall back to the
		// ToolCall name only if e.Tool is empty, so the transcript and the row agree.
		name, args, id := e.Tool, "", ""
		if e.ToolCall != nil {
			args, id = e.ToolCall.Args, e.ToolCall.ID
			if name == "" {
				name = e.ToolCall.Name
			}
		}
		content = s.subreg.RecordTool(e.AgentID, id, name, args)
	case copilot.EvMessageDelta:
		activityChanged = s.subreg.Observe(e.AgentID, convo.SubagentThinking)
		content = s.subreg.AppendText(e.AgentID, false, e.Text)
	case copilot.EvReasoningDelta:
		activityChanged = s.subreg.Observe(e.AgentID, convo.SubagentThinking)
		content = s.subreg.AppendText(e.AgentID, true, e.Text)
	case copilot.EvMessage:
		activityChanged = s.subreg.Observe(e.AgentID, convo.SubagentThinking)
		content = s.subreg.CommitText(e.AgentID, false, e.Text)
	case copilot.EvReasoning:
		activityChanged = s.subreg.Observe(e.AgentID, convo.SubagentThinking)
		content = s.subreg.CommitText(e.AgentID, true, e.Text)
	case copilot.EvToolEnd:
		activityChanged = s.subreg.Observe(e.AgentID, convo.SubagentThinking)
	default:
		s.subreg.Observe(e.AgentID, "")
	}
	var frags []fragment
	if activityChanged {
		frags = append(frags, s.subagentsFrag())
	}
	if content {
		if f, ok := s.subagentOverlayFrag(e.AgentID); ok {
			frags = append(frags, f)
		}
	}
	return frags
}

// recordSubagentUsage meters one sub-agent-tagged turn (epic 0069 S3, issue
// 0072): it prices the turn and appends a ledger record tagged with the instance
// id/name (so the per-sub-agent breakdown and the account-wide budget both see
// it), then folds the priced credits onto the instance's live registry row. The
// turn is deliberately kept OUT of the root/session token meters — a sub-agent's
// spend is the sub-agent's, never the root's (the S1 invariant, ADR-0040). The
// name is resolved from the registry (which the join binds) so the ledger row
// carries a restart-surviving label. Caller holds s.mu.
func (s *Server) recordSubagentUsage(e copilot.Event) []fragment {
	name := s.subreg.NameFor(e.AgentID)
	cost := s.recordUsage(e.Usage, spendTag{subagentID: e.AgentID, subagentName: name})
	changed := s.subreg.AddCredits(e.AgentID, cost.Credits())
	frags := []fragment{
		{Event: "cost", HTML: renderActualCostFooter(s.monthToDateActual(), s.budget())},
		s.statFrag(),
	}
	if changed {
		frags = append(frags, s.subagentsFrag())
	}
	return frags
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
