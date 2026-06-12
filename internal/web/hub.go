package web

import (
	"crypto/rand"
	"encoding/hex"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/config"
	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
	"github.com/dotts-h/copilot-sdk/internal/pause"
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
	client       copilot.Client
	forge        *ctxforge.Forge
	config       *config.Config
	meter        *telemetry.Meter
	spend        *telemetry.SpendStore
	runs         *telemetry.RunStore
	allowance    float64
	warnFraction float64
	hardCap      float64
	runCap       float64
	baseSpec     copilot.SessionSpec
	baseAgentID  string // the launch-time active agent id (config.DefaultAgent), seeded onto each new session for spend attribution
	// baseLeash/baseLeashLabel snapshot the launch-time agent's budget leash, seeded
	// onto each new session so the pre-dispatch gate reads it under s.mu without
	// reaching for the forge (issue 0072). Computed once at construction (startup is
	// single-threaded, so no forgeMu needed there).
	baseLeash      telemetry.Leash
	baseLeashLabel string
	logger         *log.Logger
	demo           bool
	workdir        string // base dir scanned by "import project instructions"
	// eventLogDir is the directory that holds per-run event logs
	// (<eventLogDir>/eventlogs/<runID>.json). An empty string disables the event log
	// so the existing recording + SSE path is byte-identical (ADR-0048).
	eventLogDir string
	// lookPath resolves an MCP server command on PATH for the page preflight. It is
	// the one impurity in the otherwise-pure forge rendering, isolated behind this
	// seam so tests can inject a fake; defaults to exec.LookPath.
	lookPath func(string) (string, error)
	// lookupEnv resolves an MCP Env ${VAR} reference for the forge→seam translation
	// and the secrets preflight (ADR-0020), behind a seam so tests inject a fake
	// env; defaults to os.Getenv.
	lookupEnv func(string) string
	// setEnv stores the Connection page's pasted token in the process env — the
	// only place it ever lands (no secret at rest, ADR-0039) — behind a seam so
	// tests never touch the real environment; defaults to os.Setenv.
	setEnv func(string, string) error

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
	// Spend is the persisted spend ledger; nil disables persistence (the trend
	// view and CSV export render empty).
	Spend *telemetry.SpendStore
	// Runs is the persisted workflow-run history; nil disables it (the Runs page
	// renders empty). — ADR-0022.
	Runs *telemetry.RunStore
	// EventLogDir is the directory that holds per-run event logs
	// (<EventLogDir>/eventlogs/<runID>.json). An empty string disables the event log
	// — disabling it leaves the existing recording + SSE path byte-identical. — ADR-0048.
	EventLogDir string
	Spec        copilot.SessionSpec
	Logger      *log.Logger
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
	var allowance, warnFraction, hardCap, runCap float64
	var baseAgentID string
	var baseLeash telemetry.Leash
	var baseLeashLabel string
	if opts.Config != nil {
		allowance = opts.Config.Telemetry.MonthlyCreditAllowance
		warnFraction = opts.Config.Telemetry.WarnFraction
		hardCap = opts.Config.Telemetry.HardCapCredits
		runCap = opts.Config.Telemetry.RunCapCredits
		baseAgentID = opts.Config.DefaultAgent
		// Startup is single-threaded, so reading the forge here needs no forgeMu.
		if opts.Forge != nil && baseAgentID != "" {
			if a := opts.Forge.Agent(baseAgentID); a != nil {
				baseLeash = telemetry.Leash{MaxCredits: a.MaxCredits, MaxTurns: a.MaxTurns}
				baseLeashLabel = a.Name
			}
		}
	}
	workdir := opts.Workdir
	if workdir == "" {
		workdir = "."
	}
	h := &Hub{
		client: opts.Client, forge: opts.Forge, config: opts.Config, meter: opts.Meter, spend: opts.Spend, runs: opts.Runs,
		allowance: allowance, warnFraction: warnFraction, hardCap: hardCap, runCap: runCap,
		baseSpec: opts.Spec, baseAgentID: baseAgentID, baseLeash: baseLeash, baseLeashLabel: baseLeashLabel,
		logger: lg, demo: opts.Demo, workdir: workdir, eventLogDir: opts.EventLogDir,
		lookPath: exec.LookPath, lookupEnv: os.Getenv, setEnv: os.Setenv,
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
		client: h.client, forge: h.forge, config: h.config, meter: h.meter, spend: h.spend, runs: h.runs,
		eventLogDir: h.eventLogDir,
		// A per-session meter on the account-wide meter's price book, so the
		// statusline scopes to this conversation while staying penny-consistent
		// with the global gauge (item 3.2 / TECH_DEBT #2). h.meter is non-nil by
		// the same invariant the rest of the package relies on (bootstrap/tests
		// always supply one).
		sessionMeter: telemetry.NewMeter(h.meter.PriceBook()),
		allowance:    h.allowance, warnFraction: h.warnFraction, hardCap: h.hardCap, runCap: h.runCap,
		lookPath: h.lookPath, lookupEnv: h.lookupEnv, setEnv: h.setEnv,
		logger: h.logger, demo: h.demo,
		now:             time.Now,
		spec:            h.baseSpec,
		agentID:         h.baseAgentID,
		sessionStartMs:  nowMs(),
		subs:            make(map[chan fragment]struct{}),
		pauses:          pause.NewLedger(),
		agentCredits:    make(map[string]float64),
		agentTurns:      make(map[string]int64),
		leashLifted:     make(map[string]bool),
		agentLeash:      h.baseLeash,
		agentLeashLabel: h.baseLeashLabel,
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
// originating session, rendering it through that session's reducer. When the
// server has an active run and the event log is enabled, it also appends the
// event to the per-run event log off the critical section (ADR-0048).
func (h *Hub) pump() {
	for e := range h.client.Events() {
		if sv := h.route(e.SessionID); sv != nil {
			sv.broadcast(sv.handleEvent(e))
			sv.appendRunEvent(e) // off the hot-path critical section — ADR-0048
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
	route("POST /budget/{action}", (*Server).handleBudget)
	route("POST /perm/{id}", (*Server).handlePerm)
	route("POST /ask/{id}", (*Server).handleAsk)
	route("POST /plan/{id}", (*Server).handlePlanReview)
	route("POST /elicit/{id}", (*Server).handleElicit)
	route("POST /pause/{id}", (*Server).handlePause)
	route("GET /subagent/{id}", (*Server).handleSubagentOverlay)
	route("POST /subagent/{id}/steer", (*Server).handleSubagentSteer)
	// The run inspector (ADR-0052): the first parameterized page route. More
	// specific than GET /page/{name} ({name} matches a single segment, so the two
	// never collide), reached from a link on each Runs-page row — it stays out of
	// pageNames (no sidebar/palette entry).
	route("GET /page/runs/{id}", (*Server).handleRunDetail)
	route("GET /page/{name}", (*Server).handlePage)
	route("GET /telemetry/export.csv", (*Server).handleSpendExport)
	route("GET /telemetry/reconcile.csv", (*Server).handleReconcileExport)
	route("GET /telemetry/digest.md", (*Server).handleDigestExport)
	route("GET /runs/export.csv", (*Server).handleRunsExport)
	route("GET /runs/compare", (*Server).handleRunCompare)
	route("POST /runs/rerun/{workflow}", (*Server).handleRunRerun)
	route("POST /run/abort", (*Server).handleRunAbort)
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

	route("GET /mcp/new", (*Server).handleMCPServerNew)
	route("GET /mcp/{id}/edit", (*Server).handleMCPServerEdit)
	route("POST /mcp", (*Server).handleMCPServerCreate)
	route("POST /mcp/{id}", (*Server).handleMCPServerUpdate)
	route("POST /mcp/{id}/toggle", (*Server).handleMCPServerToggle)
	route("POST /mcp/{id}/delete", (*Server).handleMCPServerDelete)

	route("GET /workflows/new", (*Server).handleWorkflowNew)
	route("GET /workflows/{id}/edit", (*Server).handleWorkflowEdit)
	route("POST /workflows", (*Server).handleWorkflowCreate)
	route("POST /workflows/{id}", (*Server).handleWorkflowUpdate)
	route("POST /workflows/{id}/run", (*Server).handleWorkflowRun)
	route("POST /workflows/{id}/delete", (*Server).handleWorkflowDelete)

	route("GET /hooks/new", (*Server).handleHookNew)
	route("GET /hooks/{id}/edit", (*Server).handleHookEdit)
	route("POST /hooks", (*Server).handleHookCreate)
	route("POST /hooks/preflight", (*Server).handleHookPreflight)
	route("POST /hooks/command-preflight", (*Server).handleHookCommandPreflight)
	route("POST /hooks/{id}", (*Server).handleHookUpdate)
	route("POST /hooks/{id}/toggle", (*Server).handleHookToggle)
	route("POST /hooks/{id}/delete", (*Server).handleHookDelete)

	route("GET /snippets/new", (*Server).handleSnippetNew)
	route("GET /snippets/{id}/edit", (*Server).handleSnippetEdit)
	route("POST /snippets", (*Server).handleSnippetCreate)
	route("POST /snippets/{id}", (*Server).handleSnippetUpdate)
	route("POST /snippets/{id}/delete", (*Server).handleSnippetDelete)

	route("POST /sessions/new", (*Server).handleSessionNew)
	route("POST /sessions/{id}/resume", (*Server).handleSessionResume)
	route("POST /sessions/{id}/delete", (*Server).handleSessionDelete)
	route("POST /connection", (*Server).handleConnectionSave)
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
