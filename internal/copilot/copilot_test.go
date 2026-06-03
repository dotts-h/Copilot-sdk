package copilot

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	sdk "github.com/github/copilot-sdk/go"
	"github.com/github/copilot-sdk/go/rpc"
)

func i64(v int64) *int64 { return &v }

func TestNormalizeUsageMapsEveryTokenCategory(t *testing.T) {
	d := &sdk.AssistantUsageData{
		Model:           "gpt-5",
		InputTokens:     i64(1000),
		OutputTokens:    i64(250),
		CacheReadTokens: i64(400),
		ReasoningTokens: i64(80),
		CopilotUsage:    &rpc.AssistantUsageCopilotUsage{TotalNanoAiu: 1_500_000_000}, // 1.5 AIU
	}
	u := normalizeUsage(d)
	if u.Model != "gpt-5" {
		t.Fatalf("model = %q", u.Model)
	}
	if u.InputTokens != 1000 || u.OutputTokens != 250 || u.CachedTokens != 400 || u.ReasoningTokens != 80 {
		t.Fatalf("token mapping wrong: %+v", u)
	}
	if u.NanoAIU != 1_500_000_000 {
		t.Fatalf("nanoAIU not carried: %v", u.NanoAIU)
	}
}

func TestNormalizeUsageHandlesNilPointers(t *testing.T) {
	// All optional fields nil — must not panic and must yield zeros.
	u := normalizeUsage(&sdk.AssistantUsageData{Model: "x"})
	if u.InputTokens != 0 || u.OutputTokens != 0 || u.CachedTokens != 0 || u.NanoAIU != 0 {
		t.Fatalf("nil pointers should map to zero: %+v", u)
	}
}

func TestMockClient(t *testing.T) {
	m := NewMockClient()
	defer m.Close()
	if _, err := m.CreateSession(context.Background(), SessionSpec{}); err != nil {
		t.Fatal(err)
	}
	if err := m.Send(context.Background(), "mock-session", "hello", []string{"/tmp/a.png"}, ""); err != nil {
		t.Fatal(err)
	}
	if len(m.Sent) != 1 || m.Sent[0] != "hello" {
		t.Fatalf("Send not recorded: %v", m.Sent)
	}
	if len(m.LastAttach) != 1 || m.LastAttach[0] != "/tmp/a.png" {
		t.Fatalf("attachments not recorded: %v", m.LastAttach)
	}
	if err := m.Abort(context.Background(), "mock-session"); err != nil {
		t.Fatal(err)
	}
	if len(m.Aborted) != 1 {
		t.Fatalf("Abort not recorded: %v", m.Aborted)
	}
	m.Models = []ModelInfo{{ID: "gpt-5", Name: "GPT-5"}}
	if got, err := m.ListModels(context.Background()); err != nil || len(got) != 1 || got[0].ID != "gpt-5" {
		t.Fatalf("ListModels = %v, %v", got, err)
	}
	m.Emit(Event{Type: EvMessage, Text: "hi"})
	ev := <-m.Events()
	if ev.Type != EvMessage || ev.Text != "hi" {
		t.Fatalf("unexpected event %+v", ev)
	}
}

// newTestSDKClient builds an SDKClient with its channels wired but no live
// runtime, so the pure event-translation logic can be tested without the CLI.
func newTestSDKClient() *SDKClient {
	return &SDKClient{
		perms:     newPermBridge(),
		inputs:    newInputBridge(),
		plans:     newPlanBridge(),
		elicits:   newElicitBridge(),
		sessions:  map[string]*sdk.Session{},
		unsubs:    map[string]func(){},
		toolNames: map[string]string{},
		events:    make(chan Event, 64),
		done:      make(chan struct{}),
	}
}

func TestHandlerTranslatesEvents(t *testing.T) {
	c := newTestSDKClient()
	h := c.makeHandler("")

	h(sdk.SessionEvent{Data: &sdk.AssistantMessageDeltaData{DeltaContent: "Hel"}})
	h(sdk.SessionEvent{Data: &sdk.AssistantReasoningDeltaData{DeltaContent: "(thinking)"}})
	h(sdk.SessionEvent{Data: &sdk.AssistantMessageData{Content: "Hello"}})
	h(sdk.SessionEvent{Data: &sdk.AssistantUsageData{Model: "gpt-5", InputTokens: i64(10), OutputTokens: i64(5)}})
	h(sdk.SessionEvent{Data: &sdk.ToolExecutionStartData{ToolCallID: "t1", ToolName: "bash"}})
	h(sdk.SessionEvent{Data: &sdk.ToolExecutionCompleteData{ToolCallID: "t1"}})
	h(sdk.SessionEvent{Data: &sdk.SessionIdleData{}})

	want := []struct {
		typ  EventType
		text string
		tool string
	}{
		{EvMessageDelta, "Hel", ""},
		{EvReasoningDelta, "(thinking)", ""},
		{EvMessage, "Hello", ""},
		{EvUsage, "", ""},
		{EvToolStart, "", "bash"},
		{EvToolEnd, "", "bash"}, // name recovered via toolCallID mapping
		{EvIdle, "", ""},
	}
	for i, w := range want {
		select {
		case ev := <-c.events:
			if ev.Type != w.typ || ev.Text != w.text || ev.Tool != w.tool {
				t.Fatalf("event %d = %+v, want type=%v text=%q tool=%q", i, ev, w.typ, w.text, w.tool)
			}
			if w.typ == EvUsage && ev.Usage.InputTokens != 10 {
				t.Fatalf("usage event lost token data: %+v", ev.Usage)
			}
		default:
			t.Fatalf("expected event %d (%v) but channel was empty", i, w.typ)
		}
	}
}

func TestHandlerCarriesToolDetail(t *testing.T) {
	c := newTestSDKClient()
	h := c.makeHandler("")

	mcp := "filesystem"
	h(sdk.SessionEvent{Data: &sdk.ToolExecutionStartData{
		ToolCallID: "t1", ToolName: "bash",
		Arguments:     map[string]any{"command": "go test ./...", "cwd": "/repo"},
		MCPServerName: &mcp,
	}})
	h(sdk.SessionEvent{Data: &sdk.ToolExecutionProgressData{ToolCallID: "t1", ProgressMessage: "compiling"}})
	detailed := "PASS\nok  pkg 0.1s"
	h(sdk.SessionEvent{Data: &sdk.ToolExecutionCompleteData{
		ToolCallID: "t1", Success: true,
		Result: &sdk.ToolExecutionCompleteResult{Content: "ok", DetailedContent: &detailed},
	}})

	start := <-c.events
	if start.Type != EvToolStart || start.ToolCall == nil {
		t.Fatalf("start: %+v", start)
	}
	if start.ToolCall.Args != "go test ./..." || start.ToolCall.MCPServer != "filesystem" {
		t.Fatalf("start detail wrong: %+v", start.ToolCall)
	}
	prog := <-c.events
	if prog.Type != EvToolProgress || prog.ToolCall == nil || prog.ToolCall.Progress != "compiling" {
		t.Fatalf("progress: %+v", prog)
	}
	end := <-c.events
	if end.Type != EvToolEnd || end.ToolCall == nil || !end.ToolCall.Success {
		t.Fatalf("end: %+v", end)
	}
	if end.ToolCall.Result != detailed { // detailed content preferred over Content
		t.Fatalf("end result = %q, want detailed content", end.ToolCall.Result)
	}
}

func TestHandlerEmitsFullReasoning(t *testing.T) {
	c := newTestSDKClient()
	c.makeHandler("")(sdk.SessionEvent{Data: &sdk.AssistantReasoningData{Content: "step by step"}})
	ev := <-c.events
	if ev.Type != EvReasoning || ev.Text != "step by step" {
		t.Fatalf("reasoning event = %+v", ev)
	}
}

func TestHandlerNormalizesContextAndCompaction(t *testing.T) {
	c := newTestSDKClient()
	h := c.makeHandler("")

	h(sdk.SessionEvent{Data: &sdk.SessionUsageInfoData{CurrentTokens: 1234, TokenLimit: 128000}})
	h(sdk.SessionEvent{Data: &sdk.SessionCompactionStartData{}})
	removed := int64(5000)
	h(sdk.SessionEvent{Data: &sdk.SessionCompactionCompleteData{Success: true, TokensRemoved: &removed}})
	h(sdk.SessionEvent{Data: &sdk.SessionCompactionCompleteData{Success: false, Error: strptr("oom")}})

	ctx := <-c.events
	if ctx.Type != EvContextWindow || ctx.Context.CurrentTokens != 1234 || ctx.Context.TokenLimit != 128000 {
		t.Fatalf("context-window event = %+v", ctx)
	}
	if start := <-c.events; start.Type != EvCompactionStart {
		t.Fatalf("want EvCompactionStart, got %+v", start)
	}
	ok := <-c.events
	if ok.Type != EvCompactionEnd || !contains(ok.Text, "5") {
		t.Fatalf("compaction-complete (success) event = %+v", ok)
	}
	fail := <-c.events
	if fail.Type != EvCompactionEnd || !contains(fail.Text, "oom") {
		t.Fatalf("compaction-complete (failure) event = %+v", fail)
	}
}

func strptr(s string) *string { return &s }

func contains(s, sub string) bool { return strings.Contains(s, sub) }

func TestSummarizeArgs(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"shell command", map[string]any{"command": "ls -la", "cwd": "/x"}, "ls -la"},
		{"file path", map[string]any{"path": "internal/main.go"}, "internal/main.go"},
		{"newlines collapsed", map[string]any{"command": "a\n  b\tc"}, "a b c"},
		{"fallback json", map[string]any{"foo": "bar"}, `{"foo":"bar"}`},
	}
	for _, tc := range cases {
		if got := summarizeArgs(tc.in); got != tc.want {
			t.Errorf("%s: summarizeArgs = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestToolResultTextPrefersDetailAndSurfacesError(t *testing.T) {
	detailed := "full diff"
	ok := &sdk.ToolExecutionCompleteData{Success: true, Result: &sdk.ToolExecutionCompleteResult{
		Content: "short", DetailedContent: &detailed,
	}}
	if got := toolResultText(ok); got != "full diff" {
		t.Fatalf("expected detailed content, got %q", got)
	}
	fail := &sdk.ToolExecutionCompleteData{Success: false, Error: &sdk.ToolExecutionCompleteError{Message: "boom"}}
	if got := toolResultText(fail); got != "boom" {
		t.Fatalf("expected error message, got %q", got)
	}
}

func TestEmitDoesNotBlockAfterClose(t *testing.T) {
	c := newTestSDKClient()
	close(c.done) // simulate shutdown
	// Must return promptly rather than blocking on a full/!consumed channel.
	done := make(chan struct{})
	go func() {
		c.emit(Event{Type: EvMessage, Text: "x"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("emit blocked after done was closed")
	}
}

func TestResolveAuthPrefersLoggedInUser(t *testing.T) {
	// No explicit token: use the logged-in copilot CLI session.
	token, loggedIn := ResolveAuth("")
	if token != "" {
		t.Fatalf("expected empty token, got %q", token)
	}
	if loggedIn == nil || !*loggedIn {
		t.Fatalf("expected UseLoggedInUser=true, got %v", loggedIn)
	}

	// Explicit token: override and let the SDK disable the logged-in session.
	token, loggedIn = ResolveAuth("ghp_explicit")
	if token != "ghp_explicit" {
		t.Fatalf("expected explicit token passthrough, got %q", token)
	}
	if loggedIn != nil {
		t.Fatalf("expected UseLoggedInUser=nil for explicit token, got %v", *loggedIn)
	}
}

func TestShouldDropReasoningEffort(t *testing.T) {
	cases := []struct {
		name      string
		effort    string
		supported []string
		known     bool
		drop      bool
	}{
		{"no effort requested", "", []string{"low", "high"}, true, false},
		{"capabilities unknown", "medium", nil, false, false},
		{"model supports none", "medium", []string{}, true, true},
		{"effort supported", "medium", []string{"low", "medium", "high"}, true, false},
		{"effort unsupported", "xhigh", []string{"low", "medium", "high"}, true, true},
	}
	for _, tc := range cases {
		if got := shouldDropReasoningEffort(tc.effort, tc.supported, tc.known); got != tc.drop {
			t.Errorf("%s: shouldDropReasoningEffort(%q, %v, %v)=%v, want %v",
				tc.name, tc.effort, tc.supported, tc.known, got, tc.drop)
		}
	}
}

// TestSDKIntegration exercises the real SDK path end to end. It is skipped unless
// the `copilot` CLI is installed and either a GITHUB_TOKEN is present or
// COPILOT_SDK_E2E is set (to authenticate via the logged-in CLI session),
// keeping unit CI hermetic.
func TestSDKIntegration(t *testing.T) {
	if _, err := exec.LookPath("copilot"); err != nil {
		t.Skip("copilot CLI not installed; skipping SDK integration test")
	}
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" && os.Getenv("COPILOT_SDK_E2E") == "" {
		t.Skip("no GITHUB_TOKEN and COPILOT_SDK_E2E unset; skipping SDK integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	githubToken, useLoggedInUser := ResolveAuth(token)
	client, err := NewSDKClient(ctx, Options{GitHubToken: githubToken, UseLoggedInUser: useLoggedInUser})
	if err != nil {
		t.Fatalf("new sdk client: %v", err)
	}
	defer client.Close()

	// "auto" lets the runtime pick an available model for this account, so the
	// test does not depend on a specific model being entitled. Pairing it with a
	// reasoning effort also exercises the guard: "auto" supports no effort, so
	// CreateSession must drop it rather than fail.
	model := os.Getenv("COPILOT_SDK_E2E_MODEL")
	if model == "" {
		model = "auto"
	}
	id, err := client.CreateSession(ctx, SessionSpec{Model: model, ReasoningEffort: "medium", Streaming: true})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := client.Send(ctx, id, "Reply with the single word: pong", nil, ""); err != nil {
		t.Fatalf("send: %v", err)
	}

	var text string
	var sawIdle bool
	deadline := time.After(60 * time.Second)
	for !sawIdle {
		select {
		case ev := <-client.Events():
			switch ev.Type {
			case EvMessageDelta, EvMessage:
				text += ev.Text
			case EvIdle:
				sawIdle = true
			}
		case <-deadline:
			t.Fatalf("timed out; text=%q", text)
		}
	}
	if text == "" {
		t.Fatal("no assistant text received")
	}
}

func TestHandlerMapsSessionError(t *testing.T) {
	c := newTestSDKClient()
	h := c.makeHandler("")
	h(sdk.SessionEvent{Data: &sdk.SessionErrorData{ErrorType: "rate_limit", Message: "slow down"}})
	ev := <-c.events
	if ev.Type != EvError || ev.Err == nil {
		t.Fatalf("expected EvError with Err, got %+v", ev)
	}
	if ev.Err.Error() != "rate_limit: slow down" {
		t.Fatalf("error text = %q, want category-prefixed message", ev.Err.Error())
	}
}

func TestSessionErrorFallback(t *testing.T) {
	if got := sessionError(&sdk.SessionErrorData{}).Error(); got != "session error" {
		t.Fatalf("empty session error = %q", got)
	}
}
