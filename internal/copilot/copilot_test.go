package copilot

import (
	"context"
	"os"
	"os/exec"
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
	if err := m.Send(context.Background(), "mock-session", "hello", []string{"/tmp/a.png"}); err != nil {
		t.Fatal(err)
	}
	if len(m.Sent) != 1 || m.Sent[0] != "hello" {
		t.Fatalf("Send not recorded: %v", m.Sent)
	}
	if len(m.LastAttach) != 1 || m.LastAttach[0] != "/tmp/a.png" {
		t.Fatalf("attachments not recorded: %v", m.LastAttach)
	}
	if id, _ := m.LastSessionID(context.Background()); id != "mock-session" {
		t.Fatalf("LastSessionID = %q", id)
	}
	if rid, err := m.ResumeSession(context.Background(), "sess-9"); err != nil || rid != "sess-9" {
		t.Fatalf("ResumeSession = %q, %v", rid, err)
	}
	if err := m.Abort(context.Background(), "mock-session"); err != nil {
		t.Fatal(err)
	}
	if len(m.Aborted) != 1 {
		t.Fatalf("Abort not recorded: %v", m.Aborted)
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
		sessions:  map[string]*sdk.Session{},
		unsubs:    map[string]func(){},
		toolNames: map[string]string{},
		events:    make(chan Event, 64),
		done:      make(chan struct{}),
	}
}

func TestHandlerTranslatesEvents(t *testing.T) {
	c := newTestSDKClient()
	h := c.makeHandler()

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

// TestSDKIntegration exercises the real SDK path end to end. It is skipped
// unless the `copilot` CLI is installed and a GitHub token is present, keeping
// unit CI hermetic.
func TestSDKIntegration(t *testing.T) {
	if _, err := exec.LookPath("copilot"); err != nil {
		t.Skip("copilot CLI not installed; skipping SDK integration test")
	}
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		t.Skip("GITHUB_TOKEN not set; skipping SDK integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := NewSDKClient(ctx, Options{GitHubToken: token})
	if err != nil {
		t.Fatalf("new sdk client: %v", err)
	}
	defer client.Close()

	id, err := client.CreateSession(ctx, SessionSpec{Model: "gpt-5", Streaming: true})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := client.Send(ctx, id, "Reply with the single word: pong", nil); err != nil {
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
