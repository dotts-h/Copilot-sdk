package web

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/config"
	"github.com/dotts-h/copilot-sdk/internal/convo"
	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// Server is one conversation: a single browser session's live transcript,
// pending request queues, streaming state, and SSE subscribers. The Hub owns the
// shared dependencies and routes each cookie to its Server. Conversation state
// is guarded by s.mu; the shared forge/config is guarded by the Hub's forgeMu
// (see editForge), so edits on one session are visible and serialized across all.
type Server struct {
	hub *Hub
	id  string // session id; also the cookie value

	// Shared dependencies, copied from the Hub for convenient access. The forge
	// and config pointers are shared across all sessions and must only be mutated
	// under hub.forgeMu.
	client copilot.Client
	forge  *ctxforge.Forge
	config *config.Config
	// meter is the process-global, account-wide meter shared across every
	// cookie-keyed session: it backs the topbar budget gauge, the hard-cap
	// projection, and the Telemetry page's month-to-date rows. sessionMeter is
	// this conversation's own scope (same price book), backing the statusline so
	// the footer reflects *this* session, not every session's combined spend
	// (item 3.2 / TECH_DEBT #2). EvUsage records each turn into both.
	meter        *telemetry.Meter
	sessionMeter *telemetry.Meter
	spend        *telemetry.SpendStore
	allowance    float64
	warnFraction float64 // soft-warn threshold as a fraction of the allowance
	hardCap      float64 // hard credit ceiling; 0 disables the gate
	// lookPath resolves an MCP server command on PATH for the MCP page preflight,
	// isolating the one impurity behind a seam (defaults to exec.LookPath; tests
	// inject a fake).
	lookPath func(string) (string, error)
	logger   *log.Logger
	demo     bool

	mu          sync.Mutex
	spec        copilot.SessionSpec // per-session model/effort (mutable via /model, /agent)
	state       convo.State
	perms       []copilot.PermissionRequest
	inputs      []copilot.InputRequest  // pending ask_user questions (EvUserInput)
	plans       []copilot.PlanRequest   // pending exit-plan-mode reviews (EvPlanReview)
	elicits     []copilot.ElicitRequest // pending elicitation forms (EvElicitation)
	subagents   []copilot.SubagentInfo  // sub-agents currently running (activity indicator)
	mode        string                  // agent mode for outgoing prompts: "" | "plan" | "autopilot" | "interactive"
	agentID     string                  // active agent persona id, tagged onto each turn's SpendRecord for cost attribution (ADR-0018); "" = built-in chat
	live        liveKind
	sessionID   string
	pending     []string     // file paths queued via /attach for the next prompt
	busy        bool         // a turn is in flight; further prompts are queued
	queue       []string     // prompts typed while busy, drained in order on turn end
	turnStartMs int64        // epoch ms the active turn began (drives the elapsed timer); 0 when idle
	ctxCurrent  int64        // last context-window token reading (EvContextWindow)
	ctxLimit    int64        // context-window size from the last reading
	compacting  bool         // conversation compaction is in progress
	gate        *budgetGate  // a turn paused on the hard cap (nil when none pending)
	run         *workflowRun // an active/just-finished multi-agent workflow run (item 2.1)

	sessionStartMs int64 // epoch ms this conversation began (drives the session timer)
	messagesSent   int64 // user prompts dispatched (statusline)
	toolsUsed      int64 // tool executions started this session (statusline)

	subsMu sync.Mutex
	subs   map[chan fragment]struct{} // active /events listeners for this session
}

// subscribe registers an SSE listener and returns its channel.
func (s *Server) subscribe() chan fragment {
	ch := make(chan fragment, 128)
	s.subsMu.Lock()
	s.subs[ch] = struct{}{}
	s.subsMu.Unlock()
	return ch
}

// unsubscribe removes and closes a listener channel.
func (s *Server) unsubscribe(ch chan fragment) {
	s.subsMu.Lock()
	if _, ok := s.subs[ch]; ok {
		delete(s.subs, ch)
		close(ch)
	}
	s.subsMu.Unlock()
}

// broadcast fans rendered fragments out to every listener on this session. A
// slow listener's fragments are dropped rather than blocking the pump.
func (s *Server) broadcast(frags []fragment) {
	if len(frags) == 0 {
		return
	}
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	for ch := range s.subs {
		for _, f := range frags {
			select {
			case ch <- f:
			default:
			}
		}
	}
}

// broadcastSendFailure renders a Send failure (which produces no runtime events)
// as an EvError and pushes it to this session's listeners, so a failed
// queue-drain still ends the turn and tells the user.
func (s *Server) broadcastSendFailure(err error) {
	s.broadcast(s.handleEvent(copilot.Event{Type: copilot.EvError, Err: err}))
}

// indexData is the data for the page shell.
type indexData struct {
	Nav     []navItem
	Cost    template.HTML
	Main    template.HTML
	Overlay template.HTML
	// KeymapJSON is the action→key map the frontend dispatcher reads from
	// <body data-keymap>; html/template escapes it in the attribute context.
	KeymapJSON string
}

type navItem struct {
	Slug, Label string
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	nav := make([]navItem, len(pageNames))
	for i, p := range pageNames {
		nav[i] = navItem{Slug: p.slug, Label: p.label}
	}
	s.hub.forgeMu.Lock()
	keymap := s.config.Keymap()
	s.hub.forgeMu.Unlock()
	dispatch := make(map[string]string, len(keymap))
	for _, k := range keymap {
		dispatch[k.ID] = k.Key
	}
	keymapJSON, _ := json.Marshal(dispatch) // map marshals with sorted keys → deterministic
	data := indexData{
		Nav:        nav,
		Cost:       template.HTML(renderCostFooter(s.monthToDate().Credits(), s.budget())), //nolint:gosec // internally rendered, escaped via esc()
		Main:       template.HTML(s.chatPartial()),                                         //nolint:gosec // internally rendered, escaped via esc()
		Overlay:    template.HTML(helpOverlay(keymap)),                                     //nolint:gosec // internally rendered, escaped via esc()
		KeymapJSON: string(keymapJSON),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pageTemplates.ExecuteTemplate(w, "index", data); err != nil {
		s.logger.Printf("render index: %v", err)
	}
}

// ensureSession lazily creates the backing session on first use.
func (s *Server) ensureSession(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessionID != "" {
		return s.sessionID, nil
	}
	id, err := s.client.CreateSession(ctx, s.spec)
	if err != nil {
		return "", err
	}
	s.sessionID = id
	s.hub.bind(id, s) // route this copilot session's events back to us
	return id, nil
}

// handleSend records the user's prompt in state, dispatches it to the client,
// and returns an out-of-band timeline refresh. The assistant's streamed reply
// arrives separately over /events — POST and SSE are decoupled so the composer
// is never disabled (WEB_UI_PLAN.md).
func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	prompt := strings.TrimSpace(r.FormValue("prompt"))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if prompt == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// A leading "/" is a composer command — unless it names a saved snippet, in
	// which case a bare "/trigger" expands to the snippet body and sends it like a
	// normal prompt (item 3.4). Reserved command/page slugs always win (see
	// snippetExpansion), so "/clear" can't be hijacked.
	if strings.HasPrefix(prompt, "/") {
		name, args, _ := parseCommand(prompt)
		if body, ok := s.snippetExpansion(name, args); ok {
			prompt = body
		} else {
			_, _ = w.Write([]byte(s.runCommand(prompt)))
			return
		}
	}

	// A multi-agent workflow run owns the turn; normal prompts don't queue behind
	// it (the run drives the lanes, not the chat composer). Surface a note and bail.
	s.mu.Lock()
	runActive := s.run != nil && !s.run.done
	s.mu.Unlock()
	if runActive {
		_, _ = w.Write([]byte(s.systemNote("a workflow is running — wait for it to finish or clear the chat (/clear)")))
		return
	}

	// A turn is already running: queue the prompt instead of dispatching it.
	// The user bubble shows immediately (Claude-CLI-style type-ahead); the reply
	// streams once the in-flight turn ends and the queue drains (handleEvent
	// EvIdle). The composer is never disabled — POST and SSE stay decoupled.
	s.mu.Lock()
	if s.busy {
		s.state.AddUser(prompt)
		s.messagesSent++
		s.queue = append(s.queue, prompt)
		n := len(s.queue)
		oob := s.oobTimeline() + s.oobStat() +
			`<div id="status" hx-swap-oob="innerHTML">` + renderStatus(queuedStatus(n), true, s.turnStartMs) + `</div>`
		s.mu.Unlock()
		_, _ = w.Write([]byte(oob))
		return
	}
	s.busy = true
	s.turnStartMs = nowMs()
	s.mu.Unlock()

	sessionID, err := s.ensureSession(r.Context())
	if err != nil {
		s.mu.Lock()
		s.busy = false
		s.mu.Unlock()
		s.logger.Printf("create session: %v", err)
		http.Error(w, "session unavailable", http.StatusBadGateway)
		return
	}

	s.mu.Lock()
	s.state.AddUser(prompt)
	s.messagesSent++
	attachments := s.pending
	s.pending = nil
	s.live = liveNone

	// Hard-cap gate: if the projected spend of this turn (current total + the
	// pre-flight estimate of resending the live context) would breach the cap,
	// pause and ask for confirmation instead of dispatching. The user bubble is
	// already in the transcript, so type-ahead UX is preserved.
	if projected, capped := s.overCap(); capped {
		s.gate = &budgetGate{prompt: prompt, attachments: attachments, projected: projected, cap: s.hardCap}
		s.turnStartMs = 0 // not running yet — awaiting the decision
		oob := s.oobTimeline() + s.oobStat() + s.oobBudget() +
			oobStatus("over budget cap — confirm to proceed", false, 0)
		s.mu.Unlock()
		_, _ = w.Write([]byte(oob))
		return
	}

	oob := s.oobTimeline() + s.oobStat() + oobStatus("thinking…", true, s.turnStartMs)
	s.mu.Unlock()

	if err := s.dispatch(r.Context(), sessionID, prompt, attachments); err != nil {
		oob += s.sendFailedOOB(err)
	}
	_, _ = w.Write([]byte(oob))
}

// overCap reports the projected credit spend of the next turn (running total
// plus the pre-flight estimate of resending the live context) and whether it
// would breach the hard cap. Caller must hold s.mu.
func (s *Server) overCap() (projected float64, capped bool) {
	b := s.budget()
	if b.HardCapCredits <= 0 {
		return 0, false
	}
	// The "used" baseline is the persisted month-to-date, not the in-process meter,
	// so a mid-month restart doesn't silently re-open a cap the user already
	// approached (ADR-0016). The next-turn estimate still comes from the live meter.
	projected = s.monthToDate().Credits() + s.meter.EstimateTurn(s.spec.Model, s.ctxCurrent).Credits()
	return projected, b.CapExceeded(projected)
}

// monthToDate returns the account-wide spend recorded within the current UTC
// calendar month, read from the persisted ledger so the budget rows, the cost
// footer, and the hard-cap baseline survive a restart (ADR-0016). Zero when no
// ledger is wired (the offline demo / tests without one). Distinct from the
// per-session statusline (sessionMeter) and the live token split (process meter):
// one source per surface (see REGRESSIONS "two meters" — now three sources).
func (s *Server) monthToDate() telemetry.Cost {
	if s.spend == nil {
		return telemetry.Cost{}
	}
	return telemetry.MonthToDate(s.spend.Records(), time.Now())
}

// forecast projects when the monthly allowance is exhausted at the recent burn
// rate, read account-wide off the persisted ledger like the other ADR-0016 rows
// (a pure telemetry.Forecast over the daily series — ADR-0019). ok is false when
// no ledger is wired or it holds no records, so callers can hide the surface
// entirely (distinct from a wired-but-degenerate projection, which Status carries).
func (s *Server) forecast(now time.Time) (telemetry.Projection, bool) {
	if s.spend == nil {
		return telemetry.Projection{}, false
	}
	records := s.spend.Records()
	if len(records) == 0 {
		return telemetry.Projection{}, false
	}
	return telemetry.Forecast(telemetry.DailyTotals(records), s.budget(), now), true
}

// budget builds the telemetry budget from this session's cached allowance, warn
// fraction, and hard cap (refreshed from config on a settings save).
func (s *Server) budget() telemetry.Budget {
	return telemetry.Budget{
		AllowanceCredits: s.allowance,
		WarnFraction:     s.warnFraction,
		HardCapCredits:   s.hardCap,
	}
}

// budgetGate holds a turn paused because its projected spend would breach the
// hard cap, until the user proceeds, raises the cap, or cancels it.
type budgetGate struct {
	prompt      string
	attachments []string
	projected   float64
	cap         float64
}

// handleBudget resolves a paused over-cap turn. "proceed" dispatches the held
// prompt and keeps the cap; "raise" lifts (disables) the cap, persists it, and
// dispatches; "cancel" drops the turn. Mirrors the inline permission flow.
func (s *Server) handleBudget(w http.ResponseWriter, r *http.Request) {
	action := r.PathValue("action")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	s.mu.Lock()
	g := s.gate
	s.gate = nil
	if g == nil {
		s.mu.Unlock()
		_, _ = w.Write(nil) // nothing pending (e.g. a double submit)
		return
	}

	// Only an explicit proceed/raise releases the held turn; cancel (and any
	// unrecognized action) drops it — the safe default never spends past the cap.
	if action != "proceed" && action != "raise" {
		s.busy = false
		s.queue = nil
		s.turnStartMs = 0
		s.state.AddSystem("⚠ turn cancelled — over budget cap")
		oob := s.oobTimeline() + s.oobBudget() + oobStatus("", false, 0)
		s.mu.Unlock()
		_, _ = w.Write([]byte(oob))
		return
	}

	sid := s.sessionID
	s.turnStartMs = nowMs()
	s.busy = true
	prompt, attachments := g.prompt, g.attachments
	s.mu.Unlock()

	if action == "raise" {
		if err := s.editConfig(func(c *config.Config) { c.Telemetry.HardCapCredits = 0 }); err != nil {
			s.logger.Printf("lift cap: %v", err)
		} else {
			s.mu.Lock()
			s.hardCap = 0
			s.mu.Unlock()
		}
	}

	s.mu.Lock()
	oob := s.oobTimeline() + s.oobStat() + s.oobBudget() + oobStatus("thinking…", true, s.turnStartMs)
	s.mu.Unlock()

	if err := s.dispatch(r.Context(), sid, prompt, attachments); err != nil {
		oob += s.sendFailedOOB(err)
	}
	_, _ = w.Write([]byte(oob))
}

// dispatch sends a prompt to the backing session and, in demo mode, kicks the
// scripted reply. Shared by handleSend and the queue drain (handleEvent EvIdle).
// It returns the Send error (if any) so callers can surface it; the turn would
// otherwise produce no events and leave the UI waiting forever.
func (s *Server) dispatch(ctx context.Context, sessionID, prompt string, attachments []string) error {
	s.mu.Lock()
	mode := s.mode
	s.mu.Unlock()
	if err := s.client.Send(ctx, sessionID, prompt, attachments, mode); err != nil {
		s.logger.Printf("send: %v", err)
		return err
	}
	if s.demo {
		if mock, ok := s.client.(*copilot.MockClient); ok {
			go streamDemoReply(mock, prompt)
		}
	}
	return nil
}

// sendFailedOOB records a failed dispatch as a system note, resets the busy
// state so the composer is not stuck "thinking…", and returns the OOB fragments
// (timeline + cleared status) to merge into the response.
func (s *Server) sendFailedOOB(err error) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.AddSystem("⚠ send failed: " + err.Error())
	s.busy = false
	s.turnStartMs = 0
	return s.oobTimeline() +
		`<div id="status" hx-swap-oob="innerHTML">` + renderStatus("", false, 0) + `</div>`
}

// queuedStatus renders the status text shown while prompts are queued behind an
// in-flight turn.
func queuedStatus(n int) string {
	if n == 1 {
		return "thinking… · 1 queued"
	}
	return fmt.Sprintf("thinking… · %d queued", n)
}

func (s *Server) handleAbort(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	id := s.sessionID
	s.queue = nil // abort cancels the turn and any prompts queued behind it
	s.busy = false
	s.turnStartMs = 0
	s.mu.Unlock()
	if id != "" {
		if err := s.client.Abort(r.Context(), id); err != nil {
			s.logger.Printf("abort: %v", err)
		}
	}
	// Clear the status line immediately; the client's turn-end event over /events
	// will also settle it. hx-target on the button swaps this into #status.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(renderStatus("aborted", false, 0)))
}

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

func (s *Server) handlePage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(s.renderPage(r.PathValue("name"))))
}

// --- forge mutators (list pages) ---
//
// All forge/config mutation goes through editForge (or an explicit s.mu section)
// so it is serialized against the readers in pages.go, and through the validated
// ctxforge methods so an invalid edit is rolled back rather than half-applied.

func (s *Server) handleSkillToggle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_ = s.editForge(func() error { _, err := s.forge.ToggleSkill(id); return err })
	s.writePartial(w, s.skillsPartial())
}

func (s *Server) handleSkillDelete(w http.ResponseWriter, r *http.Request) {
	skillCRUD.Delete(s, w, r) // a failure (e.g. an agent still pins it) is logged
}

func (s *Server) handleInstructionToggle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_ = s.editForge(func() error { _, err := s.forge.ToggleInstruction(id); return err })
	s.writePartial(w, s.instructionsPartial())
}

func (s *Server) handleInstructionDelete(w http.ResponseWriter, r *http.Request) {
	instructionCRUD.Delete(s, w, r)
}

func (s *Server) handleAgentSelect(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.hub.forgeMu.Lock()
	compileID := id
	if s.config.DefaultAgent == id {
		s.config.DefaultAgent = "" // toggle off → no agent persona
		compileID = ""
	} else {
		s.config.DefaultAgent = id
	}
	if err := s.config.Save(); err != nil {
		s.logger.Printf("save config: %v", err)
	}
	c := s.compiledSpec(compileID)
	s.hub.forgeMu.Unlock()
	// Compile the agent's full persona (system message + instructions + skill
	// prompts) plus model/effort/tool-allowlist + enabled MCP servers into the live
	// spec and restart the session so the selection takes effect on the next prompt.
	s.applyAgentSpec(c, compileID)
	s.writePartial(w, s.agentsPartial())
}

// handleModelSelect switches the active model from the model picker and
// re-renders the page with the new current marked.
func (s *Server) handleModelSelect(w http.ResponseWriter, r *http.Request) {
	s.setModel(r.PathValue("id")) // self-locks per-session + shared state
	s.writePartial(w, s.modelsPartial())
}

// handleEffortSelect sets the reasoning effort from the Models page and
// re-renders it with the new effort marked. "default" clears it.
func (s *Server) handleEffortSelect(w http.ResponseWriter, r *http.Request) {
	v := r.PathValue("value")
	if v == "default" {
		v = ""
	}
	s.setEffort(v) // self-locks per-session + shared state
	s.writePartial(w, s.modelsPartial())
}

func (s *Server) handleAgentDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.editForge(func() error { return s.forge.RemoveAgent(id) }); err != nil {
		s.logger.Printf("remove agent: %v", err)
		s.writePartial(w, s.agentsPartial())
		return
	}
	// Deleting the active agent clears the config pointer to it.
	s.hub.forgeMu.Lock()
	if s.config.DefaultAgent == id {
		s.config.DefaultAgent = ""
		if err := s.config.Save(); err != nil {
			s.logger.Printf("save config: %v", err)
		}
	}
	s.hub.forgeMu.Unlock()
	s.writePartial(w, s.agentsPartial())
}

// --- helpers ---

// oobTimeline renders an out-of-band #timeline refresh for POST responses.
// Caller must hold s.mu.
func (s *Server) oobTimeline() string {
	return `<div id="timeline" hx-swap-oob="innerHTML">` + renderTimelineInner(&s.state) + `</div>`
}

// oobStatus renders an out-of-band #status refresh for POST responses.
func oobStatus(text string, active bool, startMs int64) string {
	return `<div id="status" hx-swap-oob="innerHTML">` + renderStatus(text, active, startMs) + `</div>`
}

// oobBudget renders an out-of-band #budget refresh (the inline hard-cap gate, or
// empty once resolved). Caller must hold s.mu.
func (s *Server) oobBudget() string {
	return `<div id="budget" hx-swap-oob="innerHTML">` + s.renderGate() + `</div>`
}

// budgetFrag builds the SSE fragment that swaps the inline hard-cap gate into
// #budget (used when a queued turn is gated on drain). Caller must hold s.mu.
func (s *Server) budgetFrag() fragment {
	return fragment{Event: "budget", HTML: s.renderGate()}
}

// renderGate renders the pending hard-cap gate form, or "" when none is pending.
// Caller must hold s.mu.
func (s *Server) renderGate() string {
	if s.gate == nil {
		return ""
	}
	return renderBudgetForm(s.gate.projected, s.gate.cap)
}

// statusFrag builds the status SSE fragment, attaching the live elapsed-timer
// start when a turn is active. Caller must hold s.mu.
func (s *Server) statusFrag(text string, active bool) fragment {
	start := int64(0)
	if active {
		start = s.turnStartMs
	}
	return fragment{Event: "status", HTML: renderStatus(text, active, start)}
}

// ctxFrag builds the context-meter SSE fragment from the latest reading.
// Caller must hold s.mu.
func (s *Server) ctxFrag() fragment {
	return fragment{Event: "ctx", HTML: renderCtx(s.ctxCurrent, s.ctxLimit, s.compacting)}
}

// statFrag builds the live statusline SSE fragment (model, mode, context fill,
// session timer, token/credit accounting). Caller must hold s.mu.
func (s *Server) statFrag() fragment {
	return fragment{Event: "stat", HTML: renderStatline(s)}
}

// oobStat returns an out-of-band statusline refresh for POST responses (the send
// path updates it immediately rather than waiting for the next SSE event).
// Caller must hold s.mu.
func (s *Server) oobStat() string {
	return `<div id="statline" hx-swap-oob="innerHTML">` + renderStatline(s) + `</div>`
}

// subagentsFrag builds the background-activity strip from the currently-running
// sub-agents. Caller must hold s.mu.
func (s *Server) subagentsFrag() fragment {
	return fragment{Event: "subagents", HTML: renderSubagents(s.subagents)}
}

// nowMs returns the current time in epoch milliseconds.
func nowMs() int64 { return time.Now().UnixMilli() }

// dropByID returns sl with the first element whose key matches id removed (the
// original slice if none matches). The pending-request queues all share this
// "splice-out by id" shape, differing only in element type and key accessor.
func dropByID[T any](sl []T, id string, key func(T) string) []T {
	for i := range sl {
		if key(sl[i]) == id {
			return append(sl[:i], sl[i+1:]...)
		}
	}
	return sl
}

// findByID returns the first element whose key matches id, and whether one was
// found.
func findByID[T any](sl []T, id string, key func(T) string) (T, bool) {
	for _, e := range sl {
		if key(e) == id {
			return e, true
		}
	}
	var zero T
	return zero, false
}

func permID(p copilot.PermissionRequest) string { return p.ID }
func inputID(p copilot.InputRequest) string     { return p.ID }
func planID(p copilot.PlanRequest) string       { return p.ID }
func elicitID(e copilot.ElicitRequest) string   { return e.ID }
func subagentKey(a copilot.SubagentInfo) string { return a.ToolCallID }

// The drop* / findElicit helpers below keep their names (callers are unchanged);
// each is a one-line binding of dropByID/findByID to its queue. Caller holds s.mu.

func (s *Server) dropPerm(id string)  { s.perms = dropByID(s.perms, id, permID) }
func (s *Server) dropInput(id string) { s.inputs = dropByID(s.inputs, id, inputID) }
func (s *Server) dropPlan(id string)  { s.plans = dropByID(s.plans, id, planID) }
func (s *Server) dropElicit(id string) {
	s.elicits = dropByID(s.elicits, id, elicitID)
}

// findElicit returns a copy of the pending elicitation with the given id and
// whether it was found.
func (s *Server) findElicit(id string) (copilot.ElicitRequest, bool) {
	return findByID(s.elicits, id, elicitID)
}

// dropSubagent removes a finished sub-agent by its parent tool-call id.
func (s *Server) dropSubagent(toolCallID string) {
	s.subagents = dropByID(s.subagents, toolCallID, subagentKey)
}

// firstNonEmpty returns the first non-empty, trimmed string in vals, or "".
func firstNonEmpty(vals []string) string {
	for _, v := range vals {
		if t := strings.TrimSpace(v); t != "" {
			return t
		}
	}
	return ""
}

// editForge applies a forge mutation under s.mu (serializing it against the
// readers in pages.go) and persists it on success. It returns fn's error — e.g.
// a validation failure the ctxforge method already rolled back — so callers can
// surface it; a persistence (disk) failure is logged, not returned.
func (s *Server) editForge(fn func() error) error {
	s.hub.forgeMu.Lock()
	defer s.hub.forgeMu.Unlock()
	if err := fn(); err != nil {
		return err
	}
	if s.forge != nil {
		if err := s.forge.Save(); err != nil {
			s.logger.Printf("save forge: %v", err)
		}
	}
	return nil
}

func (s *Server) writePartial(w http.ResponseWriter, html string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}
