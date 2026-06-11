package copilot

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

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
	toolNames map[string]string   // toolCallID -> toolName, for matching end events
	toolMeta  map[string]toolMeta // toolCallID -> {kind, target}, for PostToolUse matching (ADR-0032)
	// toolSession reverse-indexes the live toolCallID -> sid so session teardown can
	// reclaim toolNames/toolMeta entries orphaned by a tool that started but never
	// completed (a mid-tool SDK error). Issue 0089.
	toolSession map[string]string
	reasoned    map[string]bool          // sid -> reasoning deltas streamed in the current segment
	policies    map[string]sessionPolicy // sid -> compiled governance policy (ADR-0029/0030)
	closed      bool

	// modelEfforts caches each model's supported reasoning efforts (from
	// ListModels) so CreateSession can avoid sending an effort to a model that
	// rejects it. nil until first successfully fetched.
	modelsMu     sync.Mutex
	modelEfforts map[string][]string

	// PostToolUse command-executor seams (G5, ADR-0032). runCmd execs a single
	// command (default execCommand); lookupEnv resolves a ${VAR} reference (default
	// os.Getenv); hookTimeout bounds one command (0 → defaultHookTimeout). They are
	// fields so the executor is unit-testable without spawning real processes.
	runCmd      commandRunner
	lookupEnv   func(string) string
	hookTimeout time.Duration

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

// ResolveAuthMethod dispatches the configured auth method (config.AuthMethod,
// ADR-0039) onto the Options pair. "gh" resolves the token through the ghToken
// seam (`gh auth token` at the call site); a method that cannot produce a token
// — unset var, gh missing/unauthenticated, nil seam — degrades to the
// logged-in `copilot` CLI session (auto) rather than failing the dial; the
// Connection page preflights make the degradation visible.
func ResolveAuthMethod(method, configured string, ghToken func() (string, error)) (githubToken string, useLoggedInUser *bool) {
	if method == "gh" {
		if ghToken != nil {
			if tok, err := ghToken(); err == nil && tok != "" {
				return ResolveAuth(tok)
			}
		}
		return ResolveAuth("")
	}
	// "token", auto ("") and any unknown method use the configured ${VAR} token
	// when it resolves, else the logged-in session — the pre-0068 behavior.
	return ResolveAuth(configured)
}

// authStatusFromSDK normalizes the SDK's auth.getStatus response, nil-safe on
// the response and each optional field. — ADR-0039.
func authStatusFromSDK(r *sdk.GetAuthStatusResponse) AuthStatus {
	if r == nil {
		return AuthStatus{}
	}
	deref := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	return AuthStatus{
		Authenticated: r.IsAuthenticated,
		Method:        deref(r.AuthType),
		Login:         deref(r.Login),
		Host:          deref(r.Host),
		Detail:        deref(r.StatusMessage),
	}
}

// AuthStatus implements Client: the runtime's live credential.
func (c *SDKClient) AuthStatus(ctx context.Context) (AuthStatus, error) {
	resp, err := c.client.GetAuthStatus(ctx)
	if err != nil {
		return AuthStatus{}, fmt.Errorf("auth status: %w", err)
	}
	return authStatusFromSDK(resp), nil
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
		client:      c,
		perms:       newPermBridge(),
		inputs:      newInputBridge(),
		plans:       newPlanBridge(),
		elicits:     newElicitBridge(),
		sessions:    make(map[string]*sdk.Session),
		unsubs:      make(map[string]func()),
		toolNames:   make(map[string]string),
		toolMeta:    make(map[string]toolMeta),
		toolSession: make(map[string]string),
		reasoned:    make(map[string]bool),
		policies:    make(map[string]sessionPolicy),
		runCmd:      execCommand,
		lookupEnv:   os.Getenv,
		events:      make(chan Event, 256),
		done:        make(chan struct{}),
	}, nil
}

// sessionPolicy is the per-session governance context the permission bridge
// consults by SessionID: the compiled hook set, whether the session runs with
// blanket auto-approve, and the workspace root the fence evaluates writes
// against. — ADR-0029, ADR-0030.
type sessionPolicy struct {
	hooks       []ctxforge.Hook
	autoApprove bool
	workspace   string
	// mode is the session's active agent mode, updated per turn at Send (mode
	// binding, ADR-0031). The bridge threads it into Evaluate (so a mode-scoped
	// hook participates) and resolves the auto-approve baseline from it
	// (autopilot → on, interactive → off, else the autoApprove config).
	mode string
}

// applyHandlers wires the permission/input/plan/elicit callbacks shared by
// CreateSession and ResumeSession. The policy-aware permissionHandler is ALWAYS
// used — the SDK's blanket ApproveAll is no longer wired, because the mandatory
// dangerous ruleset (G2) must run even when AutoApproveTools is set. The handler
// reads the session's recorded autoApprove flag and only blanket-approves the
// non-mandatory remainder. — ADR-0030.
func (c *SDKClient) applyHandlers() (onPerm sdk.PermissionHandlerFunc, onInput sdk.UserInputHandler, onPlan sdk.ExitPlanModeRequestHandler, onElicit sdk.ElicitationHandler) {
	return c.permissionHandler(), c.userInputHandler(), c.exitPlanModeHandler(), c.elicitationHandler()
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
	cfg.OnPermissionRequest, cfg.OnUserInputRequest, cfg.OnExitPlanModeRequest, cfg.OnElicitationRequest = c.applyHandlers()
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
	c.register(session, spec)
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
	cfg.OnPermissionRequest, cfg.OnUserInputRequest, cfg.OnExitPlanModeRequest, cfg.OnElicitationRequest = c.applyHandlers()
	if len(spec.AllowedTools) > 0 {
		cfg.AvailableTools = spec.AllowedTools
	}

	session, err := c.client.ResumeSession(ctx, sessionID, cfg)
	if err != nil {
		return "", fmt.Errorf("resume session %q: %w", sessionID, err)
	}
	c.register(session, spec)
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
	delete(c.policies, sessionID)
	c.sweepToolMaps(sessionID)
	c.mu.Unlock()
	return nil
}

// sweepToolMaps drops any toolNames/toolMeta entries owned by sid that a tool
// completion never reclaimed — a tool that started but never completed (a
// mid-tool SDK error) would otherwise leak its entry until Close. The caller
// must hold c.mu. Issue 0089.
func (c *SDKClient) sweepToolMaps(sid string) {
	for id, owner := range c.toolSession {
		if owner == sid {
			delete(c.toolNames, id)
			delete(c.toolMeta, id)
			delete(c.toolSession, id)
		}
	}
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
// and records the session's compiled governance policy (hooks + auto-approve +
// workspace root) so permissionHandler can consult it by SessionID when a
// tool-permission callback fires (ADR-0029, ADR-0030).
func (c *SDKClient) register(session *sdk.Session, spec SessionSpec) {
	unsub := session.On(c.makeHandler(session.SessionID))
	c.mu.Lock()
	c.sessions[session.SessionID] = session
	c.unsubs[session.SessionID] = unsub
	c.policies[session.SessionID] = sessionPolicy{
		hooks:       spec.Hooks,
		autoApprove: spec.AutoApproveTools,
		workspace:   spec.Workspace,
	}
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
	// Record the turn's active mode on the session policy so the permission bridge
	// (invoked synchronously during this Send) evaluates mode-scoped hooks and
	// resolves the auto-approve baseline for the right mode. — ADR-0031.
	if pol, ok := c.policies[sessionID]; ok {
		pol.mode = agentMode
		c.policies[sessionID] = pol
	}
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
		c.policies = map[string]sessionPolicy{}
		// Drop any tool entries orphaned by tools that never completed (issue 0089);
		// the whole client is going away, so reset rather than per-session sweep.
		c.toolNames = map[string]string{}
		c.toolMeta = map[string]toolMeta{}
		c.toolSession = map[string]string{}
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
