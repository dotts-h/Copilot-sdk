package web

import (
	"net/http"
	"strings"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/pause"
)

// This file is the human-in-the-loop pause surface (epic 0069 S4): the
// orchestrator's escalation back-channel and the inline pause forms. A sub-agent
// that hits a blocker or needs input parks as input-required — a non-terminal
// state — and the run stays live while the human resolves it with
// continue(payload) / cancel. The pure ledger (internal/pause) holds the records
// and guarantees idempotent resolution; the Server is the thin adapter that blocks
// the tool handler, parks the lane, and renders the form. See ADR-0043.

// escalateReq is one invocation of the orchestrator's escalation back-channel: a
// sub-agent (or lane) asking the human to continue or cancel.
type escalateReq struct {
	laneSession string // backing session of the parking lane; "" for a non-lane pause
	agentID     string // sub-agent/lane instance recorded on the pause and rendered
	kind        pause.Kind
	message     string
	caps        []pause.Cap
}

// escalate is the orchestrator's escalation back-channel — the handler the escalate
// custom tool runs in OUR process (the research's DefineTool pattern). It registers
// a typed pause, parks the caller's lane as input-required (keeping the run live so
// siblings stream), broadcasts the pause form plus the amber lane, then BLOCKS until
// POST /pause/{id} (or a run abort) resolves it. It returns the human's instruction
// (continue) or a cancel directive (cancel/timeout) as the tool-result string handed
// back to the sub-agent — the synchronous-callback-resolved-asynchronously shape the
// permission bridge already uses, generalized. — epic 0069 S4, ADR-0043.
func (s *Server) escalate(req escalateReq) string {
	s.mu.Lock()
	// A lane escalating into a run that already settled (a concurrent abort, or its
	// lane force-failed) must NOT register a pause: nothing would resolve it, the
	// handler goroutine would block on Wait forever, and a phantom form would render
	// for a finished run. Tell the caller to wrap up instead. (A non-lane pause,
	// laneSession == "", has no run to be done — it always registers.)
	if req.laneSession != "" {
		if l := s.laneBySession(req.laneSession); l == nil || l.status != laneRunning || (s.run != nil && s.run.done) {
			s.mu.Unlock()
			return escalateResult(pause.Resolution{Action: pause.ActCancel})
		}
	}
	p := s.pauses.Register(pause.Pause{
		AgentID: req.agentID, Kind: req.kind, Message: req.message, Caps: req.caps,
	})
	s.parkLane(req.laneSession, req.message, p.ID)
	// If the escalating agent is a registry sub-agent, flip its row to the
	// input-required attention state so the list and the overlay agree (S5). A
	// no-op when AgentID names a lane persona rather than a registry instance.
	parked := s.subreg.MarkInputRequired(req.agentID)
	frags := s.pauseFrags()
	if parked {
		frags = append(frags, s.subagentsFrag())
	}
	s.mu.Unlock()
	s.broadcast(frags)

	res := p.Wait() // block until the human resolves (or a run abort cancels)

	s.mu.Lock()
	s.closeLanePause(req.laneSession)
	s.resumeLane(req.laneSession, res)
	resumed := s.subreg.ClearInputRequired(req.agentID)
	frags = s.pauseFrags()
	if resumed {
		frags = append(frags, s.subagentsFrag())
	}
	s.mu.Unlock()
	s.broadcast(frags)

	return escalateResult(res)
}

// parkLane flips a RUNNING lane to input-required (non-terminal) and records the
// pause so the lane card renders amber with the wait message. Caller holds s.mu; a
// no-op when no active-run lane owns the session (a non-lane pause) or the lane is
// no longer running — so it never stamps a pauseID/detail on an already-settled lane
// (the abort-vs-escalate race).
func (s *Server) parkLane(session, message, pauseID string) {
	l := s.laneBySession(session)
	if l == nil || l.status != laneRunning {
		return
	}
	l.status = laneInputRequired
	l.pauseID = pauseID
	l.detail = "⏸ " + message
	// Open the attention accounting: count this park and stamp its start, so the
	// finished run can attribute how often and how long the lane waited on a human
	// (S6). closeLanePause settles the span when the pause resolves or the run aborts.
	l.pauses++
	l.pausedAt = s.now()
}

// closeLanePause settles the lane (resolved by session) currently-open park —
// see closeLanePauseLane. Caller holds s.mu.
func (s *Server) closeLanePause(session string) { s.closeLanePauseLane(s.laneBySession(session)) }

// closeLanePauseLane accumulates the wall-clock a lane spent input-required into
// pausedDur (the S6 attention attribution) and clears the open mark. Idempotent: a
// no-op when no park is open, so it is safe to call on every resolution path.
// The escalate goroutine calls it (via closeLanePause) once a pause resolves — a
// human answer, a cooperative cancel, or a run abort's CancelAll — but on **abort**
// the run is recorded before that goroutine can re-acquire s.mu, so abortRun closes
// the open spans itself first; the later goroutine call then finds the mark cleared
// and is a no-op (no double count). Caller holds s.mu.
func (s *Server) closeLanePauseLane(l *lane) {
	if l == nil || l.pausedAt.IsZero() {
		return
	}
	l.pausedDur += s.now().Sub(l.pausedAt)
	l.pausedAt = time.Time{}
}

// resumeLane returns a parked lane to running on continue, or arms its cooperative
// cancel on cancel/timeout (the lane keeps running so the sub-agent wraps up, then
// settles failed(cancelled) at its next idle — workflow.go EvIdle). Caller holds
// s.mu. A no-op unless the lane is still parked: if a concurrent abort already
// force-failed it (run.abort marks unsettled lanes laneFailed before CancelAll
// unblocks this goroutine), leave its terminal status and "⏹ aborted" detail
// untouched rather than overwriting them.
func (s *Server) resumeLane(session string, res pause.Resolution) {
	l := s.laneBySession(session)
	if l == nil || l.status != laneInputRequired {
		return
	}
	l.pauseID = ""
	l.detail = ""
	l.status = laneRunning
	if res.Action == pause.ActCancel || res.Action == pause.ActTimeout {
		l.cancelReason = cancelNote(res)
	}
}

// laneBySession resolves the active-run lane whose backing session is session, or
// nil. Strict (session-keyed) unlike run.laneFor's running-lane fallback, since an
// escalate must park exactly the lane that raised it. Caller holds s.mu.
func (s *Server) laneBySession(session string) *lane {
	if s.run == nil || session == "" {
		return nil
	}
	for _, l := range s.run.lanes {
		if l.sessionID == session {
			return l
		}
	}
	return nil
}

// pauseFrags re-renders the inline #pauses region, the out-of-band attention marker
// (the pending-pause count drives the title/favicon dot + Chat nav badge, S6), and —
// when a run is active — the lane strip (a park/resume changes a lane glyph). Caller
// holds s.mu.
func (s *Server) pauseFrags() []fragment {
	frags := []fragment{
		{Event: "pauses", HTML: renderPauses(s.pauses.Pending())},
		s.attentionFrag(),
	}
	if s.run != nil {
		frags = append(frags, s.lanesFrag())
	}
	return frags
}

// handlePause resolves a pending pause from the chat surface (POST /pause/{id}).
// action is "continue" (delivering the trimmed payload as the sub-agent's tool
// result) or "cancel" (cooperative). Idempotent: a duplicate POST or one racing a
// run abort is a harmless no-op — the ledger collapses it. It returns the
// re-rendered #pauses inner HTML; the parked escalate goroutine wakes separately to
// resume/settle its lane and broadcast the lane update.
func (s *Server) handlePause(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_ = r.ParseForm()
	res := pause.Resolution{Action: pause.ActContinue, Payload: strings.TrimSpace(r.FormValue("payload"))}
	if r.FormValue("action") == "cancel" {
		res = pause.Resolution{Action: pause.ActCancel}
	}

	s.mu.Lock()
	s.pauses.Resolve(id, res)
	html := renderPauses(s.pauses.Pending())
	s.mu.Unlock()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}

// escalateResult maps a resolution to the string handed back to the sub-agent as
// the escalate tool's result: the human's payload on continue, an explicit
// wrap-up-and-stop directive on cancel/timeout.
func escalateResult(res pause.Resolution) string {
	switch res.Action {
	case pause.ActCancel:
		if res.Payload != "" {
			return "cancelled — " + res.Payload
		}
		return "cancelled — wrap up and stop"
	case pause.ActTimeout:
		if res.Payload != "" {
			return "no response in time — " + res.Payload
		}
		return "no response in time — wrap up and stop"
	default:
		return res.Payload
	}
}

// cancelNote is the one-line reason recorded on a lane settled by a cooperative
// cancel or a timeout.
func cancelNote(res pause.Resolution) string {
	if res.Action == pause.ActTimeout {
		return "timed out"
	}
	if res.Payload != "" {
		return res.Payload
	}
	return "by the human"
}

// renderPauses renders every pending pause as an inline form (the EvPermission
// pattern), idempotently — replaying the same ledger state yields identical HTML.
func renderPauses(pending []pause.Pause) string {
	var b strings.Builder
	for _, p := range pending {
		b.WriteString(renderPauseForm(p))
	}
	return b.String()
}

// renderPauseForm renders one pause as a capability-flagged inline form: the kind
// glyph + message, a reply field when the pause accepts continue/respond input, and
// only the buttons the pause declared (the Agent Inbox model). It posts to
// /pause/{id}.
func renderPauseForm(p pause.Pause) string {
	return frag("pauseForm", map[string]any{
		"ID": p.ID, "Message": p.Message, "Kind": string(p.Kind),
		"CanContinue": p.Can(pause.CapContinue),
		"CanCancel":   p.Can(pause.CapCancel),
		"ShowInput":   p.Can(pause.CapContinue) || p.Can(pause.CapRespond),
	})
}
