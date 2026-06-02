package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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

func gotoPage(t *testing.T, m Model, target Page) Model {
	t.Helper()
	for i := 0; i < int(numPages); i++ {
		if m.page == target {
			return m
		}
		mm, _ := m.Update(key("tab"))
		m = mm.(Model)
	}
	t.Fatalf("could not reach page %v", target)
	return m
}

func TestAddSkillViaForm(t *testing.T) {
	m, _ := testModel(t)
	m = gotoPage(t, m, PageSkills)
	before := len(m.deps.Forge.Skills)

	mm, _ := m.Update(key("a")) // open add form
	m = mm.(Model)
	if m.form == nil {
		t.Fatal("expected a form to open")
	}
	// Fill required fields: id, name, (skip desc), prompt.
	setField(&m, "id", "newskill")
	setField(&m, "name", "New Skill")
	setField(&m, "prompt", "do the thing")

	mm, _ = m.Update(key("enter")) // submit
	m = mm.(Model)
	if m.form != nil {
		t.Fatalf("form should close on success; errText=%q", m.errText)
	}
	if len(m.deps.Forge.Skills) != before+1 {
		t.Fatalf("skill not added: have %d want %d", len(m.deps.Forge.Skills), before+1)
	}
	reloaded, _ := ctxforge.Load(m.deps.Forge.Dir)
	if reloaded.Skill("newskill") == nil {
		t.Fatal("new skill not persisted")
	}
}

func TestAddSkillValidationKeepsFormOpen(t *testing.T) {
	m, _ := testModel(t)
	m = gotoPage(t, m, PageSkills)
	mm, _ := m.Update(key("a"))
	m = mm.(Model)
	setField(&m, "id", "okid") // missing required name/prompt
	mm, _ = m.Update(key("enter"))
	m = mm.(Model)
	if m.form == nil {
		t.Fatal("form should stay open on validation error")
	}
	if m.errText == "" {
		t.Fatal("expected an error message")
	}
}

func TestEditSkillViaForm(t *testing.T) {
	m, _ := testModel(t)
	m = gotoPage(t, m, PageSkills)
	// cursor at 0 == "tdd"
	mm, _ := m.Update(key("e"))
	m = mm.(Model)
	if m.form == nil || m.form.editIndex != 0 {
		t.Fatalf("expected edit form for index 0, got %+v", m.form)
	}
	setField(&m, "name", "Renamed TDD")
	mm, _ = m.Update(key("enter"))
	m = mm.(Model)
	if m.deps.Forge.Skills[0].Name != "Renamed TDD" {
		t.Fatalf("edit not applied: %q", m.deps.Forge.Skills[0].Name)
	}
}

func TestDeleteSkill(t *testing.T) {
	m, _ := testModel(t)
	m = gotoPage(t, m, PageSkills)
	before := len(m.deps.Forge.Skills)
	mm, _ := m.Update(key("d"))
	m = mm.(Model)
	if len(m.deps.Forge.Skills) != before-1 {
		t.Fatalf("delete failed: have %d want %d", len(m.deps.Forge.Skills), before-1)
	}
}

func TestFormCancel(t *testing.T) {
	m, _ := testModel(t)
	m = gotoPage(t, m, PageSkills)
	before := len(m.deps.Forge.Skills)
	mm, _ := m.Update(key("a"))
	m = mm.(Model)
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = mm.(Model)
	if m.form != nil {
		t.Fatal("esc should close the form")
	}
	if len(m.deps.Forge.Skills) != before {
		t.Fatal("cancel should not add a skill")
	}
}

func setField(m *Model, key, val string) {
	for i := range m.form.fields {
		if m.form.fields[i].key == key {
			m.form.fields[i].input.SetValue(val)
			return
		}
	}
}

func TestSlashClear(t *testing.T) {
	m, _ := testModel(t)
	mm, _ := m.Update(sessionReadyMsg{sessionID: "s1"})
	m = mm.(Model)
	m.chat.addUser("something")
	m.input.SetValue("/clear")
	mm, _ = m.Update(key("enter"))
	m = mm.(Model)
	if len(m.chat.turns) != 0 {
		t.Fatalf("/clear should empty the transcript, have %d turns", len(m.chat.turns))
	}
	if m.input.Value() != "" {
		t.Fatal("/clear should reset the input")
	}
}

func TestSlashNavigatesAndDoesNotSend(t *testing.T) {
	m, mock := testModel(t)
	mm, _ := m.Update(sessionReadyMsg{sessionID: "s1"})
	m = mm.(Model)
	m.input.SetValue("/cost")
	mm, cmd := m.Update(key("enter"))
	m = mm.(Model)
	if m.page != PageTelemetry {
		t.Fatalf("/cost should open Telemetry, got %v", m.page)
	}
	if cmd != nil {
		cmd()
	}
	if len(mock.Sent) != 0 {
		t.Fatalf("slash command must not be sent to the agent: %v", mock.Sent)
	}
}

func TestSlashModelRecreatesSession(t *testing.T) {
	m, _ := testModel(t)
	mm, _ := m.Update(sessionReadyMsg{sessionID: "s1"})
	m = mm.(Model)
	m.input.SetValue("/model claude-opus-4.7")
	mm, cmd := m.Update(key("enter"))
	m = mm.(Model)
	if m.deps.Config.DefaultModel != "claude-opus-4.7" {
		t.Fatalf("/model did not update config: %q", m.deps.Config.DefaultModel)
	}
	if cmd == nil {
		t.Fatal("/model should trigger a new session command")
	}
	msg := cmd()
	if _, ok := msg.(sessionReadyMsg); !ok {
		t.Fatalf("expected sessionReadyMsg from recreated session, got %T", msg)
	}
}

func TestSlashUnknown(t *testing.T) {
	m, _ := testModel(t)
	m.input.SetValue("/bogus")
	mm, _ := m.Update(key("enter"))
	m = mm.(Model)
	last := m.chat.turns[len(m.chat.turns)-1]
	if last.Role != RoleSystem || !strings.Contains(last.Text, "unknown command") {
		t.Fatalf("unknown command not reported: %+v", last)
	}
}

func TestFormRendersFields(t *testing.T) {
	m, _ := testModel(t)
	m = gotoPage(t, m, PageAgents)
	mm, _ := m.Update(key("a"))
	m = mm.(Model)
	out := m.View()
	for _, label := range []string{"New agent", "Model", "Reasoning effort", "Pinned skills"} {
		if !strings.Contains(out, label) {
			t.Fatalf("form view missing %q:\n%s", label, out)
		}
	}
}

func TestSlashAgentSwitches(t *testing.T) {
	m, _ := testModel(t)
	mm, _ := m.Update(sessionReadyMsg{sessionID: "s1"})
	m = mm.(Model)
	m.input.SetValue("/agent builder")
	mm, cmd := m.Update(key("enter"))
	m = mm.(Model)
	if m.deps.Config.DefaultAgent != "builder" {
		t.Fatalf("/agent did not set agent: %q", m.deps.Config.DefaultAgent)
	}
	if cmd == nil {
		t.Fatal("/agent should recreate the session")
	}
	// Unknown agent is rejected without changing config.
	m.input.SetValue("/agent ghost")
	mm, _ = m.Update(key("enter"))
	m = mm.(Model)
	if m.deps.Config.DefaultAgent != "builder" {
		t.Fatal("/agent with unknown id should not change the agent")
	}
}

func TestPermissionApproveFlow(t *testing.T) {
	m, mock := testModel(t)
	mm, _ := m.Update(sessionReadyMsg{sessionID: "s1"})
	m = mm.(Model)
	// A permission request arrives.
	mm, _ = m.Update(permMsg{req: copilot.PermissionRequest{ID: "perm-1", Kind: "shell", Detail: "run shell: ls"}})
	m = mm.(Model)
	if len(m.permQueue) != 1 {
		t.Fatalf("permission not queued: %v", m.permQueue)
	}
	if !strings.Contains(m.View(), "[y]es") {
		t.Fatalf("permission prompt not rendered:\n%s", m.View())
	}
	// Approve with 'y'.
	mm, cmd := m.Update(key("y"))
	m = mm.(Model)
	if len(m.permQueue) != 0 {
		t.Fatal("queue should be empty after deciding")
	}
	if cmd == nil {
		t.Fatal("expected a Respond command")
	}
	cmd()
	if len(mock.Responded) != 1 || mock.Responded[0].ID != "perm-1" || !mock.Responded[0].Approve {
		t.Fatalf("Respond not invoked correctly: %+v", mock.Responded)
	}
}

func TestPermissionRejectAndQueue(t *testing.T) {
	m, mock := testModel(t)
	mm, _ := m.Update(sessionReadyMsg{sessionID: "s1"})
	m = mm.(Model)
	mm, _ = m.Update(permMsg{req: copilot.PermissionRequest{ID: "p1", Detail: "write file: a"}})
	mm, _ = mm.(Model).Update(permMsg{req: copilot.PermissionRequest{ID: "p2", Detail: "write file: b"}})
	m = mm.(Model)
	if len(m.permQueue) != 2 {
		t.Fatalf("expected 2 queued, got %d", len(m.permQueue))
	}
	// Reject the head; the second should remain.
	mm, cmd := m.Update(key("n"))
	m = mm.(Model)
	if len(m.permQueue) != 1 || m.permQueue[0].ID != "p2" {
		t.Fatalf("queue not advanced correctly: %+v", m.permQueue)
	}
	cmd()
	if len(mock.Responded) != 1 || mock.Responded[0].Approve {
		t.Fatalf("expected one reject for p1: %+v", mock.Responded)
	}
}

func TestPermissionPendingBlocksTyping(t *testing.T) {
	// While a permission is pending, 'y' decides rather than typing into input.
	m, _ := testModel(t)
	mm, _ := m.Update(sessionReadyMsg{sessionID: "s1"})
	m = mm.(Model)
	mm, _ = m.Update(permMsg{req: copilot.PermissionRequest{ID: "p1", Detail: "x"}})
	m = mm.(Model)
	mm, _ = m.Update(key("y"))
	m = mm.(Model)
	if m.input.Value() != "" {
		t.Fatalf("'y' should decide, not type; input=%q", m.input.Value())
	}
}

func TestSlashAttachAndSend(t *testing.T) {
	m, mock := testModel(t)
	mm, _ := m.Update(sessionReadyMsg{sessionID: "s1"})
	m = mm.(Model)

	f := filepath.Join(t.TempDir(), "pic.png")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.input.SetValue("/attach " + f)
	mm, _ = m.Update(key("enter"))
	m = mm.(Model)
	if len(m.pendingAttachments) != 1 || m.pendingAttachments[0] != f {
		t.Fatalf("attachment not pending: %v", m.pendingAttachments)
	}

	m.input.SetValue("describe this")
	mm, cmd := m.Update(key("enter"))
	m = mm.(Model)
	if len(m.pendingAttachments) != 0 {
		t.Fatal("attachments should clear after send")
	}
	cmd()
	if len(mock.LastAttach) != 1 || mock.LastAttach[0] != f {
		t.Fatalf("attachment not sent: %v", mock.LastAttach)
	}
}

func TestSlashAttachRejectsMissingFile(t *testing.T) {
	m, _ := testModel(t)
	mm, _ := m.Update(sessionReadyMsg{sessionID: "s1"})
	m = mm.(Model)
	m.input.SetValue("/attach /no/such/file.png")
	mm, _ = m.Update(key("enter"))
	m = mm.(Model)
	if len(m.pendingAttachments) != 0 {
		t.Fatal("nonexistent file should not be attached")
	}
}

func TestSlashResume(t *testing.T) {
	m, mock := testModel(t)
	m.input.SetValue("/resume sess-42")
	mm, cmd := m.Update(key("enter"))
	m = mm.(Model)
	if cmd == nil {
		t.Fatal("/resume should return a command")
	}
	msg := cmd()
	rr, ok := msg.(sessionReadyMsg)
	if !ok || rr.sessionID != "sess-42" {
		t.Fatalf("resume did not yield the session: %#v", msg)
	}
	if len(mock.Resumed) != 1 || mock.Resumed[0] != "sess-42" {
		t.Fatalf("ResumeSession not called: %v", mock.Resumed)
	}
}

func TestResumeLastSessionHelper(t *testing.T) {
	mock := copilot.NewMockClient()
	if _, err := mock.CreateSession(context.Background(), copilot.SessionSpec{}); err != nil {
		t.Fatal(err)
	}
	msg := resumeSession(mock, "")()
	if rr, ok := msg.(sessionReadyMsg); !ok || rr.sessionID != "mock-session" {
		t.Fatalf("resume of last session wrong: %#v", msg)
	}
}

func TestResumeNoPreviousSessionErrors(t *testing.T) {
	mock := copilot.NewMockClient() // no session created
	msg := resumeSession(mock, "")()
	if _, ok := msg.(errMsg); !ok {
		t.Fatalf("expected errMsg when no session to resume, got %#v", msg)
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
