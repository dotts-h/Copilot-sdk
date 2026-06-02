package copilot

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
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

	events chan Event
	done   chan struct{}
	once   sync.Once
}

// Options configures the SDK client.
type Options struct {
	GitHubToken string
	LogLevel    string // "error" by default
	// OTLPEndpoint, when set, enables OpenTelemetry trace/metric export.
	OTLPEndpoint string
}

// NewSDKClient constructs and starts a real SDK-backed client. It requires the
// `copilot` CLI to be available (it is not bundled for Go).
func NewSDKClient(ctx context.Context, opts Options) (*SDKClient, error) {
	logLevel := opts.LogLevel
	if logLevel == "" {
		logLevel = "error"
	}
	clientOpts := &sdk.ClientOptions{
		LogLevel:    logLevel,
		GitHubToken: opts.GitHubToken,
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
		case *sdk.AssistantMessageData:
			c.emit(Event{Type: EvMessage, Text: d.Content})
		case *sdk.AssistantUsageData:
			c.emit(Event{Type: EvUsage, Usage: normalizeUsage(d)})
		case *sdk.ToolExecutionStartData:
			c.mu.Lock()
			c.toolNames[d.ToolCallID] = d.ToolName
			c.mu.Unlock()
			c.emit(Event{Type: EvToolStart, Tool: d.ToolName})
		case *sdk.ToolExecutionCompleteData:
			c.mu.Lock()
			name := c.toolNames[d.ToolCallID]
			delete(c.toolNames, d.ToolCallID)
			c.mu.Unlock()
			c.emit(Event{Type: EvToolEnd, Tool: name})
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
