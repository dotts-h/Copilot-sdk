// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package web

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/config"
	"github.com/dotts-h/copilot-sdk/internal/convo"
	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
	"github.com/dotts-h/copilot-sdk/internal/pause"
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
	// runs is the persisted workflow-run history (sibling of the spend ledger);
	// a completed run is appended once on finish (ADR-0022). nil disables it.
	runs *telemetry.RunStore
	// eventLogDir is the directory that holds per-run event logs. Empty disables
	// the log — leaving the existing recording + SSE path byte-identical (ADR-0048).
	eventLogDir string
	// runEventLog is the active run's per-run event log, lazily created when the
	// first event for a run arrives. Guarded by s.mu for creation; the underlying
	// AppendOnlyStore[RunEvent] is goroutine-safe for concurrent Append calls.
	runEventLog *telemetry.RunEventLog
	// runEventLogID is the run id for which runEventLog was created; used to detect
	// when a new run starts so the log is re-created for the new run id.
	runEventLogID string
	allowance     float64
	warnFraction  float64 // soft-warn threshold as a fraction of the allowance
	hardCap       float64 // hard credit ceiling; 0 disables the gate
	runCap        float64 // per-run credit ceiling (ADR-0053); 0 disables it
	// lookPath resolves an MCP server command on PATH for the MCP page preflight,
	// isolating the one impurity behind a seam (defaults to exec.LookPath; tests
	// inject a fake).
	lookPath func(string) (string, error)
	// lookupEnv resolves an MCP Env ${VAR} reference at the forge→seam boundary
	// and in the secrets preflight (ADR-0020), behind a seam so tests inject a
	// fake env without touching the process environment (defaults to os.Getenv).
	lookupEnv func(string) string
	// setEnv stores the Connection page's pasted token in the process env (its
	// only landing place — ADR-0039), seam-injected in tests (defaults to
	// os.Setenv).
	setEnv func(string, string) error
	logger *log.Logger
	demo   bool
	// now is the session's clock seam (defaults to time.Now). It times how long a
	// lane spends parked on a human-in-the-loop pause (the S6 attention attribution
	// recorded on the run), behind a seam so that accounting is deterministic under
	// test — mirroring the pause ledger's clock-injected Sweep (ADR-0043).
	now func() time.Time

	mu          sync.Mutex
	spec        copilot.SessionSpec // per-session model/effort (mutable via /model, /agent)
	state       convo.State
	perms       []copilot.PermissionRequest
	inputs      []copilot.InputRequest  // pending ask_user questions (EvUserInput)
	plans       []copilot.PlanRequest   // pending exit-plan-mode reviews (EvPlanReview)
	elicits     []copilot.ElicitRequest // pending elicitation forms (EvElicitation)
	subreg      convo.Subagents         // sub-agent registry: the live list of every instance this session (issue 0071)
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
	gate        *budgetGate  // a turn paused on the hard cap or an agent leash (nil when none pending)
	run         *workflowRun // an active/just-finished multi-agent workflow run (item 2.1)
	// pauses is the human-in-the-loop pause ledger: the typed records a sub-agent
	// parks on when it escalates / needs input, resolved by POST /pause/{id}. The
	// orchestrator owns it (one per session); the escalate back-channel blocks on it
	// while the run stays live (epic 0069 S4, ADR-0043).
	pauses *pause.Ledger

	// agentCredits/agentTurns accumulate each persona's metered spend this session
	// (keyed by agent id), so the budget leash can gate a persona's next turn before
	// it spends past its cap (issue 0072). leashLifted records personas whose leash
	// the user raised this session, so a raised leash stops re-gating. All three are
	// reset on /clear. Sub-agent turns (tagged by instance, not persona) don't
	// accumulate here — their cap rides S4.
	agentCredits map[string]float64
	agentTurns   map[string]int64
	leashLifted  map[string]bool
	// agentLeash/agentLeashLabel snapshot the ACTIVE persona's budget leash + display
	// label, captured under forgeMu when the agent is selected (like s.spec) — so the
	// pre-dispatch gate, which runs under s.mu, reads the snapshot instead of reaching
	// for forgeMu while holding s.mu (the forge→s.mu lock-order inversion this repo
	// forbids — REGRESSIONS, CONTEXT "lock order"). — issue 0072
	agentLeash      telemetry.Leash
	agentLeashLabel string

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
	Nav     []navGroup
	Cost    template.HTML
	Main    template.HTML
	Overlay template.HTML
	Palette template.HTML
	// KeymapJSON is the action→key map the frontend dispatcher reads from
	// <body data-keymap>; html/template escapes it in the attribute context.
	KeymapJSON string
	// Attention is the initial parked-sub-agent count rendered into the #attention
	// marker, so a full page load (or a reconnect) restores the title prefix +
	// favicon dot from current state before the first live SSE swap arrives (S6).
	Attention int
}

type navItem struct {
	Slug, Label string
}

// navGroup is one labelled cluster of the sidebar (V22, ADR-0026): a group label
// with its ordered pages. PinnedStart marks the first bottom-pinned group
// (Config), which CSS pushes to the foot of the column with margin-top:auto so
// Config + Help defer to the bottom (progressive disclosure).
type navGroup struct {
	Label       string
	Items       []navItem
	PinnedStart bool
}

// navGroups folds pageNames into ordered, labelled groups: a new group starts
// whenever the group field changes (pageNames is ordered by group), and the
// first pinned group is flagged PinnedStart. One source — the grouping lives on
// pageNames, not a second table.
func navGroups() []navGroup {
	var groups []navGroup
	seenPinned := false
	for _, p := range pageNames {
		if len(groups) == 0 || groups[len(groups)-1].Label != p.group {
			g := navGroup{Label: p.group}
			if pinnedGroups[p.group] && !seenPinned {
				g.PinnedStart = true
				seenPinned = true
			}
			groups = append(groups, g)
		}
		last := &groups[len(groups)-1]
		last.Items = append(last.Items, navItem{Slug: p.slug, Label: p.label})
	}
	return groups
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	s.hub.forgeMu.Lock()
	keymap := s.config.Keymap()
	s.hub.forgeMu.Unlock()
	s.mu.Lock()
	attention := len(s.pauses.Pending())
	s.mu.Unlock()
	data := indexData{
		Nav:        navGroups(),
		Cost:       template.HTML(renderActualCostFooter(s.monthToDateActual(), s.budget())), //nolint:gosec // internally rendered, escaped via esc()
		Main:       template.HTML(s.chatPartial()),                                           //nolint:gosec // internally rendered, escaped via esc()
		Overlay:    template.HTML(helpOverlay(keymap)),                                       //nolint:gosec // internally rendered, escaped via esc()
		Palette:    template.HTML(commandPalette()),                                          //nolint:gosec // internally rendered, escaped via esc()
		KeymapJSON: keymapJSON(keymap),                                                       // shared with the Settings live-apply OOB swap
		Attention:  attention,
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

	// Per-agent budget-leash gate: if the active persona has already crossed its
	// leash this session, pause before spending more (issue 0072). Checked before
	// the account-wide cap so the more specific ceiling speaks first. The user
	// bubble is already in the transcript, so type-ahead UX is preserved.
	if g := s.pendingLeashGate(prompt, attachments); g != nil {
		s.gate = g
		s.turnStartMs = 0
		oob := s.oobTimeline() + s.oobStat() + s.oobBudget() +
			oobStatus("agent leash reached — confirm to proceed", false, 0)
		s.mu.Unlock()
		_, _ = w.Write([]byte(oob))
		return
	}

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

// The session's budget accounting — budget(), monthToDate(), forecast(), overCap(),
// and the budgetTracker they delegate to — lives in budget.go.

// budgetGate holds a turn paused because its spend would breach a ceiling — the
// account-wide hard cap or, when leashAgent is set, that agent persona's budget
// leash (issue 0072) — until the user proceeds, raises the ceiling, or cancels.
// The two share one pause-resolve shape and one /budget/{action} route; the
// leashAgent tag is what tells handleBudget which ceiling "raise" lifts.
type budgetGate struct {
	prompt      string
	attachments []string
	projected   float64
	cap         float64
	leashAgent  string // non-empty → a per-agent leash gate; the agent whose leash tripped
	leashLabel  string // the agent's display label, for the gate prompt
	leashReason string // human reason the leash tripped (e.g. "spent 12.00 cr of its 12.00 cr leash")
}

// leashGate builds a paused-turn gate for an agent persona that has crossed its
// budget leash (issue 0072), reusing the hard-cap gate's pause-resolve shape.
// Caller holds s.mu.
// pendingLeashGate returns a paused-turn gate when the active persona has crossed
// its budget leash this session, else nil — the shared pre-dispatch leash check for
// both the first send (handleSend) and the queue drain (EvIdle). Caller holds s.mu.
func (s *Server) pendingLeashGate(prompt string, attachments []string) *budgetGate {
	leash, breached := s.leashFor(s.agentID)
	if !breached {
		return nil
	}
	return s.leashGate(prompt, attachments, s.agentID, leash)
}

func (s *Server) leashGate(prompt string, attachments []string, agentID string, leash telemetry.Leash) *budgetGate {
	label := s.agentLeashLabel
	if label == "" {
		label = agentID
	}
	return &budgetGate{
		prompt: prompt, attachments: attachments,
		leashAgent: agentID, leashLabel: label,
		leashReason: leashReason(leash, s.agentCredits[agentID], s.agentTurns[agentID]),
	}
}

// leashReason describes which axis of the leash tripped, for the gate prompt.
func leashReason(leash telemetry.Leash, credits float64, turns int64) string {
	if leash.MaxCredits > 0 && credits >= leash.MaxCredits {
		return fmt.Sprintf("spent %s of its %s leash", telemetry.FormatCredits(credits), telemetry.FormatCredits(leash.MaxCredits))
	}
	if leash.MaxTurns > 0 && turns >= leash.MaxTurns {
		return fmt.Sprintf("ran %d of its %d-turn leash", turns, leash.MaxTurns)
	}
	return "reached its budget leash"
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
		cancelNote := "⚠ turn cancelled — over budget cap"
		if g.leashAgent != "" {
			cancelNote = "⚠ turn cancelled — agent budget leash"
		}
		s.state.AddSystem(cancelNote)
		oob := s.oobTimeline() + s.oobBudget() + oobStatus("", false, 0)
		s.mu.Unlock()
		_, _ = w.Write([]byte(oob))
		return
	}

	sid := s.sessionID
	s.turnStartMs = nowMs()
	s.busy = true
	prompt, attachments := g.prompt, g.attachments
	leashAgent := g.leashAgent
	s.mu.Unlock()

	if action == "raise" {
		if leashAgent != "" {
			// A leash gate's "raise" lifts THIS persona's leash for the session (a
			// transient override, not a forge edit), so its turns stop re-gating —
			// the account-wide cap is untouched (issue 0072).
			s.mu.Lock()
			s.leashLifted[leashAgent] = true
			s.mu.Unlock()
		} else if err := s.editConfig(func(c *config.Config) { c.Telemetry.HardCapCredits = 0 }); err != nil {
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

func (s *Server) handlePage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// ?window= scopes the Telemetry trend (clamped to the allowed set in renderPage);
	// every other page ignores it.
	_, _ = w.Write([]byte(s.renderPage(r.PathValue("name"), r.URL.Query().Get("window"))))
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
	if s.gate.leashAgent != "" {
		return renderLeashForm(s.gate.leashLabel, s.gate.leashReason)
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

// subagentsFrag builds the live sub-agent list from the registry — always the
// idempotent full-fragment re-render (issue 0071). Caller must hold s.mu.
func (s *Server) subagentsFrag() fragment {
	return fragment{Event: "subagents", HTML: renderSubagents(s.subreg.Entries())}
}

// attentionFrag carries the out-of-band attention marker (S6): how many
// human-in-the-loop pauses are pending — every parked lane or sub-agent that needs
// the human (the inclusive "when any pause is pending" trigger, so a lane escalation
// with no registry sub-agent still raises it). It is swapped into the shell's hidden
// #attention element so the client script can prefix the title (`(N) …`), raise the
// favicon dot, and badge the Chat nav while the human is needed — and clear them when
// the queue drains. Emitted on every pause change (pauseFrags). Caller must hold s.mu.
func (s *Server) attentionFrag() fragment {
	return fragment{Event: "attention", HTML: attentionMarker(len(s.pauses.Pending()))}
}

// attentionMarker renders the count marker swapped into #attention, carrying the
// integer count on a data-attribute AND an inline script that applies it — htmx runs
// scripts in swapped content, so the out-of-band signals (title prefix + favicon dot
// + Chat nav badge) update the moment the marker lands, the same live-apply idiom the
// keymap OOB swap uses (help.go). The data-count rides along so a full page load (or a
// reconnect) can re-read current state without an event. Presentation lives
// client-side — no new route, degrades silently — the theme-toggle precedent.
func attentionMarker(n int) string {
	c := strconv.Itoa(n)
	return `<i data-count="` + c + `"></i><script>applyAttention(` + c + `)</script>`
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
