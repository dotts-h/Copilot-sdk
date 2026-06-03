package web

import (
	"fmt"
	"strings"

	"github.com/dotts-h/copilot-sdk/internal/convo"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// This file implements the composer's /slash commands. A prompt that begins with
// "/" is intercepted in handleSend and routed here instead of being sent to the
// model. Commands mutate session/config state and return out-of-band HTML the
// (hx-swap="none") composer response applies — a #timeline refresh, a #main page
// swap for navigation, or a #cost-footer refresh. In-place model/agent switches
// restart the backing session by clearing sessionID so the next prompt opens a
// fresh session with the updated spec (WEB_UI_PLAN.md workstream 3).

// parseCommand splits a "/name args" composer input. ok is false when the input
// is not a slash command.
func parseCommand(input string) (name, args string, ok bool) {
	if !strings.HasPrefix(input, "/") {
		return "", "", false
	}
	rest := strings.TrimSpace(input[1:])
	name, args, _ = strings.Cut(rest, " ")
	return strings.ToLower(name), strings.TrimSpace(args), true
}

// runCommand executes a parsed slash command and returns the OOB HTML to write
// back to the composer response. It is only called for inputs beginning with "/".
func (s *Server) runCommand(input string) string {
	name, args, _ := parseCommand(input)

	switch name {
	case "help", "?":
		return s.systemNote(commandHelp())
	case "clear":
		return s.cmdClear()
	case "model":
		return s.cmdModel(args)
	case "agent":
		return s.cmdAgent(args)
	case "cost":
		return s.cmdCost()
	case "attach":
		return s.cmdAttach(args)
	case "plan":
		return s.cmdPlan(args)
	}
	if isNavSlug(name) {
		return s.cmdNav(name)
	}
	return s.systemNote("unknown command: /" + name + " — try /help")
}

// systemNote appends a system turn and returns an OOB #timeline refresh.
func (s *Server) systemNote(text string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.AddSystem(text)
	return s.oobTimeline()
}

// cmdClear resets the transcript and restarts the session so the next prompt
// opens a fresh one.
func (s *Server) cmdClear() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = convo.State{}
	s.perms = nil
	s.inputs = nil
	s.plans = nil
	s.elicits = nil
	s.subagents = nil
	s.planMode = false
	s.pending = nil
	s.queue = nil
	s.busy = false
	s.turnStartMs = 0
	s.ctxCurrent, s.ctxLimit = 0, 0
	s.compacting = false
	s.sessionID = ""
	s.live = liveNone
	s.state.AddSystem("conversation cleared")
	return s.oobTimeline() +
		`<div id="perms" hx-swap-oob="innerHTML"></div>` +
		`<div id="asks" hx-swap-oob="innerHTML"></div>` +
		`<div id="plans" hx-swap-oob="innerHTML"></div>` +
		`<div id="elicits" hx-swap-oob="innerHTML"></div>` +
		`<div id="subagents" hx-swap-oob="innerHTML"></div>` +
		`<div id="status" hx-swap-oob="innerHTML"></div>` +
		`<div id="ctx" hx-swap-oob="innerHTML"></div>`
}

// cmdPlan toggles plan mode: while on, prompts are sent with AgentMode=plan so
// the agent drafts a plan and asks to exit plan mode (handled inline by the
// plan-review form). "/plan on" and "/plan off" set it explicitly; a bare
// "/plan" toggles. Approving a plan also turns it off (see handlePlanReview).
func (s *Server) cmdPlan(args string) string {
	s.mu.Lock()
	switch strings.ToLower(strings.TrimSpace(args)) {
	case "on":
		s.planMode = true
	case "off":
		s.planMode = false
	default:
		s.planMode = !s.planMode
	}
	on := s.planMode
	s.mu.Unlock()
	if on {
		return s.systemNote("plan mode on — the next prompt drafts a plan for your review")
	}
	return s.systemNote("plan mode off")
}

// cmdModel switches the session model in place (restarting the session) and
// records it as the default. With no argument it reports the current model.
func (s *Server) cmdModel(name string) string {
	if name == "" {
		s.mu.Lock()
		cur := s.spec.Model
		s.mu.Unlock()
		return s.systemNote("model: " + def(cur, "(default)"))
	}
	s.mu.Lock()
	s.setModel(name)
	s.state.AddSystem("model → " + name + " (new session)")
	oob := s.oobTimeline()
	s.mu.Unlock()
	return oob
}

// setModel switches the active model in place: it updates the session spec,
// clears the session id so the next prompt opens a fresh session with the new
// model, and persists it as the default. Caller must hold s.mu.
func (s *Server) setModel(name string) {
	s.spec.Model = name
	s.sessionID = "" // restart on next send
	if s.config != nil {
		s.config.DefaultModel = name
		if err := s.config.Save(); err != nil {
			s.logger.Printf("save config: %v", err)
		}
	}
}

// cmdAgent activates a forge agent (applying its model + reasoning effort to the
// session spec and restarting the session), or clears the active agent with
// "none"/"off". With no argument it reports the current agent and lists the
// available ids.
func (s *Server) cmdAgent(arg string) string {
	if s.forge == nil || s.config == nil {
		return s.systemNote("agents unavailable")
	}
	switch arg {
	case "":
		s.mu.Lock()
		cur := def(s.config.DefaultAgent, "(none)")
		ids := make([]string, len(s.forge.Agents))
		for i, a := range s.forge.Agents {
			ids[i] = a.ID
		}
		s.mu.Unlock()
		note := "agent: " + cur
		if len(ids) > 0 {
			note += " — available: " + strings.Join(ids, ", ")
		}
		return s.systemNote(note)
	case "none", "off":
		s.mu.Lock()
		s.config.DefaultAgent = ""
		s.spec.Model = s.config.DefaultModel
		s.spec.ReasoningEffort = s.config.ReasoningEffort
		s.sessionID = ""
		if err := s.config.Save(); err != nil {
			s.logger.Printf("save config: %v", err)
		}
		s.state.AddSystem("agent cleared (new session)")
		oob := s.oobTimeline()
		s.mu.Unlock()
		return oob
	}

	s.mu.Lock()
	agent := s.forge.Agent(arg)
	if agent == nil {
		s.mu.Unlock()
		return s.systemNote("unknown agent: " + arg + " — see the Agents page")
	}
	s.config.DefaultAgent = agent.ID
	if agent.Model != "" {
		s.spec.Model = agent.Model
	}
	s.spec.ReasoningEffort = agent.ReasoningEffort
	s.sessionID = ""
	if err := s.config.Save(); err != nil {
		s.logger.Printf("save config: %v", err)
	}
	s.state.AddSystem("agent → " + agent.Name + " (" + def(agent.Model, s.spec.Model) + ", new session)")
	oob := s.oobTimeline()
	s.mu.Unlock()
	return oob
}

// cmdCost appends a one-line credit summary and refreshes the ambient cost meter.
func (s *Server) cmdCost() string {
	totals := s.meter.Totals()
	budget := telemetry.Budget{AllowanceCredits: s.allowance}
	note := fmt.Sprintf("cost: %s of %.0f cr · remaining %s",
		telemetry.FormatCredits(totals.Credits()), budget.AllowanceCredits,
		telemetry.FormatCredits(budget.Remaining(totals.Credits())))
	return s.systemNote(note) +
		`<div id="cost-footer" hx-swap-oob="innerHTML">` + renderCostFooter(s.meter, s.allowance) + `</div>`
}

// cmdAttach queues a file path to ride along with the next prompt.
func (s *Server) cmdAttach(path string) string {
	if path == "" {
		return s.systemNote("usage: /attach <path>")
	}
	s.mu.Lock()
	s.pending = append(s.pending, path)
	s.state.AddSystem("attached " + path + " (sends with next message)")
	oob := s.oobTimeline()
	s.mu.Unlock()
	return oob
}

// cmdNav swaps the main panel to a page, mirroring a nav-link click. The composer
// posts with hx-swap="none", so the swap is delivered out-of-band on #main.
func (s *Server) cmdNav(slug string) string {
	return `<div id="main" hx-swap-oob="innerHTML">` + s.renderPage(slug) + `</div>`
}

// isNavSlug reports whether slug names one of the top-level pages.
func isNavSlug(slug string) bool {
	for _, p := range pageNames {
		if p.slug == slug {
			return true
		}
	}
	return false
}

// commandHelp lists the available slash commands for the /help note, derived
// from the same registry that powers autocomplete so the two never drift.
func commandHelp() string {
	parts := make([]string, 0, len(fixedCommandSpecs)+1)
	for _, c := range fixedCommandSpecs {
		s := "/" + c.Name
		if c.Args != "" {
			s += " " + c.Args
		}
		parts = append(parts, s)
	}
	navSlugs := make([]string, len(pageNames))
	for i, p := range pageNames {
		navSlugs[i] = "/" + p.slug
	}
	return "commands: " + strings.Join(parts, " · ") + " · " + strings.Join(navSlugs, " ")
}
