package web

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/dotts-h/copilot-sdk/internal/copilot"
)

// This file holds the human-in-the-loop interaction handlers lifted out of the
// god file (0088): the perm/ask/plan/elicit POST handlers that resolve a pending
// request via the matching client bridge, record the decision in the transcript,
// and return an out-of-band refresh. They share the "resolve → drop → oob" shape
// with handlePerm as the template; the Server struct + send/budget core stay in
// server.go.

// handlePerm resolves a pending tool-permission request via the client's
// permBridge, records the decision, and returns an OOB timeline refresh (the
// non-OOB empty body removes the inline form).
func (s *Server) handlePerm(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	approve := r.FormValue("approve") == "1"
	if err := s.client.Respond(id, approve); err != nil {
		s.logger.Printf("respond perm: %v", err)
	}
	verb := "approved"
	if !approve {
		verb = "rejected"
	}
	s.mu.Lock()
	s.dropPerm(id)
	// A permission may belong to a workflow lane (B1) rather than the chat
	// transcript; if so, drop it from the lane and refresh #lanes out-of-band
	// instead of the timeline (the lane card renders the inline form).
	laneOOB := s.dropLanePerm(id)
	oob := laneOOB
	if oob == "" {
		s.state.AddSystem("permission " + verb)
		oob = s.oobTimeline()
	}
	s.mu.Unlock()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(oob))
}

// dropLanePerm removes a resolved permission from whatever active-run lane holds
// it, returning an out-of-band #lanes refresh — or "" when no active run owns it
// (a plain chat permission). Caller holds s.mu.
func (s *Server) dropLanePerm(id string) string {
	if s.run == nil {
		return ""
	}
	for _, l := range s.run.lanes {
		if l.dropPerm(id) {
			return `<div id="lanes" hx-swap-oob="innerHTML">` + renderLanes(s.run) + `</div>`
		}
	}
	return ""
}

// handleAsk answers a pending ask_user request via the client's inputBridge,
// records the answer in the transcript, and returns an OOB timeline refresh (the
// non-OOB empty body removes the inline form). Mirrors handlePerm. The form may
// submit several "answer" values (an empty freeform field alongside a clicked
// choice button); the first non-empty one wins.
func (s *Server) handleAsk(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_ = r.ParseForm()
	answer := firstNonEmpty(r.Form["answer"])
	if err := s.client.RespondInput(id, answer); err != nil {
		s.logger.Printf("respond input: %v", err)
	}
	s.mu.Lock()
	s.dropInput(id)
	s.state.AddSystem("answered: " + answer)
	oob := s.oobTimeline()
	s.mu.Unlock()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(oob))
}

// handlePlanReview resolves a pending exit-plan-mode request via the client's
// planBridge. A non-empty "action" approves and proceeds with that action; an
// empty action with "feedback" declines and requests changes. Mirrors handleAsk.
func (s *Server) handlePlanReview(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_ = r.ParseForm()
	action := firstNonEmpty(r.Form["action"])
	feedback := firstNonEmpty(r.Form["feedback"])
	approved := action != ""
	if err := s.client.RespondPlan(id, approved, action, feedback); err != nil {
		s.logger.Printf("respond plan: %v", err)
	}
	note := "plan approved: " + action
	if !approved {
		note = "plan changes requested"
		if feedback != "" {
			note += ": " + feedback
		}
	}
	s.mu.Lock()
	s.dropPlan(id)
	if approved && s.mode == "plan" {
		s.mode = "" // approving the plan exits plan mode
	}
	s.state.AddSystem(note)
	oob := s.oobTimeline()
	s.mu.Unlock()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(oob))
}

// handleElicit resolves a pending elicitation form via the client's
// elicitBridge. The "action" value is "accept" (submit the form) or "decline";
// on accept the field values are read by key and coerced to the types the schema
// declared. Mirrors handlePlanReview.
func (s *Server) handleElicit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_ = r.ParseForm()
	action := r.FormValue("action")
	if action != "accept" {
		action = "decline"
	}

	s.mu.Lock()
	req, ok := s.findElicit(id)
	var content map[string]any
	if action == "accept" && ok {
		content = elicitContent(req.Fields, r.Form)
	}
	s.mu.Unlock()

	if err := s.client.RespondElicit(id, action, content); err != nil {
		s.logger.Printf("respond elicit: %v", err)
	}

	note := "form submitted"
	if action != "accept" {
		note = "form declined"
	}
	s.mu.Lock()
	s.dropElicit(id)
	s.state.AddSystem(note)
	oob := s.oobTimeline()
	s.mu.Unlock()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(oob))
}

// elicitContent coerces submitted form values into the schema's declared types,
// keyed by field name: booleans from checkbox presence, numbers/integers parsed
// (omitted when blank or unparseable), and strings passed through. Empty
// non-boolean fields are omitted so absent optional fields stay absent.
func elicitContent(fields []copilot.ElicitField, form url.Values) map[string]any {
	content := make(map[string]any, len(fields))
	for _, f := range fields {
		raw := strings.TrimSpace(form.Get(elicitFieldKey(f.Name)))
		switch f.Type {
		case "boolean":
			content[f.Name] = raw != "" // an unchecked checkbox submits nothing
		case "integer":
			if raw == "" {
				continue
			}
			if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
				content[f.Name] = n
			}
		case "number":
			if raw == "" {
				continue
			}
			if n, err := strconv.ParseFloat(raw, 64); err == nil {
				content[f.Name] = n
			}
		default:
			if raw != "" {
				content[f.Name] = raw
			}
		}
	}
	return content
}
