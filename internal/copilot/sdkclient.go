package copilot

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"sync"

	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
	sdk "github.com/github/copilot-sdk/go"
)

// This file is the SDKClient session lifecycle and Client-interface
// implementation. The SDK→Event normalization lives in normalize.go and the
// sync↔async permission/input/plan/elicit bridges in handlers.go — together they
// form the one boundary that "knows the SDK exists" (CONVENTIONS seam-purity rule).

// SDKClient is the production Client, backed by the official Copilot Go SDK.
// It translates the SDK's rich session events into normalized Events and fans
// them onto a single channel the TUI consumes.
type SDKClient struct {
	client *sdk.Client

	perms   *bridge[bool]
	inputs  *bridge[string]
	plans   *bridge[planDecision]
	elicits *bridge[elicitDecision]

	mu        sync.Mutex
	sessions  map[string]*sdk.Session
	unsubs    map[string]func()
	toolNames map[string]string          // toolCallID -> toolName, for matching end events
	reasoned  map[string]bool            // sid -> reasoning deltas streamed in the current segment
	hooks     map[string][]ctxforge.Hook // sid -> compiled governance policy (ADR-0029)
	closed    bool

	// modelEfforts caches each model's supported reasoning efforts (from
	// ListModels) so CreateSession can avoid sending an effort to a model that
	// rejects it. nil until first successfully fetched.
	modelsMu     sync.Mutex
	modelEfforts map[string][]string

	events chan Event
	done   chan struct{}
	once   sync.Once
}

// Options configures the SDK client.
type Options struct {
	// GitHubToken, when non-empty, is used as an explicit auth token and takes
	// priority over the logged-in session. Leave it empty to use the
	// already-logged-in `copilot` CLI session (see UseLoggedInUser).
	GitHubToken string
	// UseLoggedInUser, when non-nil, controls whether the runtime authenticates
	// with the logged-in `copilot` CLI session (stored OAuth / gh CLI auth).
	// It defaults to true unless GitHubToken is set. Use ResolveAuth to derive it.
	UseLoggedInUser *bool
	LogLevel        string // "error" by default
	// OTLPEndpoint, when set, enables OpenTelemetry trace/metric export.
	OTLPEndpoint string
}

// ResolveAuth derives the auth-related Options fields for an explicit token.
// With no explicit token it selects the already-logged-in `copilot` CLI session
// (UseLoggedInUser=true); a non-empty token is used as an explicit override and
// the runtime then ignores the logged-in session.
func ResolveAuth(token string) (githubToken string, useLoggedInUser *bool) {
	if token == "" {
		loggedIn := true
		return "", &loggedIn
	}
	return token, nil
}

// NewSDKClient constructs and starts a real SDK-backed client. It requires the
// `copilot` CLI to be available (it is not bundled for Go).
func NewSDKClient(ctx context.Context, opts Options) (*SDKClient, error) {
	logLevel := opts.LogLevel
	if logLevel == "" {
		logLevel = "error"
	}
	clientOpts := &sdk.ClientOptions{
		LogLevel:        logLevel,
		GitHubToken:     opts.GitHubToken,
		UseLoggedInUser: opts.UseLoggedInUser,
	}
	if opts.OTLPEndpoint != "" {
		clientOpts.Telemetry = &sdk.TelemetryConfig{
			OTLPEndpoint: opts.OTLPEndpoint,
			ExporterType: "otlp-http",
			SourceName:   "my-orchestra",
		}
	}
	c := sdk.NewClient(clientOpts)
	if err := c.Start(ctx); err != nil {
		return nil, fmt.Errorf("start copilot runtime: %w", err)
	}
	return &SDKClient{
		client:    c,
		perms:     newPermBridge(),
		inputs:    newInputBridge(),
		plans:     newPlanBridge(),
		elicits:   newElicitBridge(),
		sessions:  make(map[string]*sdk.Session),
		unsubs:    make(map[string]func()),
		toolNames: make(map[string]string),
		reasoned:  make(map[string]bool),
		hooks:     make(map[string][]ctxforge.Hook),
		events:    make(chan Event, 256),
		done:      make(chan struct{}),
	}, nil
}

// applyHandlers wires the permission/input/plan/elicit callbacks shared by
// CreateSession and ResumeSession. AutoApproveTools swaps the interactive
// permission bridge for the SDK's approve-all handler.
func (c *SDKClient) applyHandlers(autoApprove bool) (onPerm sdk.PermissionHandlerFunc, onInput sdk.UserInputHandler, onPlan sdk.ExitPlanModeRequestHandler, onElicit sdk.ElicitationHandler) {
	if autoApprove {
		onPerm = sdk.PermissionHandler.ApproveAll
	} else {
		onPerm = c.permissionHandler()
	}
	return onPerm, c.userInputHandler(), c.exitPlanModeHandler(), c.elicitationHandler()
}

// CreateSession implements Client.
func (c *SDKClient) CreateSession(ctx context.Context, spec SessionSpec) (string, error) {
	cfg := &sdk.SessionConfig{
		Model:           spec.Model,
		ReasoningEffort: spec.ReasoningEffort,
		Streaming:       sdk.Bool(spec.Streaming),
	}
	// Some models (e.g. "auto", claude-haiku-4.5) reject any reasoning-effort
	// setting; sending one fails session.create. Drop it when the model is known
	// not to support the requested effort.
	if cfg.ReasoningEffort != "" {
		supported, known := c.modelReasoningEfforts(ctx, spec.Model)
		if shouldDropReasoningEffort(cfg.ReasoningEffort, supported, known) {
			cfg.ReasoningEffort = ""
		}
	}
	if spec.SystemMessage != "" {
		cfg.SystemMessage = &sdk.SystemMessageConfig{Mode: "append", Content: spec.SystemMessage}
	}
	cfg.OnPermissionRequest, cfg.OnUserInputRequest, cfg.OnExitPlanModeRequest, cfg.OnElicitationRequest = c.applyHandlers(spec.AutoApproveTools)
	if len(spec.AllowedTools) > 0 {
		cfg.AvailableTools = spec.AllowedTools
	}
	if len(spec.MCPServers) > 0 {
		cfg.MCPServers = make(map[string]sdk.MCPServerConfig, len(spec.MCPServers))
		for _, s := range spec.MCPServers {
			// Key by the unique id (not the non-unique Name) so two enabled servers
			// with the same or empty Name can't collide and silently drop one.
			cfg.MCPServers[s.Key()] = sdk.MCPStdioServerConfig{
				Command: s.Command, Args: s.Args, Env: s.Env,
			}
		}
	}

	session, err := c.client.CreateSession(ctx, cfg)
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	c.register(session, spec.Hooks)
	return session.SessionID, nil
}

// ListSessions implements Client: persisted sessions, most-recent first.
func (c *SDKClient) ListSessions(ctx context.Context) ([]SessionMeta, error) {
	metas, err := c.client.ListSessions(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	out := make([]SessionMeta, 0, len(metas))
	for _, m := range metas {
		sm := SessionMeta{ID: m.SessionID, Modified: m.ModifiedTime}
		if m.Summary != nil {
			sm.Summary = *m.Summary
		}
		out = append(out, sm)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Modified.After(out[j].Modified) })
	return out, nil
}

// ResumeSession implements Client: reattach to a persisted session, wiring the
// same handlers as CreateSession so live events flow over Events() as usual.
// Model/effort/tools are re-applied; the system message and MCP servers are not
// re-sent on resume — the runtime restores them from the persisted session.
func (c *SDKClient) ResumeSession(ctx context.Context, sessionID string, spec SessionSpec) (string, error) {
	cfg := &sdk.ResumeSessionConfig{
		Model:           spec.Model,
		ReasoningEffort: spec.ReasoningEffort,
		Streaming:       sdk.Bool(spec.Streaming),
	}
	if cfg.ReasoningEffort != "" {
		supported, known := c.modelReasoningEfforts(ctx, spec.Model)
		if shouldDropReasoningEffort(cfg.ReasoningEffort, supported, known) {
			cfg.ReasoningEffort = ""
		}
	}
	cfg.OnPermissionRequest, cfg.OnUserInputRequest, cfg.OnExitPlanModeRequest, cfg.OnElicitationRequest = c.applyHandlers(spec.AutoApproveTools)
	if len(spec.AllowedTools) > 0 {
		cfg.AvailableTools = spec.AllowedTools
	}

	session, err := c.client.ResumeSession(ctx, sessionID, cfg)
	if err != nil {
		return "", fmt.Errorf("resume session %q: %w", sessionID, err)
	}
	c.register(session, spec.Hooks)
	return session.SessionID, nil
}

// SessionHistory implements Client: the persisted conversation of an
// already-resumed session, normalized for transcript rebuild.
func (c *SDKClient) SessionHistory(ctx context.Context, sessionID string) ([]Event, error) {
	c.mu.Lock()
	session := c.sessions[sessionID]
	c.mu.Unlock()
	if session == nil {
		return nil, fmt.Errorf("session %q not resumed", sessionID)
	}
	raw, err := session.GetEvents(ctx)
	if err != nil {
		return nil, fmt.Errorf("session history %q: %w", sessionID, err)
	}
	return historyEvents(sessionID, raw), nil
}

// DeleteSession implements Client: remove the persisted session and stop tracking
// it locally.
func (c *SDKClient) DeleteSession(ctx context.Context, sessionID string) error {
	if err := c.client.DeleteSession(ctx, sessionID); err != nil {
		return fmt.Errorf("delete session %q: %w", sessionID, err)
	}
	c.mu.Lock()
	if unsub := c.unsubs[sessionID]; unsub != nil {
		unsub()
	}
	delete(c.sessions, sessionID)
	delete(c.unsubs, sessionID)
	delete(c.hooks, sessionID)
	c.mu.Unlock()
	return nil
}

// shouldDropReasoningEffort reports whether a requested reasoning effort must be
// cleared before creating a session. known is whether the model's capabilities
// were resolved; when false (capabilities unknown), the effort is left untouched
// so behavior degrades to the runtime's own validation.
func shouldDropReasoningEffort(effort string, supported []string, known bool) bool {
	if effort == "" || !known {
		return false
	}
	if len(supported) == 0 {
		return true // model accepts no reasoning-effort setting at all
	}
	for _, e := range supported {
		if e == effort {
			return false
		}
	}
	return true // requested effort is not in the model's supported set
}

// modelReasoningEfforts returns the supported reasoning efforts for a model and
// whether that was determined. It lazily caches ListModels; on a list failure it
// reports known=false so callers do not drop the effort.
func (c *SDKClient) modelReasoningEfforts(ctx context.Context, model string) (efforts []string, known bool) {
	c.modelsMu.Lock()
	cache := c.modelEfforts
	c.modelsMu.Unlock()

	if cache == nil {
		models, err := c.client.ListModels(ctx)
		if err != nil {
			return nil, false
		}
		cache = make(map[string][]string, len(models))
		for _, m := range models {
			cache[m.ID] = m.SupportedReasoningEfforts
		}
		c.modelsMu.Lock()
		c.modelEfforts = cache
		c.modelsMu.Unlock()
	}
	efforts, known = cache[model]
	return efforts, known
}

// register wires a session's event handler and tracks it for Send/Abort/Close,
// and records the session's compiled hook policy so permissionHandler can consult
// it by SessionID when a tool-permission callback fires (ADR-0029).
func (c *SDKClient) register(session *sdk.Session, hooks []ctxforge.Hook) {
	unsub := session.On(c.makeHandler(session.SessionID))
	c.mu.Lock()
	c.sessions[session.SessionID] = session
	c.unsubs[session.SessionID] = unsub
	c.hooks[session.SessionID] = hooks
	c.mu.Unlock()
}

// ListModels implements Client, mapping the SDK's ModelInfo onto the normalized
// subset the UI needs (id, name, supported reasoning efforts).
func (c *SDKClient) ListModels(ctx context.Context) ([]ModelInfo, error) {
	models, err := c.client.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ModelInfo, len(models))
	for i, m := range models {
		out[i] = ModelInfo{ID: m.ID, Name: m.Name, SupportedReasoningEfforts: m.SupportedReasoningEfforts}
	}
	return out, nil
}

// Respond implements Client.
func (c *SDKClient) Respond(id string, approve bool) error {
	if !c.perms.resolve(id, approve) {
		return fmt.Errorf("no pending permission %q", id)
	}
	return nil
}

// RespondInput implements Client.
func (c *SDKClient) RespondInput(id, answer string) error {
	if !c.inputs.resolve(id, answer) {
		return fmt.Errorf("no pending input %q", id)
	}
	return nil
}

// RespondPlan implements Client.
func (c *SDKClient) RespondPlan(id string, approved bool, action, feedback string) error {
	if !c.plans.resolve(id, planDecision{Approved: approved, Action: action, Feedback: feedback}) {
		return fmt.Errorf("no pending plan review %q", id)
	}
	return nil
}

// RespondElicit implements Client.
func (c *SDKClient) RespondElicit(id, action string, content map[string]any) error {
	if !c.elicits.resolve(id, elicitDecision{Action: action, Content: content}) {
		return fmt.Errorf("no pending elicitation %q", id)
	}
	return nil
}

// Send implements Client.
func (c *SDKClient) Send(ctx context.Context, sessionID, prompt string, attachments []string, agentMode string) error {
	c.mu.Lock()
	session := c.sessions[sessionID]
	c.mu.Unlock()
	if session == nil {
		return fmt.Errorf("unknown session %q", sessionID)
	}
	opts := sdk.MessageOptions{Prompt: prompt, AgentMode: toAgentMode(agentMode)}
	for _, p := range attachments {
		opts.Attachments = append(opts.Attachments, &sdk.AttachmentFile{
			Path: p, DisplayName: filepath.Base(p),
		})
	}
	_, err := session.Send(ctx, opts)
	return err
}

// toAgentMode maps a normalized mode string onto the SDK's AgentMode. An unknown
// or empty value yields the zero AgentMode, which the runtime treats as the
// session's current mode.
func toAgentMode(mode string) sdk.AgentMode {
	switch mode {
	case "plan":
		return sdk.AgentModePlan
	case "autopilot":
		return sdk.AgentModeAutopilot
	case "interactive":
		return sdk.AgentModeInteractive
	case "shell":
		return sdk.AgentModeShell
	default:
		return ""
	}
}

// Abort implements Client.
func (c *SDKClient) Abort(ctx context.Context, sessionID string) error {
	c.mu.Lock()
	session := c.sessions[sessionID]
	c.mu.Unlock()
	if session == nil {
		return fmt.Errorf("unknown session %q", sessionID)
	}
	return session.Abort(ctx)
}

// Events implements Client.
func (c *SDKClient) Events() <-chan Event { return c.events }

// emit delivers an event unless the client is shutting down.
func (c *SDKClient) emit(e Event) {
	select {
	case c.events <- e:
	case <-c.done:
	}
}

// Close implements Client. It is idempotent.
func (c *SDKClient) Close() error {
	var err error
	c.once.Do(func() {
		c.mu.Lock()
		c.closed = true
		for _, unsub := range c.unsubs {
			if unsub != nil {
				unsub()
			}
		}
		sessions := c.sessions
		c.sessions = map[string]*sdk.Session{}
		c.unsubs = map[string]func(){}
		c.hooks = map[string][]ctxforge.Hook{}
		c.mu.Unlock()

		for _, s := range sessions {
			_ = s.Disconnect()
		}
		// Close done first so any in-flight callback's emit() unblocks via the
		// done branch. We deliberately do NOT close c.events: an SDK callback
		// goroutine may still be mid-emit, and closing the channel would risk a
		// "send on closed channel" panic. The runtime is being torn down and the
		// consumer (the TUI) is exiting, so leaving events open is harmless.
		close(c.done)
		err = c.client.Stop()
	})
	return err
}

// ensure interface compliance at compile time.
var _ Client = (*SDKClient)(nil)
var _ Client = (*MockClient)(nil)

// ErrClosed is returned when operating on a closed client.
var ErrClosed = errors.New("copilot: client closed")
