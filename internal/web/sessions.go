// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package web

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/convo"
	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// This file implements the session picker (backlog #8, ADR 0002): list the
// runtime's persisted sessions, start a fresh chat, resume a past one (rebuilding
// its transcript from history), or delete one. The runtime is the source of
// truth — nothing is persisted here.

// sessionsPartial renders the Sessions page: the persisted sessions with resume
// and delete controls, plus "+ New chat".
func (s *Server) sessionsPartial() string {
	metas, err := s.client.ListSessions(context.Background())
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	return frag("sessionsPage", map[string]any{"Rows": s.sessionRows(metas), "Err": errMsg})
}

// sessionRows builds the per-session template rows, flagging the active session.
// Shared by sessionsPartial and sessionsError so both renderings agree (the error
// path used to drop the "Current" marker).
func (s *Server) sessionRows(metas []copilot.SessionMeta) []map[string]any {
	// Join the pure SessionShares reader (turn count + credits per copilot session
	// id) onto each listed session, keyed by id (G2). The spend store carries its
	// own leaf mutex, so this read takes neither s.mu nor forgeMu — it can't invert
	// the forgeMu→s.mu lock order. With no store wired the map stays empty and rows
	// render their prior shape (title + age, no cost cell). A listed session with no
	// spend keeps its row (HasSpend=false → "no cost yet"); a spend bucket whose id
	// matches no listed session is simply never iterated.
	haveSpend := s.spend != nil
	cost := map[string]telemetry.SessionShare{}
	if haveSpend {
		for _, sh := range telemetry.SessionShares(s.spend.Records()) {
			cost[sh.SessionID] = sh
		}
	}
	s.mu.Lock()
	current := s.sessionID
	s.mu.Unlock()
	rows := make([]map[string]any, 0, len(metas))
	for _, m := range metas {
		row := map[string]any{
			"ID": m.ID, "Title": sessionTitle(m), "When": humanWhen(m.Modified),
			"Current": m.ID == current,
		}
		// ShowCost gates the whole cell on a wired store, so a nil-spend deployment
		// keeps the prior shape (no "no cost yet" either).
		if haveSpend {
			row["ShowCost"] = true
			if sh, ok := cost[m.ID]; ok {
				row["HasSpend"] = true
				row["Turns"] = sh.Turns
				row["Credits"] = telemetry.FormatCredits(sh.Credits)
			}
		}
		rows = append(rows, row)
	}
	return rows
}

// sessionTitle is the session's summary, falling back to a short id.
func sessionTitle(m copilot.SessionMeta) string {
	if m.Summary != "" {
		return m.Summary
	}
	id := m.ID
	if len(id) > 12 {
		id = id[:12]
	}
	return "session " + id
}

// humanWhen renders a modified time as a compact relative age.
func humanWhen(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m ago"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h ago"
	default:
		return strconv.Itoa(int(d.Hours()/24)) + "d ago"
	}
}

// handleSessionNew starts a fresh conversation and shows the chat page.
func (s *Server) handleSessionNew(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.clearConversation()
	s.state.AddSystem("new chat")
	s.mu.Unlock()
	s.writePartial(w, s.chatPartial())
}

// handleSessionResume reattaches to a persisted session and rebuilds its
// transcript from history, then shows the chat page. A resume failure (e.g. the
// session expired) falls back to the sessions list with the error.
func (s *Server) handleSessionResume(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	spec := s.spec
	s.mu.Unlock()

	resumed, err := s.client.ResumeSession(r.Context(), id, spec)
	if err != nil {
		s.writePartial(w, s.sessionsError("resume failed: "+err.Error()))
		return
	}
	history, err := s.client.SessionHistory(r.Context(), resumed)
	if err != nil {
		s.writePartial(w, s.sessionsError("loaded session but could not read history: "+err.Error()))
		return
	}

	s.mu.Lock()
	s.clearConversation()
	s.sessionID = resumed
	s.hub.bind(resumed, s) // route the resumed session's live events back to us
	s.loadHistory(history)
	s.mu.Unlock()
	s.writePartial(w, s.chatPartial())
}

// handleSessionDelete permanently removes a session and re-renders the list.
func (s *Server) handleSessionDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.client.DeleteSession(r.Context(), id); err != nil {
		s.writePartial(w, s.sessionsError("delete failed: "+err.Error()))
		return
	}
	s.writePartial(w, s.sessionsPartial())
}

// sessionsError re-renders the sessions list with a banner.
func (s *Server) sessionsError(msg string) string {
	metas, _ := s.client.ListSessions(context.Background())
	return frag("sessionsPage", map[string]any{"Rows": s.sessionRows(metas), "Err": msg})
}

// loadHistory rebuilds s.state from a session's normalized history events. The
// caller must hold s.mu. It mirrors the live reducer for committed events and
// adds user prompts (which the live path records itself on send). A trailing
// Finish flushes any reasoning block with no following message.
func (s *Server) loadHistory(events []copilot.Event) {
	st := convo.State{}
	for _, e := range events {
		switch e.Type {
		case copilot.EvUserMessage:
			st.AddUser(e.Text)
		case copilot.EvReasoning:
			st.AppendReasoning(e.Text)
		case copilot.EvMessage:
			st.Finish(e.Text)
		case copilot.EvToolStart:
			if e.ToolCall != nil {
				st.ToolStart(e.ToolCall.ID, e.ToolCall.Name, e.ToolCall.Args)
			}
		case copilot.EvToolEnd:
			if e.ToolCall != nil {
				st.ToolEnd(e.ToolCall.ID, e.ToolCall.Result, e.ToolCall.Success)
			}
		}
	}
	st.Finish("")
	s.state = st
}
