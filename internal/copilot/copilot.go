// Package copilot wraps the official GitHub Copilot Go SDK
// (github.com/github/copilot-sdk/go) behind a small interface tailored to
// my-orchestra's TUI. The interface exists for one reason: testability. The TUI
// is driven entirely through Client, so it can be exercised with an in-memory
// mock, while production uses the real SDK-backed implementation.
package copilot

import (
	"context"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
)

// EventType enumerates the normalized events the TUI reacts to. They collapse
// the SDK's richer event set into just what the UI needs.
type EventType int

const (
	EvUnknown EventType = iota
	EvMessageDelta
	EvReasoningDelta
	EvReasoning // full reasoning text (non-streaming)
	EvMessage
	EvUsage
	EvToolStart
	EvToolProgress
	EvToolEnd
	EvIdle
	EvError
	EvPermission
	EvContextWindow   // context-window usage update (SessionUsageInfoData)
	EvCompactionStart // conversation compaction began
	EvCompactionEnd   // conversation compaction finished (Text carries a summary)
	EvUserInput       // the agent is asking the user a question (ask_user tool)
	EvPlanReview      // the agent finished a plan and is asking to exit plan mode
	EvPlanChanged     // the plan file was created/updated/deleted (notification)
	EvSubagentStart   // a sub-agent began running (background activity)
	EvSubagentEnd     // a sub-agent finished (Subagent.Success reports outcome)
	EvElicitation     // an MCP server is requesting structured input (a schema-driven form)
	EvUserMessage     // a past user prompt, emitted only when rehydrating history (never live)
	EvToolDecision    // a governance hook auto-approved or denied a tool call (timeline "why", ADR-0031)
	EvHookRun         // a PostToolUse hook ran an external command; carries its untrusted output (ADR-0032)
)

// SessionMeta is the normalized metadata for a persisted session, used to render
// the session picker.
type SessionMeta struct {
	ID       string
	Summary  string    // a short title/summary the runtime keeps, may be empty
	Modified time.Time // last-modified time, for ordering and display
}

// PermissionRequest describes a tool-permission prompt awaiting a decision.
// For a file-write request the runtime also supplies the proposed change, so the
// UI can render a review lane (item 3.1): FileName, a one-line Intention, and the
// unified Diff. These are empty for non-write requests (e.g. shell), where Detail
// alone carries the summary.
type PermissionRequest struct {
	ID        string
	Kind      string
	Detail    string
	FileName  string // path being written (write requests only)
	Intention string // human-readable description of the change (write requests only)
	Diff      string // unified diff of the proposed change (write requests only)
	// Reason is the message of the governance hook that gated this call (a
	// mandatory ask, e.g. "confirm: sudo escalates privileges"), surfaced on the
	// gate so the human sees WHY they're being asked. Empty for a plain gate with
	// no matching hook (the default ask). — ADR-0031.
	Reason string
}

// InputRequest describes an ask_user prompt awaiting an answer: a question, an
// optional set of suggested choices, and whether a freeform answer is allowed.
// It is the elicitation analogue of PermissionRequest.
type InputRequest struct {
	ID            string
	Question      string
	Choices       []string
	AllowFreeform bool
}

// SubagentInfo is the normalized view of a sub-agent's lifecycle, surfaced as a
// background-activity indicator. ToolCallID (the parent tool invocation that
// spawned it) threads the start and end events so the UI can update one entry.
// Detail carries a one-line summary on completion (duration/tokens) or the error
// message on failure.
type SubagentInfo struct {
	ToolCallID  string
	Name        string
	DisplayName string
	Description string
	Model       string
	Success     bool
	Detail      string
}

// ElicitRequest describes a schema-driven elicitation form awaiting input: an
// MCP server (via OnElicitationRequest) asking the user for structured data. It
// is distinct from InputRequest (a single ask_user question) — it carries a
// message, the requesting source, and a set of typed form Fields. Resolved via
// RespondElicit. It reuses the bridge plumbing (a synchronous SDK callback
// resolved asynchronously by the web UI).
type ElicitRequest struct {
	ID      string
	Message string
	Source  string // the MCP server / source that initiated the request
	Fields  []ElicitField
}

// ElicitField is one field of an elicitation form, normalized for display: its
// form key (Name), label, help text, primitive type, whether it is required,
// the default value, and — for single-select fields — the allowed enum values
// with optional display labels.
type ElicitField struct {
	Name        string
	Label       string
	Description string
	// Type is the field's primitive type: "string", "boolean", "number", or
	// "integer". It selects the control rendered for the field.
	Type       string
	Required   bool
	Default    string
	Enum       []string // allowed values for a single-select field; empty otherwise
	EnumLabels []string // display labels parallel to Enum (may be empty)
}

// PlanRequest describes an exit-plan-mode prompt awaiting a decision: the
// agent's plan summary and full content, the actions the user may pick to
// proceed, and the action the agent recommends. It reuses the elicitation
// request/response plumbing (a synchronous SDK callback resolved via RespondPlan).
type PlanRequest struct {
	ID          string
	Summary     string
	Plan        string
	Actions     []string
	Recommended string
}

// UsageData is normalized token accounting derived from the SDK's
// assistant.usage event. It captures every token category GitHub meters.
type UsageData struct {
	Model            string
	InputTokens      int64
	CachedTokens     int64 // prompt-cache reads
	CacheWriteTokens int64 // prompt-cache writes
	OutputTokens     int64
	ReasoningTokens  int64
	// NanoAIU is GitHub's own authoritative cost for the call, in nano-AI units
	// (1e-9 AIU). Zero when the runtime does not report it.
	NanoAIU float64
}

// Event is a normalized, already-decoded notification from a session.
type Event struct {
	Type EventType
	// SessionID is the copilot session the event came from, so a multi-session
	// frontend can route it to the right conversation. Empty for events from a
	// MockClient (demo/tests), which a single-session consumer can ignore.
	SessionID string
	Text      string // delta or full message/reasoning text
	Tool      string // tool name for tool events

	// ToolCall carries the timeline detail for tool events (EvToolStart,
	// EvToolProgress, EvToolEnd). Nil for non-tool events.
	ToolCall *ToolCall

	Usage      UsageData
	Context    ContextInfo        // set for EvContextWindow
	Permission *PermissionRequest // set for EvPermission
	Input      *InputRequest      // set for EvUserInput
	Plan       *PlanRequest       // set for EvPlanReview
	Elicit     *ElicitRequest     // set for EvElicitation
	Subagent   *SubagentInfo      // set for EvSubagentStart / EvSubagentEnd
	Decision   *ToolDecision      // set for EvToolDecision
	HookRun    *HookRun           // set for EvHookRun
	Err        error
}

// HookRun is the normalized result of a PostToolUse hook executing its external
// command after a matching tool completed (G5, ADR-0032). It is display-only
// TELEMETRY: Output is the command's combined stdout/stderr, captured UNTRUSTED
// and bounded — it is never fed back to the agent and can never flip a permission
// decision. HookID names the hook, Command is the resolved command line (with
// ${VAR}s already substituted, secrets omitted), ExitCode is the process exit
// status (-1 when it never started or was killed), TimedOut reports the command
// exceeded the executor's deadline, and Failed is true on a non-zero exit, a
// timeout, or a start error. The reducer renders Output ESCAPED (ADR-0001).
type HookRun struct {
	HookID   string
	Command  string
	Output   string
	ExitCode int
	TimedOut bool
	Failed   bool
}

// ToolDecision is the normalized "why" of a governance decision the permission
// bridge made WITHOUT pausing for the human gate — surfaced in the timeline so an
// auto-approve or a hard-deny is explainable (ADR-0031). Kind is the resolved
// action (allow|deny), HookID names the winning hook ("auto-approved by X"),
// Reason is its message, and Detail summarises the call (kind + command). A
// gated (ask) decision is NOT a ToolDecision — it already surfaces as the
// EvPermission form, which now carries the same Reason.
type ToolDecision struct {
	Kind   string // "allow" | "deny" (ctxforge.HookAllow / HookDeny)
	HookID string
	Reason string
	Detail string // one-line summary of the governed call (tool kind + command)
}

// ContextInfo is normalized context-window accounting from the SDK's
// session.usage_info event: how many tokens of the model's context window are
// currently in use, and the window's total size. Drives the live context meter.
type ContextInfo struct {
	CurrentTokens int64
	TokenLimit    int64
}

// ModelInfo is a normalized, selectable model from ListModels: its id (passed
// to SessionSpec.Model), display name, and the reasoning efforts it accepts.
type ModelInfo struct {
	ID                        string
	Name                      string
	SupportedReasoningEfforts []string
}

// AuthStatus is the normalized live credential from the runtime's
// auth.getStatus: whether a credential is live and, when known, which kind,
// for whom, and against which host. Method is OPAQUE display text (the SDK's
// AuthType vocabulary is unverified — issue 0067), never an enum to switch on.
// Read-only: the runtime exposes no auth write surface. — ADR-0039.
type AuthStatus struct {
	Authenticated bool
	Method        string // the SDK's AuthType, e.g. an OAuth/token/gh label
	Login         string
	Host          string
	Detail        string // the SDK's StatusMessage
}

// ToolCall is the normalized, displayable view of a single tool execution as it
// moves through start → progress → completion. The same ID threads all three
// phases so the UI can update one timeline entry in place.
type ToolCall struct {
	ID   string
	Name string
	// Args is a short, human-readable summary of the tool's arguments
	// (e.g. the shell command or the file path), suitable for one line.
	Args string
	// Progress is the latest progress message (EvToolProgress), if any.
	Progress string
	// Result is the detailed result content for timeline display (EvToolEnd).
	Result string
	// Success reports whether the tool completed without error (EvToolEnd).
	Success bool
	// MCPServer names the hosting MCP server when the tool is an MCP tool.
	MCPServer string
}

// SessionSpec is the subset of SDK session options my-orchestra drives.
type SessionSpec struct {
	Model            string
	ReasoningEffort  string
	SystemMessage    string
	Streaming        bool
	AutoApproveTools bool
	MCPServers       []MCPServer
	// AllowedTools restricts the session to these tool names (maps to the SDK's
	// AvailableTools). Empty = all tools available.
	AllowedTools []string
	// Hooks is the compiled governance policy for the session (built-in safe
	// defaults + the built-in mandatory dangerous ruleset + the forge's enabled
	// user hooks). The permission bridge consults it via ctxforge.Evaluate before
	// the interactive gate: a PreToolUse decision of allow auto-approves, deny
	// rejects with the reason, and ask falls through to the human-in-the-loop gate.
	// This generalizes the flat AutoApproveTools from an all-or-nothing switch to a
	// per-tool ruleset. The mandatory subset (dangerous-action deny/ask) is enforced
	// even when AutoApproveTools is set. — ADR-0029, ADR-0030.
	Hooks []ctxforge.Hook
	// Workspace is the session's workspace root (absolute), threaded to the
	// evaluator so the built-in fence can deny/gate a write whose target resolves
	// OUTSIDE the project tree. Empty disables the fence. — ADR-0030.
	Workspace string
}

// MCPServer is a stdio MCP server to expose to the session.
type MCPServer struct {
	// ID is the stable, unique key the server registers under (the forge guarantees
	// uniqueness). Name is the human label; it is neither unique nor required, so it
	// must not be used as a map key (see CreateSession). ID falls back to Name only
	// for legacy callers that set no ID.
	ID      string
	Name    string
	Command string
	Args    []string
	Env     map[string]string
}

// Key returns the unique identifier the server registers under: its ID, or its
// Name when no ID is set (legacy callers). Used as the SDK MCP-config map key so
// two servers with the same (or empty) Name can't silently overwrite each other.
func (m MCPServer) Key() string {
	if m.ID != "" {
		return m.ID
	}
	return m.Name
}

// Client is the dependency the TUI talks to.
type Client interface {
	// CreateSession opens a session and returns its id.
	CreateSession(ctx context.Context, spec SessionSpec) (string, error)
	// Send submits a prompt (with optional file/image attachment paths); output
	// arrives as events. agentMode selects the turn's UI mode ("plan",
	// "autopilot", "interactive", "shell"); "" uses the session's current mode.
	Send(ctx context.Context, sessionID, prompt string, attachments []string, agentMode string) error
	// Abort cancels the in-flight turn for a session.
	Abort(ctx context.Context, sessionID string) error
	// Respond answers a pending tool-permission request (EvPermission).
	Respond(id string, approve bool) error
	// RespondInput answers a pending ask_user request (EvUserInput) with the
	// user's chosen or freeform answer.
	RespondInput(id, answer string) error
	// RespondPlan answers a pending exit-plan-mode request (EvPlanReview): either
	// approve and proceed with action, or decline with feedback requesting changes.
	RespondPlan(id string, approved bool, action, feedback string) error
	// RespondElicit answers a pending elicitation request (EvElicitation). action
	// is "accept" (with the submitted form content), "decline", or "cancel";
	// content is ignored unless action is "accept".
	RespondElicit(id, action string, content map[string]any) error
	// ListModels returns the models available to the account.
	ListModels(ctx context.Context) ([]ModelInfo, error)
	// AuthStatus reports the live credential (auth.getStatus): authenticated?,
	// method, login, host. Read-only — changing the method is a config edit
	// applied at the next dial (ADR-0039).
	AuthStatus(ctx context.Context) (AuthStatus, error)
	// ListSessions returns metadata for the persisted sessions, most-recent first.
	ListSessions(ctx context.Context) ([]SessionMeta, error)
	// ResumeSession reattaches to a persisted session by id (wiring the same event
	// handlers as CreateSession) and returns its id. The runtime restores the full
	// conversation context from disk; the next turn sees the entire prior history.
	ResumeSession(ctx context.Context, sessionID string, spec SessionSpec) (string, error)
	// SessionHistory returns a session's persisted conversation as normalized
	// events (user/assistant/reasoning/tool), for rebuilding the transcript.
	SessionHistory(ctx context.Context, sessionID string) ([]Event, error)
	// DeleteSession permanently removes a persisted session and its on-disk state.
	DeleteSession(ctx context.Context, sessionID string) error
	// Events streams normalized events until Close.
	Events() <-chan Event
	// Close releases all resources (stops the runtime).
	Close() error
}
