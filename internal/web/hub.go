package web

import (
	"crypto/rand"
	"encoding/hex"
	"io/fs"
	"log"
	"net/http"
	"sync"

	"github.com/dotts-h/copilot-sdk/internal/config"
	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// cookieName is the session cookie that keys each browser to its conversation.
const cookieName = "mo_sid"

// Hub is the htmx web frontend's process-wide owner. It holds the dependencies
// shared by every conversation (the copilot client, the forge, config, and cost
// meter), keeps one Server per browser session (keyed by cookie), and runs the
// single goroutine that fans the client's event stream out to the right session.
// Conversation state lives on each Server; the shared forge/config is mutated
// only under forgeMu.
type Hub struct {
	client    copilot.Client
	forge     *ctxforge.Forge
	config    *config.Config
	meter     *telemetry.Meter
	allowance float64
	baseSpec  copilot.SessionSpec
	logger    *log.Logger
	demo      bool
	workdir   string // base dir scanned by "import project instructions"

	// forgeMu serializes mutation of (and reads against mutation of) the shared
	// forge and config across all sessions.
	forgeMu sync.Mutex

	mu        sync.Mutex
	sessions  map[string]*Server // by cookie/session id
	byCopilot map[string]*Server // by copilot session id, for event routing
}

// Options configures the Hub.
type Options struct {
	Client copilot.Client
	Forge  *ctxforge.Forge
	Config *config.Config
	Meter  *telemetry.Meter
	Spec   copilot.SessionSpec
	Logger *log.Logger
	// Demo drives a scripted streaming reply through a MockClient so the UI can
	// be exercised offline (WEB_UI_PLAN.md step 1).
	Demo bool
	// Workdir is the directory scanned for well-known instruction files on import
	// (.github/copilot-instructions.md, AGENTS.md, CLAUDE.md). Defaults to ".".
	Workdir string
}

// New builds the Hub and starts the event pump.
func New(opts Options) *Hub {
	lg := opts.Logger
	if lg == nil {
		lg = log.Default()
	}
	var allowance float64
	if opts.Config != nil {
		allowance = opts.Config.Telemetry.MonthlyCreditAllowance
	}
	workdir := opts.Workdir
	if workdir == "" {
		workdir = "."
	}
	h := &Hub{
		client: opts.Client, forge: opts.Forge, config: opts.Config, meter: opts.Meter,
		allowance: allowance, baseSpec: opts.Spec, logger: lg, demo: opts.Demo, workdir: workdir,
		sessions: map[string]*Server{}, byCopilot: map[string]*Server{},
	}
	go h.pump()
	return h
}

// newSession creates and registers a Server for a cookie id, seeded with the
// shared dependencies and a copy of the base session spec.
func (h *Hub) newSession(id string) *Server {
	s := &Server{
		hub: h, id: id,
		client: h.client, forge: h.forge, config: h.config, meter: h.meter,
		allowance: h.allowance, logger: h.logger, demo: h.demo,
		spec:           h.baseSpec,
		sessionStartMs: nowMs(),
		subs:           make(map[chan fragment]struct{}),
	}
	h.mu.Lock()
	h.sessions[id] = s
	h.mu.Unlock()
	return s
}

// session resolves the Server for a request from its cookie. A request whose
// cookie names a known session gets that one, so distinct browsers (distinct
// cookies) keep independent conversations. A request with no usable cookie joins
// the sole active session when exactly one exists — covering a single-user
// reload and the offline demo — and otherwise starts a fresh conversation; the
// resolved session's id is written back as the cookie either way.
func (h *Hub) session(w http.ResponseWriter, r *http.Request) *Server {
	if c, err := r.Cookie(cookieName); err == nil && c.Value != "" {
		h.mu.Lock()
		sv := h.sessions[c.Value]
		h.mu.Unlock()
		if sv != nil {
			return sv
		}
	}

	h.mu.Lock()
	var sv *Server
	if len(h.sessions) == 1 {
		for _, only := range h.sessions {
			sv = only
		}
	}
	h.mu.Unlock()
	if sv == nil {
		sv = h.newSession(newID())
	}
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: sv.id, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	return sv
}

// bind records the copilot session id → Server mapping so the pump can route
// that session's events back to the right conversation.
func (h *Hub) bind(copilotID string, s *Server) {
	h.mu.Lock()
	h.byCopilot[copilotID] = s
	h.mu.Unlock()
}

// route finds the Server an event belongs to by its copilot session id. Events
// from a MockClient (demo/tests) carry no session id; when exactly one session
// exists they route to it, so the offline demo still streams.
func (h *Hub) route(copilotID string) *Server {
	h.mu.Lock()
	defer h.mu.Unlock()
	if sv := h.byCopilot[copilotID]; sv != nil {
		return sv
	}
	if len(h.sessions) == 1 {
		for _, sv := range h.sessions {
			return sv
		}
	}
	return nil
}

// pump consumes the client's single event stream and fans each event out to the
// originating session, rendering it through that session's reducer.
func (h *Hub) pump() {
	for e := range h.client.Events() {
		if sv := h.route(e.SessionID); sv != nil {
			sv.broadcast(sv.handleEvent(e))
		}
	}
}

// Handler returns the HTTP mux. Each route resolves the requesting browser's
// Server from its cookie, then dispatches to that Server's handler.
func (h *Hub) Handler() http.Handler {
	mux := http.NewServeMux()

	staticSub, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticSub)))

	// route registers a handler that first resolves the per-cookie Server.
	route := func(pattern string, fn func(*Server, http.ResponseWriter, *http.Request)) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			fn(h.session(w, r), w, r)
		})
	}

	route("GET /", (*Server).handleIndex)
	route("GET /events", (*Server).serveEvents)
	route("POST /send", (*Server).handleSend)
	route("POST /abort", (*Server).handleAbort)
	route("POST /perm/{id}", (*Server).handlePerm)
	route("POST /ask/{id}", (*Server).handleAsk)
	route("POST /plan/{id}", (*Server).handlePlanReview)
	route("POST /elicit/{id}", (*Server).handleElicit)
	route("GET /page/{name}", (*Server).handlePage)
	route("GET /commands", (*Server).handleCommands)

	route("GET /skills/new", (*Server).handleSkillNew)
	route("GET /skills/{id}/edit", (*Server).handleSkillEdit)
	route("POST /skills", (*Server).handleSkillCreate)
	route("POST /skills/{id}", (*Server).handleSkillUpdate)
	route("POST /skills/{id}/toggle", (*Server).handleSkillToggle)
	route("POST /skills/{id}/delete", (*Server).handleSkillDelete)

	route("POST /instructions/import", (*Server).handleInstructionImport)
	route("GET /instructions/new", (*Server).handleInstructionNew)
	route("GET /instructions/{id}/edit", (*Server).handleInstructionEdit)
	route("POST /instructions", (*Server).handleInstructionCreate)
	route("POST /instructions/{id}", (*Server).handleInstructionUpdate)
	route("POST /instructions/{id}/toggle", (*Server).handleInstructionToggle)
	route("POST /instructions/{id}/delete", (*Server).handleInstructionDelete)

	route("GET /agents/new", (*Server).handleAgentNew)
	route("GET /agents/{id}/edit", (*Server).handleAgentEdit)
	route("POST /agents", (*Server).handleAgentCreate)
	route("POST /agents/{id}", (*Server).handleAgentUpdate)
	route("POST /agents/{id}/select", (*Server).handleAgentSelect)
	route("POST /agents/{id}/delete", (*Server).handleAgentDelete)

	route("POST /sessions/new", (*Server).handleSessionNew)
	route("POST /sessions/{id}/resume", (*Server).handleSessionResume)
	route("POST /sessions/{id}/delete", (*Server).handleSessionDelete)
	route("POST /settings", (*Server).handleSettingsSave)
	route("POST /models/{id}/select", (*Server).handleModelSelect)
	route("POST /effort/{value}/select", (*Server).handleEffortSelect)
	return mux
}

// Handler is a convenience for tests and embedders: a session's HTTP handler is
// its hub's mux (routing is per-cookie, not per-session).
func (s *Server) Handler() http.Handler { return s.hub.Handler() }

// newID returns a random hex session id.
func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
