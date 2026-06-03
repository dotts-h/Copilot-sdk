package copilot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	sdk "github.com/github/copilot-sdk/go"
	"github.com/github/copilot-sdk/go/rpc"
)

// SDKClient is the production Client, backed by the official Copilot Go SDK.
// It translates the SDK's rich session events into normalized Events and fans
// them onto a single channel the TUI consumes.
type SDKClient struct {
	client *sdk.Client

	perms *permBridge

	mu        sync.Mutex
	sessions  map[string]*sdk.Session
	unsubs    map[string]func()
	toolNames map[string]string // toolCallID -> toolName, for matching end events
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
		sessions:  make(map[string]*sdk.Session),
		unsubs:    make(map[string]func()),
		toolNames: make(map[string]string),
		events:    make(chan Event, 256),
		done:      make(chan struct{}),
	}, nil
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
	if spec.AutoApproveTools {
		cfg.OnPermissionRequest = sdk.PermissionHandler.ApproveAll
	} else {
		cfg.OnPermissionRequest = c.permissionHandler()
	}
	if len(spec.MCPServers) > 0 {
		cfg.MCPServers = make(map[string]sdk.MCPServerConfig, len(spec.MCPServers))
		for _, s := range spec.MCPServers {
			cfg.MCPServers[s.Name] = sdk.MCPStdioServerConfig{
				Command: s.Command, Args: s.Args, Env: s.Env,
			}
		}
	}

	session, err := c.client.CreateSession(ctx, cfg)
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	c.register(session)
	return session.SessionID, nil
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

// ResumeSession implements Client.
func (c *SDKClient) ResumeSession(ctx context.Context, sessionID string) (string, error) {
	session, err := c.client.ResumeSession(ctx, sessionID, &sdk.ResumeSessionConfig{
		Streaming:           sdk.Bool(true),
		OnPermissionRequest: c.permissionHandler(),
	})
	if err != nil {
		return "", fmt.Errorf("resume session %q: %w", sessionID, err)
	}
	c.register(session)
	return session.SessionID, nil
}

// LastSessionID implements Client.
func (c *SDKClient) LastSessionID(ctx context.Context) (string, error) {
	id, err := c.client.GetLastSessionID(ctx)
	if err != nil {
		return "", err
	}
	if id == nil {
		return "", nil
	}
	return *id, nil
}

// register wires a session's event handler and tracks it for Send/Abort/Close.
func (c *SDKClient) register(session *sdk.Session) {
	unsub := session.On(c.makeHandler())
	c.mu.Lock()
	c.sessions[session.SessionID] = session
	c.unsubs[session.SessionID] = unsub
	c.mu.Unlock()
}

// makeHandler returns a session event handler that normalizes and forwards.
func (c *SDKClient) makeHandler() func(sdk.SessionEvent) {
	return func(ev sdk.SessionEvent) {
		switch d := ev.Data.(type) {
		case *sdk.AssistantMessageDeltaData:
			c.emit(Event{Type: EvMessageDelta, Text: d.DeltaContent})
		case *sdk.AssistantReasoningDeltaData:
			c.emit(Event{Type: EvReasoningDelta, Text: d.DeltaContent})
		case *sdk.AssistantReasoningData:
			c.emit(Event{Type: EvReasoning, Text: d.Content})
		case *sdk.AssistantMessageData:
			c.emit(Event{Type: EvMessage, Text: d.Content})
		case *sdk.AssistantUsageData:
			c.emit(Event{Type: EvUsage, Usage: normalizeUsage(d)})
		case *sdk.ToolExecutionStartData:
			c.mu.Lock()
			c.toolNames[d.ToolCallID] = d.ToolName
			c.mu.Unlock()
			tc := &ToolCall{ID: d.ToolCallID, Name: d.ToolName, Args: summarizeArgs(d.Arguments)}
			if d.MCPServerName != nil {
				tc.MCPServer = *d.MCPServerName
			}
			c.emit(Event{Type: EvToolStart, Tool: d.ToolName, ToolCall: tc})
		case *sdk.ToolExecutionProgressData:
			c.emit(Event{Type: EvToolProgress, ToolCall: &ToolCall{
				ID: d.ToolCallID, Progress: d.ProgressMessage,
			}})
		case *sdk.ToolExecutionCompleteData:
			c.mu.Lock()
			name := c.toolNames[d.ToolCallID]
			delete(c.toolNames, d.ToolCallID)
			c.mu.Unlock()
			c.emit(Event{Type: EvToolEnd, Tool: name, ToolCall: &ToolCall{
				ID: d.ToolCallID, Name: name, Success: d.Success, Result: toolResultText(d),
			}})
		case *sdk.SessionIdleData:
			c.emit(Event{Type: EvIdle})
		}
	}
}

// permissionHandler bridges the SDK's synchronous permission callback to the
// async TUI: it emits an EvPermission and blocks until Respond() (or shutdown).
func (c *SDKClient) permissionHandler() sdk.PermissionHandlerFunc {
	return func(req sdk.PermissionRequest, _ sdk.PermissionInvocation) (rpc.PermissionDecision, error) {
		id, ch := c.perms.begin()
		c.emit(Event{Type: EvPermission, Permission: &PermissionRequest{
			ID: id, Kind: string(req.Kind()), Detail: describePermission(req),
		}})
		select {
		case approve := <-ch:
			if approve {
				return &rpc.PermissionDecisionApproveOnce{}, nil
			}
			fb := "Rejected by user"
			return &rpc.PermissionDecisionReject{Feedback: &fb}, nil
		case <-c.done:
			return &rpc.PermissionDecisionUserNotAvailable{}, nil
		}
	}
}

// describePermission renders a short, human-readable summary of a request.
func describePermission(req sdk.PermissionRequest) string {
	switch r := req.(type) {
	case sdk.PermissionRequestShell:
		return "run shell: " + r.FullCommandText
	case sdk.PermissionRequestWrite:
		return "write file: " + r.FileName
	default:
		return string(req.Kind())
	}
}

// summarizeArgs renders a tool's arguments as a single concise line for the
// timeline. It favors the fields humans care about (shell command, file path)
// and otherwise falls back to compact JSON, truncated.
func summarizeArgs(args any) string {
	m, ok := args.(map[string]any)
	if !ok {
		return clip(oneLine(fmt.Sprint(args)), 120)
	}
	for _, k := range []string{"command", "commandLine", "fullCommandText", "cmd", "query", "pattern"} {
		if v, ok := stringField(m, k); ok {
			return clip(oneLine(v), 160)
		}
	}
	for _, k := range []string{"path", "filePath", "file", "fileName", "filename"} {
		if v, ok := stringField(m, k); ok {
			return clip(v, 120)
		}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return clip(oneLine(string(b)), 120)
}

func stringField(m map[string]any, key string) (string, bool) {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return s, true
		}
	}
	return "", false
}

// toolResultText extracts the most detailed displayable result from a completed
// tool execution, preferring the full UI content over the model-facing summary.
// On failure it surfaces the error message. Output is capped to keep the
// timeline bounded.
func toolResultText(d *sdk.ToolExecutionCompleteData) string {
	if !d.Success && d.Error != nil && d.Error.Message != "" {
		return clip(d.Error.Message, 4000)
	}
	if d.Result != nil {
		if d.Result.DetailedContent != nil && *d.Result.DetailedContent != "" {
			return clip(*d.Result.DetailedContent, 4000)
		}
		if d.Result.Content != "" {
			return clip(d.Result.Content, 4000)
		}
	}
	return ""
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

// Respond implements Client.
func (c *SDKClient) Respond(id string, approve bool) error {
	if !c.perms.resolve(id, approve) {
		return fmt.Errorf("no pending permission %q", id)
	}
	return nil
}

// normalizeUsage maps the SDK's assistant.usage payload onto UsageData,
// dereferencing optional pointers safely.
func normalizeUsage(d *sdk.AssistantUsageData) UsageData {
	u := UsageData{
		Model:           d.Model,
		InputTokens:     deref(d.InputTokens),
		CachedTokens:    deref(d.CacheReadTokens),
		OutputTokens:    deref(d.OutputTokens),
		ReasoningTokens: deref(d.ReasoningTokens),
	}
	if d.CopilotUsage != nil {
		u.NanoAIU = d.CopilotUsage.TotalNanoAiu
	}
	return u
}

func deref(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

// Send implements Client.
func (c *SDKClient) Send(ctx context.Context, sessionID, prompt string, attachments []string) error {
	c.mu.Lock()
	session := c.sessions[sessionID]
	c.mu.Unlock()
	if session == nil {
		return fmt.Errorf("unknown session %q", sessionID)
	}
	opts := sdk.MessageOptions{Prompt: prompt}
	for _, p := range attachments {
		opts.Attachments = append(opts.Attachments, &sdk.AttachmentFile{
			Path: p, DisplayName: filepath.Base(p),
		})
	}
	_, err := session.Send(ctx, opts)
	return err
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
