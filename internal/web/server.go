package web

import (
	"context"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/dotts-h/copilot-sdk/internal/config"
	"github.com/dotts-h/copilot-sdk/internal/convo"
	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// Server is the htmx web frontend. It owns the per-process session state (single
// local user assumed) and bridges browser actions to the copilot.Client seam.
// The fields under mu are the web analogue of the TUI's Model minus rendering:
// the live transcript, the permission queue, and the streaming kind.
type Server struct {
	client    copilot.Client
	forge     *ctxforge.Forge
	config    *config.Config
	meter     *telemetry.Meter
	allowance float64
	spec      copilot.SessionSpec
	logger    *log.Logger
	demo      bool

	mu        sync.Mutex
	state     convo.State
	perms     []copilot.PermissionRequest
	live      liveKind
	sessionID string
}

// Options configures a Server.
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
}

// New builds a Server.
func New(opts Options) *Server {
	lg := opts.Logger
	if lg == nil {
		lg = log.Default()
	}
	var allowance float64
	if opts.Config != nil {
		allowance = opts.Config.Telemetry.MonthlyCreditAllowance
	}
	return &Server{
		client:    opts.Client,
		forge:     opts.Forge,
		config:    opts.Config,
		meter:     opts.Meter,
		allowance: allowance,
		spec:      opts.Spec,
		logger:    lg,
		demo:      opts.Demo,
	}
}

// Handler returns the configured HTTP mux.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	staticSub, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticSub)))

	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("GET /events", s.serveEvents)
	mux.HandleFunc("POST /send", s.handleSend)
	mux.HandleFunc("POST /abort", s.handleAbort)
	mux.HandleFunc("POST /perm/{id}", s.handlePerm)
	mux.HandleFunc("GET /page/{name}", s.handlePage)

	mux.HandleFunc("GET /skills/new", s.handleSkillNew)
	mux.HandleFunc("GET /skills/{id}/edit", s.handleSkillEdit)
	mux.HandleFunc("POST /skills", s.handleSkillCreate)
	mux.HandleFunc("POST /skills/{id}", s.handleSkillUpdate)
	mux.HandleFunc("POST /skills/{id}/toggle", s.handleSkillToggle)
	mux.HandleFunc("POST /skills/{id}/delete", s.handleSkillDelete)

	mux.HandleFunc("GET /instructions/new", s.handleInstructionNew)
	mux.HandleFunc("GET /instructions/{id}/edit", s.handleInstructionEdit)
	mux.HandleFunc("POST /instructions", s.handleInstructionCreate)
	mux.HandleFunc("POST /instructions/{id}", s.handleInstructionUpdate)
	mux.HandleFunc("POST /instructions/{id}/toggle", s.handleInstructionToggle)
	mux.HandleFunc("POST /instructions/{id}/delete", s.handleInstructionDelete)

	mux.HandleFunc("GET /agents/new", s.handleAgentNew)
	mux.HandleFunc("GET /agents/{id}/edit", s.handleAgentEdit)
	mux.HandleFunc("POST /agents", s.handleAgentCreate)
	mux.HandleFunc("POST /agents/{id}", s.handleAgentUpdate)
	mux.HandleFunc("POST /agents/{id}/select", s.handleAgentSelect)
	mux.HandleFunc("POST /agents/{id}/delete", s.handleAgentDelete)
	return mux
}

// indexData is the data for the page shell.
type indexData struct {
	Nav  []navItem
	Cost template.HTML
	Main template.HTML
}

type navItem struct {
	Slug, Label string
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	nav := make([]navItem, len(pageNames))
	for i, p := range pageNames {
		nav[i] = navItem{Slug: p.slug, Label: p.label}
	}
	data := indexData{
		Nav:  nav,
		Cost: template.HTML(renderCostFooter(s.meter, s.allowance)), //nolint:gosec // internally rendered, escaped via esc()
		Main: template.HTML(s.chatPartial()),                        //nolint:gosec // internally rendered, escaped via esc()
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

	sessionID, err := s.ensureSession(r.Context())
	if err != nil {
		s.logger.Printf("create session: %v", err)
		http.Error(w, "session unavailable", http.StatusBadGateway)
		return
	}

	s.mu.Lock()
	s.state.AddUser(prompt)
	s.live = liveNone
	oob := s.oobTimeline()
	s.mu.Unlock()

	if err := s.client.Send(r.Context(), sessionID, prompt, nil); err != nil {
		s.logger.Printf("send: %v", err)
		http.Error(w, "send failed", http.StatusBadGateway)
		return
	}
	if s.demo {
		if mock, ok := s.client.(*copilot.MockClient); ok {
			go streamDemoReply(mock, prompt)
		}
	}
	_, _ = w.Write([]byte(oob))
}

func (s *Server) handleAbort(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	id := s.sessionID
	s.mu.Unlock()
	if id != "" {
		if err := s.client.Abort(r.Context(), id); err != nil {
			s.logger.Printf("abort: %v", err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
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
	s.state.AddSystem("permission " + verb)
	oob := s.oobTimeline()
	s.mu.Unlock()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(oob))
}

func (s *Server) handlePage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(s.renderPage(r.PathValue("name"))))
}

// --- forge mutators (list pages) ---

func (s *Server) handleSkillToggle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	for i := range s.forge.Skills {
		if s.forge.Skills[i].ID == id {
			s.forge.Skills[i].Enabled = !s.forge.Skills[i].Enabled
			break
		}
	}
	s.persist()
	s.writePartial(w, s.skillsPartial())
}

func (s *Server) handleSkillDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	for i := range s.forge.Skills {
		if s.forge.Skills[i].ID == id {
			s.forge.Skills = append(s.forge.Skills[:i], s.forge.Skills[i+1:]...)
			break
		}
	}
	s.persist()
	s.writePartial(w, s.skillsPartial())
}

func (s *Server) handleInstructionToggle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	for i := range s.forge.Instructions {
		if s.forge.Instructions[i].ID == id {
			s.forge.Instructions[i].Enabled = !s.forge.Instructions[i].Enabled
			break
		}
	}
	s.persist()
	s.writePartial(w, s.instructionsPartial())
}

func (s *Server) handleInstructionDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	for i := range s.forge.Instructions {
		if s.forge.Instructions[i].ID == id {
			s.forge.Instructions = append(s.forge.Instructions[:i], s.forge.Instructions[i+1:]...)
			break
		}
	}
	s.persist()
	s.writePartial(w, s.instructionsPartial())
}

func (s *Server) handleAgentSelect(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.config.DefaultAgent == id {
		s.config.DefaultAgent = "" // toggle off
	} else {
		s.config.DefaultAgent = id
	}
	if err := s.config.Save(); err != nil {
		s.logger.Printf("save config: %v", err)
	}
	s.writePartial(w, s.agentsPartial())
}

func (s *Server) handleAgentDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	for i := range s.forge.Agents {
		if s.forge.Agents[i].ID == id {
			s.forge.Agents = append(s.forge.Agents[:i], s.forge.Agents[i+1:]...)
			if s.config.DefaultAgent == id {
				s.config.DefaultAgent = ""
				_ = s.config.Save()
			}
			break
		}
	}
	s.persist()
	s.writePartial(w, s.agentsPartial())
}

// --- helpers ---

// oobTimeline renders an out-of-band #timeline refresh for POST responses.
// Caller must hold s.mu.
func (s *Server) oobTimeline() string {
	return `<div id="timeline" hx-swap-oob="innerHTML">` + renderTimelineInner(&s.state) + `</div>`
}

// dropPerm removes a resolved permission from the queue. Caller must hold s.mu.
func (s *Server) dropPerm(id string) {
	for i := range s.perms {
		if s.perms[i].ID == id {
			s.perms = append(s.perms[:i], s.perms[i+1:]...)
			return
		}
	}
}

func (s *Server) persist() {
	if s.forge == nil {
		return
	}
	if err := s.forge.Save(); err != nil {
		s.logger.Printf("save forge: %v", err)
	}
}

func (s *Server) writePartial(w http.ResponseWriter, html string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}
