package copilot

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
	sdk "github.com/github/copilot-sdk/go"
)

// toolMeta is the per-tool-call context captured at execution start so a
// PostToolUse command hook can be matched when the call completes (ADR-0032). The
// SDK's completion event carries no kind/argument detail, so they are remembered
// here keyed by ToolCallID. kind is a best-effort permission kind derived from the
// tool name (see toolKindFromName); target is the tool name plus its one-line
// argument summary, the string a PostToolUse Pattern matches against (so a hook
// can target a tool by name or by a path/command substring).
type toolMeta struct {
	kind   string
	target string
}

// postToolMeta derives the PostToolUse match context for a starting tool call.
func postToolMeta(d *sdk.ToolExecutionStartData, argsSummary string) toolMeta {
	return toolMeta{
		kind:   toolKindFromName(d.ToolName, d.MCPServerName != nil),
		target: strings.TrimSpace(d.ToolName + " " + argsSummary),
	}
}

// toolKindFromName maps a completed tool's name onto a hook ToolKind so a
// PostToolUse hook scoped to read/write/shell/mcp matches (ADR-0032). Unlike the
// PRE-tool path — which gets an authoritative `req.Kind()` from the SDK — the
// completion event reports only a tool NAME, so this is a best-effort map over the
// known built-in Copilot tools (an MCP tool is always "mcp"; an unrecognized name
// yields "" so the hook falls back to Pattern matching on the target). A post-tool
// command can never gate, so an occasional mis-classification only runs or skips a
// side-effect command — it is never a control-surface decision.
func toolKindFromName(name string, isMCP bool) string {
	if isMCP {
		return "mcp"
	}
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "bash", "shell", "run", "exec", "terminal", "command":
		return "shell"
	case "edit", "create", "write", "write_file", "str_replace", "str_replace_editor",
		"apply_patch", "patch", "insert", "multiedit", "notebook_edit":
		return "write"
	case "view", "read", "read_file", "open", "cat", "grep", "search", "glob",
		"ls", "list", "find", "web_fetch", "fetch", "web_search":
		return "read"
	}
	return ""
}

// This file is the pure SDK→normalized-Event translation layer: the makeHandler
// event switch, the history-event mapping, and the side-effect-free helpers that
// summarize/normalize SDK payloads into the compact Event the UI consumes. It is
// the half of the seam that "knows the SDK exists" but holds no client state
// beyond the per-segment reasoning-dedup map on the receiver — which is why every
// SDK-event → Event mapping is independently testable. See ARCHITECTURE.md
// "Event normalization".

// makeHandler returns a session event handler that normalizes and forwards.
// Every emitted event is stamped with sid so a multi-session consumer can route
// it to the originating conversation.
func (c *SDKClient) makeHandler(sid string) func(sdk.SessionEvent) {
	return func(ev sdk.SessionEvent) {
		// Stamp every event this turn with the session id and the envelope's
		// sub-agent tag (AgentID), so a reducer can route root vs sub-agent
		// activity (epic 0069 S1). derefStr yields "" for the root/main agent and
		// session-level events — additive: empty for every pre-existing path.
		agentID := derefStr(ev.AgentID)
		emit := func(e Event) {
			e.SessionID = sid
			e.AgentID = agentID
			c.emit(e)
		}
		switch d := ev.Data.(type) {
		case *sdk.AssistantMessageDeltaData:
			emit(Event{Type: EvMessageDelta, Text: d.DeltaContent})
		case *sdk.AssistantReasoningDeltaData:
			c.mu.Lock()
			c.reasoned[sid] = true
			c.mu.Unlock()
			emit(Event{Type: EvReasoningDelta, Text: d.DeltaContent})
		case *sdk.AssistantReasoningData:
			// The runtime emits a full reasoning block at the end of a segment that
			// already streamed as deltas — emitting it too would render the thinking
			// twice (once live, once committed below the answer). Only forward it when
			// no deltas streamed this segment (non-streaming reasoning). Reset either
			// way so the next segment is judged on its own.
			c.mu.Lock()
			streamed := c.reasoned[sid]
			delete(c.reasoned, sid)
			c.mu.Unlock()
			if !streamed {
				emit(Event{Type: EvReasoning, Text: d.Content})
			}
		case *sdk.AssistantMessageData:
			emit(Event{Type: EvMessage, Text: d.Content})
		case *sdk.AssistantUsageData:
			emit(Event{Type: EvUsage, Usage: normalizeUsage(d)})
		case *sdk.ToolExecutionStartData:
			argsSummary := summarizeArgs(d.Arguments)
			c.mu.Lock()
			c.toolNames[d.ToolCallID] = d.ToolName
			c.toolMeta[d.ToolCallID] = postToolMeta(d, argsSummary)
			c.mu.Unlock()
			tc := &ToolCall{ID: d.ToolCallID, Name: d.ToolName, Args: argsSummary}
			if d.MCPServerName != nil {
				tc.MCPServer = *d.MCPServerName
			}
			emit(Event{Type: EvToolStart, Tool: d.ToolName, ToolCall: tc})
		case *sdk.ToolExecutionProgressData:
			emit(Event{Type: EvToolProgress, ToolCall: &ToolCall{
				ID: d.ToolCallID, Progress: d.ProgressMessage,
			}})
		case *sdk.ToolExecutionCompleteData:
			c.mu.Lock()
			name := c.toolNames[d.ToolCallID]
			delete(c.toolNames, d.ToolCallID)
			meta := c.toolMeta[d.ToolCallID]
			delete(c.toolMeta, d.ToolCallID)
			pol := c.policies[sid]
			c.mu.Unlock()
			emit(Event{Type: EvToolEnd, Tool: name, ToolCall: &ToolCall{
				ID: d.ToolCallID, Name: name, Success: d.Success, Result: toolResultText(d),
			}})
			// Fire any matching PostToolUse command hooks (G5, ADR-0032). They run
			// off this event-handler goroutine so a command's latency never blocks
			// event delivery; each emits an EvHookRun annotation when it finishes.
			if cmds := ctxforge.PostToolUseCommands(pol.hooks, meta.kind, meta.target, pol.workspace, pol.mode); len(cmds) > 0 {
				go c.firePostToolHooks(sid, cmds, pol.workspace)
			}
		case *sdk.SessionUsageInfoData:
			emit(Event{Type: EvContextWindow, Context: ContextInfo{
				CurrentTokens: d.CurrentTokens, TokenLimit: d.TokenLimit,
			}})
		case *sdk.SessionCompactionStartData:
			emit(Event{Type: EvCompactionStart})
		case *sdk.SessionCompactionCompleteData:
			emit(Event{Type: EvCompactionEnd, Text: compactionSummary(d)})
		case *sdk.SessionPlanChangedData:
			emit(Event{Type: EvPlanChanged, Text: planChangeText(d.Operation)})
		case *sdk.SubagentStartedData:
			emit(Event{Type: EvSubagentStart, Subagent: &SubagentInfo{
				ToolCallID: d.ToolCallID, Name: d.AgentName, DisplayName: d.AgentDisplayName,
				Description: d.AgentDescription, Model: derefStr(d.Model),
			}})
		case *sdk.SubagentCompletedData:
			emit(Event{Type: EvSubagentEnd, Subagent: &SubagentInfo{
				ToolCallID: d.ToolCallID, Name: d.AgentName, DisplayName: d.AgentDisplayName,
				Model: derefStr(d.Model), Success: true, Detail: subagentSummary(d.DurationMs, d.TotalTokens),
			}})
		case *sdk.SubagentFailedData:
			emit(Event{Type: EvSubagentEnd, Subagent: &SubagentInfo{
				ToolCallID: d.ToolCallID, Name: d.AgentName, DisplayName: d.AgentDisplayName,
				Model: derefStr(d.Model), Success: false, Detail: d.Error,
			}})
		case *sdk.SessionErrorData:
			emit(Event{Type: EvError, Err: sessionError(d)})
		case *sdk.SessionIdleData:
			// A turn ended: clear any lingering reasoning-streamed flag so an
			// interrupted segment (deltas with no trailing full block) can't suppress
			// a later non-streaming reasoning block.
			c.mu.Lock()
			delete(c.reasoned, sid)
			c.mu.Unlock()
			emit(Event{Type: EvIdle})
		}
	}
}

// historyEvents normalizes a session's persisted events into the committed
// transcript events used to rebuild the timeline. It is deliberately separate
// from makeHandler: history holds the committed forms (no streaming deltas) plus
// the user's own prompts, and needs none of the per-segment dedup state.
func historyEvents(sid string, raw []sdk.SessionEvent) []Event {
	names := map[string]string{} // toolCallID -> name, within this history
	out := make([]Event, 0, len(raw))
	add := func(e Event) { e.SessionID = sid; out = append(out, e) }
	for _, ev := range raw {
		switch d := ev.Data.(type) {
		case *sdk.UserMessageData:
			add(Event{Type: EvUserMessage, Text: d.Content})
		case *sdk.AssistantMessageData:
			add(Event{Type: EvMessage, Text: d.Content})
		case *sdk.AssistantReasoningData:
			add(Event{Type: EvReasoning, Text: d.Content})
		case *sdk.ToolExecutionStartData:
			names[d.ToolCallID] = d.ToolName
			tc := &ToolCall{ID: d.ToolCallID, Name: d.ToolName, Args: summarizeArgs(d.Arguments)}
			if d.MCPServerName != nil {
				tc.MCPServer = *d.MCPServerName
			}
			add(Event{Type: EvToolStart, Tool: d.ToolName, ToolCall: tc})
		case *sdk.ToolExecutionCompleteData:
			add(Event{Type: EvToolEnd, Tool: names[d.ToolCallID], ToolCall: &ToolCall{
				ID: d.ToolCallID, Name: names[d.ToolCallID], Success: d.Success, Result: toolResultText(d),
			}})
		}
	}
	return out
}

// normalizeUsage maps the SDK's assistant.usage payload onto UsageData,
// dereferencing optional pointers safely.
func normalizeUsage(d *sdk.AssistantUsageData) UsageData {
	u := UsageData{
		Model:            d.Model,
		InputTokens:      deref(d.InputTokens),
		CachedTokens:     deref(d.CacheReadTokens),
		CacheWriteTokens: deref(d.CacheWriteTokens),
		OutputTokens:     deref(d.OutputTokens),
		ReasoningTokens:  deref(d.ReasoningTokens),
	}
	if d.CopilotUsage != nil {
		u.NanoAIU = d.CopilotUsage.TotalNanoAiu
	}
	return u
}

// normalizeElicitFields flattens an elicitation JSON schema into an ordered slice
// of displayable fields. Schema properties arrive as a map (order undefined), so
// fields are sorted by name for deterministic rendering, with required fields
// marked from the schema's Required list.
func normalizeElicitFields(schema *sdk.ElicitationSchema) []ElicitField {
	if schema == nil || len(schema.Properties) == 0 {
		return nil
	}
	required := make(map[string]bool, len(schema.Required))
	for _, r := range schema.Required {
		required[r] = true
	}
	names := make([]string, 0, len(schema.Properties))
	for name := range schema.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	fields := make([]ElicitField, 0, len(names))
	for _, name := range names {
		f := normalizeElicitField(name, schema.Properties[name])
		f.Required = required[name]
		fields = append(fields, f)
	}
	return fields
}

// normalizeElicitField maps one JSON-schema property (a generic decoded object)
// onto an ElicitField. Unknown or malformed properties degrade to a plain string
// field so the form always renders something usable.
func normalizeElicitField(name string, raw any) ElicitField {
	f := ElicitField{Name: name, Label: name, Type: "string"}
	m, ok := raw.(map[string]any)
	if !ok {
		return f
	}
	if t := elicitStr(m, "type"); t != "" {
		f.Type = t
	}
	if title := elicitStr(m, "title"); title != "" {
		f.Label = title
	}
	f.Description = elicitStr(m, "description")
	f.Default = elicitDefault(m["default"])
	f.Enum = elicitStrSlice(m["enum"])
	f.EnumLabels = elicitStrSlice(m["enumNames"])
	return f
}

// elicitStr reads a string-valued schema key, or "" if absent/non-string.
func elicitStr(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// elicitStrSlice coerces a decoded JSON array into a slice of strings, skipping
// non-string entries. Returns nil for a missing or non-array value.
func elicitStrSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// elicitDefault renders a schema default value as the string to pre-fill a form
// control with. Booleans become "true"/"false"; numbers drop a trailing ".0".
func elicitDefault(v any) string {
	switch d := v.(type) {
	case nil:
		return ""
	case string:
		return d
	case bool:
		if d {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(d, 'f', -1, 64)
	default:
		return fmt.Sprint(d)
	}
}

// sessionError turns a runtime session.error event into a displayable error,
// prefixing the human-readable message with its category (e.g. "rate_limit:
// …") so the UI can surface it instead of silently dropping the turn.
func sessionError(d *sdk.SessionErrorData) error {
	msg := strings.TrimSpace(d.Message)
	if msg == "" {
		msg = "session error"
	}
	if d.ErrorType != "" {
		return fmt.Errorf("%s: %s", d.ErrorType, msg)
	}
	return errors.New(msg)
}

// planChangeText renders a one-line note for a plan-file change operation.
func planChangeText(op sdk.PlanChangedOperation) string {
	switch op {
	case sdk.PlanChangedOperationCreate:
		return "plan created"
	case sdk.PlanChangedOperationUpdate:
		return "plan updated"
	case sdk.PlanChangedOperationDelete:
		return "plan deleted"
	default:
		return "plan changed"
	}
}

// compactionSummary renders a one-line, human-readable result of a conversation
// compaction for display as a system note.
func compactionSummary(d *sdk.SessionCompactionCompleteData) string {
	if !d.Success {
		if d.Error != nil && *d.Error != "" {
			return "compaction failed: " + *d.Error
		}
		return "compaction failed"
	}
	if d.TokensRemoved != nil && *d.TokensRemoved > 0 {
		return fmt.Sprintf("compacted context (freed %d tokens)", *d.TokensRemoved)
	}
	return "compacted context"
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

// permWriteFields extracts the file name, intention, and unified diff for a
// file-write permission request; all empty for any other request kind. It keeps
// the write-specific payload (which feeds the diff review lane) out of the
// generic Detail summary.
func permWriteFields(req sdk.PermissionRequest) (file, intention, diff string) {
	if w, ok := req.(sdk.PermissionRequestWrite); ok {
		return w.FileName, w.Intention, w.Diff
	}
	return "", "", ""
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

// oneLine collapses all runs of whitespace (including tabs/newlines) into single
// spaces and trims the ends, for a one-line timeline summary.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
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

func deref(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// subagentSummary renders a one-line completion summary for a sub-agent from its
// optional duration and token totals (e.g. "1.2s · 3.4k tok"). Empty when
// neither is reported.
func subagentSummary(durationMs, totalTokens *int64) string {
	var parts []string
	if durationMs != nil && *durationMs > 0 {
		parts = append(parts, fmt.Sprintf("%.1fs", float64(*durationMs)/1000))
	}
	if totalTokens != nil && *totalTokens > 0 {
		parts = append(parts, humanTokenCount(*totalTokens)+" tok")
	}
	return strings.Join(parts, " · ")
}

// humanTokenCount renders a token count compactly (1.5k, 2.5M).
func humanTokenCount(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1000:
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	default:
		return fmt.Sprintf("%d", n)
	}
}
