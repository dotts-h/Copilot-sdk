package web

import (
	"net/http"
	"strings"

	"github.com/dotts-h/copilot-sdk/internal/convo"
	"github.com/dotts-h/copilot-sdk/internal/pause"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// This file is the per-sub-agent chat overlay (epic 0069 S5, issue 0074): the
// drill-down from the S2 list into a sub-agent's own live transcript, its
// input-required pause form, and — for a lane-backed sub-agent — a steer composer.
//
// The overlay is a native <dialog> loaded by an htmx GET (button + dblclick on the
// row). Its transcript region carries its own named SSE listener (subagent-<id>)
// on the SAME /events connection, so it streams live while open (htmx-ext-sse
// supports many child listeners per connection). Re-rendering the full transcript
// fragment idempotently is the foundation: a reopen replays the same bounded state
// without duplicating turns. Steering an SDK-native (in-session) sub-agent is out
// of scope — it has no Send target — so the composer renders only when the registry
// entry carries a backing LaneSession. See the issue and ADR-0026 (dialog pattern).

// subagentEvent is the per-sub-agent SSE event name an open overlay listens on.
// Keyed by the spawn id the list row carries, so an unopened overlay's events are
// a silent no-op (no matching listener) — the idempotent full re-render means a
// just-opened overlay catches up from its GET regardless.
func subagentEvent(spawnID string) string { return "subagent-" + spawnID }

// subagentOverlayFrag builds the live transcript fragment for the sub-agent bound
// to instanceID, addressed to its open overlay's named listener. Returns false
// when no entry is bound yet (an early tag before the join). Caller holds s.mu.
func (s *Server) subagentOverlayFrag(instanceID string) (fragment, bool) {
	v, ok := s.subreg.ViewByInstance(instanceID)
	if !ok {
		return fragment{}, false
	}
	return fragment{Event: subagentEvent(v.SpawnID), HTML: renderSubagentTranscript(v)}, true
}

// handleSubagentOverlay (GET /subagent/{id}) renders the drill-down dialog for one
// sub-agent (id = spawn ToolCallID, the key the list row carries). A 404 when the
// id is unknown so htmx leaves the page untouched (the row button's open call
// guards on the dialog existing). Read-only: it snapshots the registry + any
// pending pauses addressed to this sub-agent.
func (s *Server) handleSubagentOverlay(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	v, ok := s.subreg.ByID(id)
	pauses := s.pausesFor(v)
	s.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.writePartial(w, renderSubagentOverlay(v, pauses))
}

// handleSubagentSteer (POST /subagent/{id}/steer) delivers a human-authored steer
// into a lane-backed sub-agent via Send on its backing session (the mission-control
// contract: queued, applied after the current tool call). It is gated to lane-backed
// sub-agents — an SDK-native one has no Send target (read+pause-only). The steer is
// annotated in the overlay transcript so the intervention is visible, and the
// re-rendered transcript is returned (and broadcast) so the open overlay updates.
func (s *Server) handleSubagentSteer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_ = r.ParseForm()
	prompt := strings.TrimSpace(r.FormValue("prompt"))

	s.mu.Lock()
	v, ok := s.subreg.ByID(id)
	if !ok || v.LaneSession == "" || prompt == "" {
		// Nothing to steer into (unknown, SDK-native, or empty) — re-render the
		// current overlay transcript so the composer just resets, no Send.
		var html string
		if ok {
			html = renderSubagentTranscript(v)
		}
		s.mu.Unlock()
		s.writePartial(w, html)
		return
	}
	session := v.LaneSession
	s.subreg.RecordSteer(id, prompt)
	v, _ = s.subreg.ByID(id)
	frag := s.subagentTranscriptFrag(v)
	s.mu.Unlock()

	// Send outside the lock — it may block on the runtime. Offline/mock records it.
	if err := s.client.Send(r.Context(), session, prompt, nil, ""); err != nil {
		s.logger.Printf("steer sub-agent %s: %v", id, err)
	}
	s.broadcast([]fragment{frag})
	s.writePartial(w, renderSubagentTranscript(v))
}

// subagentTranscriptFrag wraps a view's transcript as its per-sub-agent SSE
// fragment. Caller holds s.mu.
func (s *Server) subagentTranscriptFrag(v convo.SubagentView) fragment {
	return fragment{Event: subagentEvent(v.SpawnID), HTML: renderSubagentTranscript(v)}
}

// renderSubagentOverlay renders the drill-down dialog for one sub-agent: a native
// <dialog> with a header (status glyph + text label, name, model, live credits), a
// live transcript region wired to its own named SSE listener, any input-required
// pause form addressed to it, and — only for a lane-backed sub-agent — a steer
// composer. All model/SDK-originated text is escaped at the template seam (ADR-0001);
// the pre-rendered transcript/pause HTML is injected trusted (each escaped its own
// inputs).
func renderSubagentOverlay(v convo.SubagentView, pauses []pause.Pause) string {
	var pf strings.Builder
	for _, p := range pauses {
		pf.WriteString(renderPauseForm(p))
	}
	return frag("subagentOverlay", map[string]any{
		"ID":          v.SpawnID,
		"Event":       subagentEvent(v.SpawnID),
		"Name":        firstNonEmpty([]string{v.DisplayName, v.Name, "sub-agent"}),
		"Model":       v.Model,
		"Glyph":       subagentGlyph(v.Status),
		"Label":       v.Status.Label(),
		"Class":       v.Status.Class(),
		"Working":     v.Status == convo.SubagentWorking,
		"Credits":     telemetry.FormatCredits(v.Credits),
		"Description": v.Description,
		"Transcript":  trusted(renderSubagentTranscript(v)),
		"Pauses":      trusted(pf.String()),
		// A steer target exists only for a lane-backed sub-agent that is still live;
		// SDK-native (no LaneSession) and settled sub-agents are read+pause-only.
		"Steerable": v.LaneSession != "" &&
			(v.Status == convo.SubagentWorking || v.Status == convo.SubagentInputRequired),
	})
}

// renderSubagentTranscript renders the bounded per-sub-agent transcript: prose and
// reasoning as text blocks, each tool call as a collapsed one-liner (name + args),
// and a human steer as a distinct annotated line. Always a full, idempotent
// re-render (the reopen/SSE-replay foundation). Empty until the first observed
// event. All text is model/SDK/human-originated → escaped via the template (richtext
// for the multi-line runs, plain escaping for the one-liners).
func renderSubagentTranscript(v convo.SubagentView) string {
	entries := make([]map[string]any, 0, len(v.Transcript))
	for _, e := range v.Transcript {
		entries = append(entries, map[string]any{
			"Message":   e.Kind == convo.SubagentMessage,
			"Reasoning": e.Kind == convo.SubagentReasoning,
			"Tool":      e.Kind == convo.SubagentToolCall,
			"Steer":     e.Kind == convo.SubagentSteer,
			"Text":      e.Text,
			"Args":      e.Args,
		})
	}
	return frag("subagentTranscript", map[string]any{"Entries": entries, "Empty": len(entries) == 0})
}

// pausesFor returns the pending pauses addressed to this sub-agent — those whose
// AgentID matches either of its identity keys (instance or spawn). Caller holds
// s.mu.
func (s *Server) pausesFor(v convo.SubagentView) []pause.Pause {
	var out []pause.Pause
	for _, p := range s.pauses.Pending() {
		if p.AgentID != "" && (p.AgentID == v.InstanceID || p.AgentID == v.SpawnID) {
			out = append(out, p)
		}
	}
	return out
}
