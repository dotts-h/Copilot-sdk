package web

import (
	"fmt"
	"strings"

	"github.com/dotts-h/copilot-sdk/internal/convo"
	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
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
	case "auto", "autopilot":
		return s.cmdAuto(args)
	case "ask":
		return s.cmdAsk(args)
	case "effort":
		return s.cmdEffort(args)
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

// clearConversation resets all per-conversation state to a fresh chat. The
// caller must hold s.mu. Shared by /clear and "+ New chat" on the sessions page.
func (s *Server) clearConversation() {
	s.state = convo.State{}
	s.perms = nil
	s.inputs = nil
	s.plans = nil
	s.elicits = nil
	s.subagents = nil
	s.run = nil
	s.mode = ""
	s.pending = nil
	s.queue = nil
	s.busy = false
	s.turnStartMs = 0
	s.ctxCurrent, s.ctxLimit = 0, 0
	s.compacting = false
	s.sessionID = ""
	s.live = liveNone
	s.sessionStartMs = nowMs()
	s.messagesSent, s.toolsUsed = 0, 0
}

// clearableRegions are the dynamic chat regions cmdClear empties out-of-band.
// Keep in sync with the regions chatPartial renders.
var clearableRegions = []string{"perms", "asks", "plans", "elicits", "subagents", "lanes", "status", "ctx"}

func (s *Server) cmdClear() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clearConversation()
	s.state.AddSystem("conversation cleared")
	var b strings.Builder
	b.WriteString(s.oobTimeline())
	b.WriteString(s.oobStat())
	for _, id := range clearableRegions {
		b.WriteString(`<div id="` + id + `" hx-swap-oob="innerHTML"></div>`)
	}
	return b.String()
}

// setMode moves the per-session agent mode toward target ("plan"/"autopilot"/
// "interactive"): "on" sets it, "off" clears it when it was target, and a bare
// arg toggles. Modes are mutually exclusive (one outgoing AgentMode per prompt),
// so setting one replaces another. Returns whether target is now active.
func (s *Server) setMode(target, args string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch strings.ToLower(strings.TrimSpace(args)) {
	case "on":
		s.mode = target
	case "off":
		if s.mode == target {
			s.mode = ""
		}
	default:
		if s.mode == target {
			s.mode = ""
		} else {
			s.mode = target
		}
	}
	return s.mode == target
}

// cmdPlan toggles plan mode: while on, prompts are sent with AgentMode=plan so
// the agent drafts a plan and asks to exit plan mode (handled inline by the
// plan-review form). "/plan on" and "/plan off" set it explicitly; a bare
// "/plan" toggles. Approving a plan also turns it off (see handlePlanReview).
func (s *Server) cmdPlan(args string) string {
	if s.setMode("plan", args) {
		return s.systemNote("plan mode on — the next prompt drafts a plan for your review")
	}
	return s.systemNote("plan mode off")
}

// cmdAuto toggles autopilot: prompts run with AgentMode=autopilot, so the agent
// executes tools without pausing for approval. Mutually exclusive with /plan and
// /ask.
func (s *Server) cmdAuto(args string) string {
	if s.setMode("autopilot", args) {
		return s.systemNote("autopilot on — the agent runs tools without pausing to ask")
	}
	return s.systemNote("autopilot off — back to the default mode")
}

// cmdAsk toggles ask (interactive) mode: prompts run with AgentMode=interactive,
// so the agent checks in before acting. Mutually exclusive with /plan and /auto.
func (s *Server) cmdAsk(args string) string {
	if s.setMode("interactive", args) {
		return s.systemNote("ask mode on — the agent checks in before each action")
	}
	return s.systemNote("ask mode off — back to the default mode")
}

// cmdEffort sets the reasoning effort for the session in place (restarting it)
// and persists it as the default. With no argument it reports the current value.
// Valid values are low/medium/high; "default"/"none" clears it.
func (s *Server) cmdEffort(arg string) string {
	arg = strings.ToLower(strings.TrimSpace(arg))
	if arg == "" {
		s.mu.Lock()
		cur := s.spec.ReasoningEffort
		s.mu.Unlock()
		return s.systemNote("reasoning effort: " + def(cur, "(default)"))
	}
	switch arg {
	case "low", "medium", "high":
	case "default", "none":
		arg = ""
	default:
		return s.systemNote("usage: /effort <low|medium|high>")
	}
	s.setEffort(arg)
	return s.systemNote("reasoning effort → " + def(arg, "(default)") + " (new session)")
}

// setEffort applies a reasoning effort to the session (clearing the session id so
// the next prompt opens fresh) and persists it as the shared default. Callers
// must hold neither lock.
func (s *Server) setEffort(effort string) {
	s.mu.Lock()
	s.spec.ReasoningEffort = effort
	s.sessionID = ""
	s.mu.Unlock()
	if s.config != nil {
		s.hub.forgeMu.Lock()
		s.config.ReasoningEffort = effort
		if err := s.config.Save(); err != nil {
			s.logger.Printf("save config: %v", err)
		}
		s.hub.forgeMu.Unlock()
	}
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
	s.setModel(name)
	return s.systemNote("model → " + name + " (new session)")
}

// setModel switches this session's model in place (clearing the session id so
// the next prompt opens a fresh copilot session) and persists it as the shared
// default. It locks the per-session and shared state separately, so callers must
// hold neither lock.
func (s *Server) setModel(name string) {
	s.mu.Lock()
	s.spec.Model = name
	s.sessionID = "" // restart on next send
	s.mu.Unlock()
	if s.config != nil {
		s.hub.forgeMu.Lock()
		s.config.DefaultModel = name
		if err := s.config.Save(); err != nil {
			s.logger.Printf("save config: %v", err)
		}
		s.hub.forgeMu.Unlock()
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
		s.hub.forgeMu.Lock()
		cur := def(s.config.DefaultAgent, "(none)")
		ids := make([]string, len(s.forge.Agents))
		for i, a := range s.forge.Agents {
			ids[i] = a.ID
		}
		s.hub.forgeMu.Unlock()
		note := "agent: " + cur
		if len(ids) > 0 {
			note += " — available: " + strings.Join(ids, ", ")
		}
		return s.systemNote(note)
	case "none", "off":
		s.hub.forgeMu.Lock()
		s.config.DefaultAgent = ""
		if err := s.config.Save(); err != nil {
			s.logger.Printf("save config: %v", err)
		}
		// Compile with no agent so global instructions and enabled skills still
		// reach the session, just without an agent persona.
		c := s.compiledSpec("")
		s.hub.forgeMu.Unlock()
		s.applyAgentSpec(c, "")
		return s.systemNote("agent cleared (new session)")
	}

	s.hub.forgeMu.Lock()
	agent := s.forge.Agent(arg)
	if agent == nil {
		s.hub.forgeMu.Unlock()
		return s.systemNote("unknown agent: " + arg + " — see the Agents page")
	}
	name := agent.Name
	s.config.DefaultAgent = agent.ID
	if err := s.config.Save(); err != nil {
		s.logger.Printf("save config: %v", err)
	}
	agentID := agent.ID
	c := s.compiledSpec(agentID)
	s.hub.forgeMu.Unlock()

	shown := s.applyAgentSpec(c, agentID)
	return s.systemNote("agent → " + name + " (" + shown + ", new session)")
}

// SeamSpec translates a forge-compiled SessionSpec into the copilot seam's
// SessionSpec, mapping the system message, tool allowlist, and enabled MCP
// servers and applying the config defaults for an unset model/effort (the
// built-in chat agent and the no-agent case carry no model of their own).
// Streaming is always on. lookupEnv resolves each enabled server's Env ${VAR}
// references at this boundary (ADR-0020). workspace is the session's workspace
// root, threaded onto the seam spec so the bridge's built-in fence can gate a
// write that escapes the project tree (ADR-0030). autoApprove carries the
// "Auto-approve tools" config toggle to the seam: it blanket-approves the
// non-mandatory remainder of the policy, but the mandatory dangerous ruleset still
// denies/gates (the bridge enforces that regardless — ADR-0030). It is the single
// forge→seam translation shared by the startup path (bootstrap.Build), the
// agent-restart path (compiledSpec), and each workflow lane (workflowLaneSpec) — so
// a newly added spec field can't be dropped on one path and not another (the class
// of bug behind REGRESSIONS #13/#15).
func SeamSpec(cs ctxforge.SessionSpec, defModel, defEffort string, lookupEnv func(string) string, workspace string, autoApprove bool) copilot.SessionSpec {
	spec := copilot.SessionSpec{
		Model: cs.Model, ReasoningEffort: cs.ReasoningEffort,
		SystemMessage: cs.SystemMessage, Streaming: true,
		AllowedTools: cs.AllowedTools, MCPServers: MCPServerSpecs(cs.MCPServers, lookupEnv),
		Hooks: cs.Hooks, Workspace: workspace, AutoApproveTools: autoApprove,
	}
	if spec.Model == "" {
		spec.Model = defModel
	}
	if spec.ReasoningEffort == "" {
		spec.ReasoningEffort = defEffort
	}
	return spec
}

// compiledSpec compiles the forge for agentID into the seam SessionSpec to apply.
// The caller must hold s.hub.forgeMu.
func (s *Server) compiledSpec(agentID string) copilot.SessionSpec {
	cspec, err := s.forge.Compile(agentID)
	if err != nil {
		s.logger.Printf("compile agent %q: %v", agentID, err)
	}
	return SeamSpec(cspec, s.config.DefaultModel, s.config.ReasoningEffort, s.lookupEnv, s.hub.baseSpec.Workspace, s.config.AutoApproveTools)
}

// applyAgentSpec applies a compiled spec to this session and restarts it. An
// empty model leaves the current one; a non-empty model overrides it. Streaming
// is owned by the live spec, so it is not overwritten here. agentID is the id of
// the agent now active (empty = the no-agent / built-in chat case); it is recorded
// so subsequent turns attribute their spend to it (ADR-0018). Returns the
// resulting model for status messages.
func (s *Server) applyAgentSpec(c copilot.SessionSpec, agentID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.Model != "" {
		s.spec.Model = c.Model
	}
	s.spec.ReasoningEffort = c.ReasoningEffort
	s.spec.SystemMessage = c.SystemMessage
	s.spec.AllowedTools = c.AllowedTools
	s.spec.MCPServers = c.MCPServers
	s.spec.Hooks = c.Hooks
	s.agentID = agentID
	s.sessionID = ""
	return s.spec.Model
}

// cmdCost appends a one-line credit summary and refreshes the ambient cost meter.
func (s *Server) cmdCost() string {
	// Account-wide "remaining this month" reads the persisted ledger, matching the
	// footer it re-renders (ADR-0016), so /cost survives a restart too. Spend is read
	// authoritative-cost-first (reported-where-present, estimate elsewhere) — ADR-0033.
	actual := s.monthToDateActual()
	credits := actual.ActualCredits
	budget := s.budget()
	source := "estimated"
	if actual.AllReported && actual.AnyReported {
		source = "reported"
	} else if actual.AnyReported {
		source = "mixed reported/estimated"
	}
	note := fmt.Sprintf("cost: %s of %.0f cr · remaining %s · %s",
		telemetry.FormatCredits(credits), budget.AllowanceCredits,
		telemetry.FormatCredits(budget.Remaining(credits)), source)
	return s.systemNote(note) +
		`<div id="cost-footer" hx-swap-oob="innerHTML">` + renderActualCostFooter(actual, budget) + `</div>`
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
	return `<div id="main" hx-swap-oob="innerHTML">` + s.renderPage(slug, "") + `</div>`
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
