package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dotts-h/copilot-sdk/internal/config"
	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

func testModel(t *testing.T) (Model, *copilot.MockClient) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default(dir)
	forge := ctxforge.New(dir)
	_ = forge.AddSkill(ctxforge.Skill{ID: "tdd", Name: "TDD", Prompt: "test first", Enabled: false})
	_ = forge.AddSkill(ctxforge.Skill{ID: "lint", Name: "Lint", Prompt: "clean", Enabled: true})
	forge.Agents = []ctxforge.Agent{{ID: "builder", Name: "Builder", Model: "gpt-5", ReasoningEffort: "high"}}
	mock := copilot.NewMockClient()
	m := New(Deps{Config: cfg, Forge: forge, Client: mock, Meter: telemetry.NewMeter(nil)})
	// Give it a size so viewport renders.
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return mm.(Model), mock
}

func key(s string) tea.KeyMsg {
	switch s {
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case " ":
		return tea.KeyMsg{Type: tea.KeySpace}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestTabNavigationCycles(t *testing.T) {
	m, _ := testModel(t)
	if m.page != PageChat {
		t.Fatalf("should start on Chat, got %v", m.page)
	}
	mm, _ := m.Update(key("tab"))
	m = mm.(Model)
	if m.page != PageTelemetry {
		t.Fatalf("tab should advance to Telemetry, got %v", m.page)
	}
	mm, _ = m.Update(key("shift+tab"))
	m = mm.(Model)
	if m.page != PageChat {
		t.Fatalf("shift+tab should return to Chat, got %v", m.page)
	}
}

func TestSendPromptAddsUserTurnAndCallsClient(t *testing.T) {
	m, mock := testModel(t)
	// Mark session ready.
	mm, _ := m.Update(sessionReadyMsg{sessionID: "sess-x"})
	m = mm.(Model)
	// Type a prompt into the textarea.
	m.input.SetValue("build me a thing")
	mm, cmd := m.Update(key("enter"))
	m = mm.(Model)

	if got := m.chat.turns; len(got) == 0 || got[len(got)-1].Role != RoleUser {
		t.Fatalf("user turn not appended: %+v", got)
	}
	if m.input.Value() != "" {
		t.Fatalf("input should reset after send, got %q", m.input.Value())
	}
	// Execute the returned command (the send) and verify the mock saw it.
	if cmd == nil {
		t.Fatal("expected a send command")
	}
	cmd()
	if len(mock.Sent) != 1 || mock.Sent[0] != "build me a thing" {
		t.Fatalf("client.Send not invoked correctly: %v", mock.Sent)
	}
}

func TestSendBlockedWhenNotReady(t *testing.T) {
	m, _ := testModel(t)
	m.input.SetValue("hi")
	mm, cmd := m.Update(key("enter"))
	m = mm.(Model)
	if len(m.chat.turns) != 0 {
		t.Fatal("should not send before session is ready")
	}
	if cmd != nil {
		t.Fatal("no command expected when not ready")
	}
}

func TestUsageEventRecordsCredits(t *testing.T) {
	m, _ := testModel(t)
	mm, _ := m.Update(usageMsg{usage: copilot.UsageData{Model: "gpt-5", InputTokens: 1_000_000, OutputTokens: 1_000_000}})
	m = mm.(Model)
	// gpt-5: $11.25 -> 1125 credits.
	if got := m.deps.Meter.Totals().Credits(); got < 1124 || got > 1126 {
		t.Fatalf("credits not recorded from usage event: %v", got)
	}
}

func TestSessionReadyBeforeResizeStillRenders(t *testing.T) {
	// Regression: sessionReadyMsg must not prevent the first WindowSizeMsg from
	// constructing the viewport.
	dir := t.TempDir()
	m := New(Deps{
		Config: config.Default(dir),
		Forge:  ctxforge.New(dir),
		Client: copilot.NewMockClient(),
		Meter:  telemetry.NewMeter(nil),
	})
	mm, _ := m.Update(sessionReadyMsg{sessionID: "s1"}) // arrives first
	mm, _ = mm.(Model).Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	mm, _ = mm.(Model).Update(streamDeltaMsg{text: "hello-stream"})
	m = mm.(Model)
	if !m.sized {
		t.Fatal("viewport should be marked sized after WindowSizeMsg")
	}
	if !strings.Contains(m.View(), "hello-stream") {
		t.Fatalf("streamed text not rendered after ready-before-resize:\n%s", m.View())
	}
}

func TestUsageDoesNotDoubleCountReasoning(t *testing.T) {
	m, _ := testModel(t)
	// gpt-5 output rate $10/Mt: 1M output => $10 => 1000 credits. Reasoning
	// tokens must NOT be added on top.
	mm, _ := m.Update(usageMsg{usage: copilot.UsageData{
		Model: "gpt-5", OutputTokens: 1_000_000, ReasoningTokens: 500_000,
	}})
	m = mm.(Model)
	if got := m.deps.Meter.Totals().Credits(); got < 999 || got > 1001 {
		t.Fatalf("reasoning tokens double-counted: credits=%v (want ~1000)", got)
	}
}

func TestStreamingDeltaAccumulates(t *testing.T) {
	m, _ := testModel(t)
	for _, chunk := range []string{"Hel", "lo ", "world"} {
		mm, _ := m.Update(streamDeltaMsg{text: chunk})
		m = mm.(Model)
	}
	mm, _ := m.Update(assistantDoneMsg{})
	m = mm.(Model)
	last := m.chat.turns[len(m.chat.turns)-1]
	if last.Role != RoleAgent || last.Text != "Hello world" {
		t.Fatalf("streamed assistant turn wrong: %+v", last)
	}
}

func TestToggleSkillPersists(t *testing.T) {
	m, _ := testModel(t)
	// Navigate to Skills page (Chat=0 -> Skills=2): tab twice.
	mm, _ := m.Update(key("tab"))
	mm, _ = mm.(Model).Update(key("tab"))
	m = mm.(Model)
	if m.page != PageSkills {
		t.Fatalf("expected Skills page, got %v", m.page)
	}
	// Cursor at 0 (tdd, disabled). Toggle it on.
	before := m.deps.Forge.Skills[0].Enabled
	mm, _ = m.Update(key(" "))
	m = mm.(Model)
	if m.deps.Forge.Skills[0].Enabled == before {
		t.Fatal("space should toggle the selected skill")
	}
	// Reload from disk to confirm it persisted.
	reloaded, err := ctxforge.Load(m.deps.Forge.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Skills[0].Enabled == before {
		t.Fatal("toggle was not persisted to disk")
	}
}

func TestSetDefaultAgent(t *testing.T) {
	m, _ := testModel(t)
	// Chat(0) -> Agents(4): tab 4 times.
	for i := 0; i < 4; i++ {
		mm, _ := m.Update(key("tab"))
		m = mm.(Model)
	}
	if m.page != PageAgents {
		t.Fatalf("expected Agents page, got %v", m.page)
	}
	mm, _ := m.Update(key("enter"))
	m = mm.(Model)
	if m.deps.Config.DefaultAgent != "builder" {
		t.Fatalf("enter should set default agent, got %q", m.deps.Config.DefaultAgent)
	}
}

func TestViewRendersWithoutPanicAllPages(t *testing.T) {
	m, _ := testModel(t)
	for p := Page(0); p < numPages; p++ {
		m.page = p
		out := m.View()
		if strings.TrimSpace(out) == "" {
			t.Fatalf("page %v rendered empty", p)
		}
	}
}

func TestBuildSpecUsesAgentOverride(t *testing.T) {
	m, _ := testModel(t)
	m.deps.Config.DefaultAgent = "builder"
	spec := m.buildSpec()
	if spec.Model != "gpt-5" || spec.ReasoningEffort != "high" {
		t.Fatalf("agent override not applied to spec: %+v", spec)
	}
}

func TestErrMsgSurfaces(t *testing.T) {
	m, _ := testModel(t)
	mm, _ := m.Update(errMsg{err: errors.New("boom")})
	m = mm.(Model)
	if m.errText != "boom" {
		t.Fatalf("error not surfaced: %q", m.errText)
	}
}
