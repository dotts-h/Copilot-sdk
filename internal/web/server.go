package web

import (
	"context"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/dotts-h/copilot-sdk/internal/copilot"
)

// Server is the htmx web frontend. It owns the per-process session state (single
// local user assumed) and bridges browser actions to the copilot.Client seam.
type Server struct {
	client copilot.Client
	spec   copilot.SessionSpec
	logger *log.Logger

	// demo streams a scripted reply on /send when the backing client cannot
	// produce one on its own (the MockClient-driven walking skeleton).
	demo bool

	mu        sync.Mutex
	sessionID string
}

// Options configures a Server.
type Options struct {
	Client copilot.Client
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
	return &Server{client: opts.Client, spec: opts.Spec, logger: lg, demo: opts.Demo}
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
	return mux
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

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pageTemplates.ExecuteTemplate(w, "index", nil); err != nil {
		s.logger.Printf("render index: %v", err)
	}
}

// handleSend echoes the user's prompt as a bubble (swapped above #cur-msg) and
// dispatches it to the client. The assistant's streamed reply arrives separately
// over the always-open /events channel — POST and SSE are decoupled so the
// composer is never disabled (WEB_UI_PLAN.md).
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

	_, _ = w.Write([]byte(userBubble(prompt)))
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
