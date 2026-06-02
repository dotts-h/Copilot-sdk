// Package copilot is the Go bridge to the GitHub Copilot SDK. Because the SDK
// ships only for Node/Python/.NET, my-orchestra runs a thin Node "sidecar" that
// embeds @github/copilot-sdk and speaks a line-delimited JSON protocol over
// stdio. This package owns the Go half of that protocol.
//
// Wire format: each line is one JSON object.
//
// Go -> sidecar (request):
//
//	{"id": 1, "method": "createSession", "params": {...}}
//
// sidecar -> Go (response to a request):
//
//	{"id": 1, "result": {...}}            // success
//	{"id": 1, "error": "message"}          // failure
//
// sidecar -> Go (unsolicited event, no id):
//
//	{"event": "assistant.message_delta", "data": {...}}
package copilot

import "encoding/json"

// Request is a Go->sidecar method call.
type Request struct {
	ID     int             `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Response is a sidecar->Go reply correlated by ID.
type Response struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// Frame is the union used when decoding a line: it may be a response (has a
// non-zero id with result/error) or an event (has an event name).
type Frame struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
	Event  string          `json:"event,omitempty"`
	Data   json.RawMessage `json:"data,omitempty"`
}

// isResponse reports whether the frame is a reply to a request.
func (f Frame) isResponse() bool { return f.Event == "" && f.ID != 0 }

// Event types emitted by the sidecar, mirroring the SDK's event names.
const (
	EventMessageDelta   = "assistant.message_delta"
	EventReasoningDelta = "assistant.reasoning_delta"
	EventMessage        = "assistant.message"
	EventToolStart      = "tool.execution_start"
	EventToolComplete   = "tool.execution_complete"
	EventUsage          = "usage"
	EventIdle           = "session.idle"
	EventError          = "error"
)

// Event is a decoded sidecar event delivered to consumers.
type Event struct {
	Type string
	Data json.RawMessage
}

// DeltaData carries streaming text for message/reasoning delta events.
type DeltaData struct {
	SessionID    string `json:"sessionId"`
	DeltaContent string `json:"deltaContent"`
}

// MessageData carries a complete assistant message.
type MessageData struct {
	SessionID string `json:"sessionId"`
	Content   string `json:"content"`
}

// ToolData describes a tool execution boundary.
type ToolData struct {
	SessionID string `json:"sessionId"`
	ToolName  string `json:"toolName"`
	Status    string `json:"status,omitempty"`
}

// UsageData is a token-accounting event used to drive the telemetry meter.
type UsageData struct {
	SessionID    string `json:"sessionId"`
	Model        string `json:"model"`
	InputTokens  int64  `json:"inputTokens"`
	CachedTokens int64  `json:"cachedTokens"`
	OutputTokens int64  `json:"outputTokens"`
}

// ErrorData carries a sidecar/runtime error surfaced as an event.
type ErrorData struct {
	SessionID string `json:"sessionId"`
	Message   string `json:"message"`
}

// SessionSpec is the subset of SDK session options my-orchestra drives.
type SessionSpec struct {
	Model            string      `json:"model,omitempty"`
	ReasoningEffort  string      `json:"reasoningEffort,omitempty"`
	SystemMessage    string      `json:"systemMessage,omitempty"`
	Streaming        bool        `json:"streaming"`
	AutoApproveTools bool        `json:"autoApproveTools"`
	MCPServers       []MCPServer `json:"mcpServers,omitempty"`
}

// MCPServer mirrors the forge's MCP server config on the wire.
type MCPServer struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// createSessionResult is the result payload of a createSession call.
type createSessionResult struct {
	SessionID string `json:"sessionId"`
}
