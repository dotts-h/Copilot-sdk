package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
)

func TestBuiltinChatAgentListedWhenForgeEmpty(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := get(t, srv, "/page/agents")
	if !strings.Contains(body, "Chat") || !strings.Contains(body, `hx-post="/agents/chat/select"`) {
		t.Errorf("built-in chat agent not listed/selectable:\n%s", body)
	}
	// The built-in is virtual: no edit/delete controls for it.
	if strings.Contains(body, `hx-get="/agents/chat/edit"`) || strings.Contains(body, `hx-post="/agents/chat/delete"`) {
		t.Errorf("built-in chat agent must not be editable/deletable:\n%s", body)
	}
}

func TestAgentAllowedToolsAppliedOnSelect(t *testing.T) {
	s, _ := newTestServer()
	s.forge.Agents = []ctxforge.Agent{
		{ID: "builder", Name: "Builder", Model: "gpt-5", ReasoningEffort: "high", AllowedTools: []string{"bash", "read"}},
	}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/agents/builder/select", url.Values{})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	got := strings.Join(s.spec.AllowedTools, ",")
	if got != "bash,read" {
		t.Errorf("agent AllowedTools not applied to spec: %q", got)
	}
	if s.spec.Model != "gpt-5" {
		t.Errorf("agent model not applied: %q", s.spec.Model)
	}
}

func TestAgentSystemMessageCompiledOnSelect(t *testing.T) {
	s, _ := newTestServer()
	s.forge.Instructions = []ctxforge.Instruction{
		{ID: "house", Title: "House rules", Body: "Be terse.", Enabled: true, Priority: 0},
	}
	s.forge.Skills = []ctxforge.Skill{
		{ID: "tdd", Name: "TDD", Prompt: "Write a failing test first.", Enabled: false},
	}
	s.forge.Agents = []ctxforge.Agent{
		{ID: "builder", Name: "Builder", Model: "gpt-5", ReasoningEffort: "high",
			SystemMessage: "You are the Builder.", Skills: []string{"tdd"}},
	}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/agents/builder/select", url.Values{})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// Activation must compile the agent's full persona into the session, not just
	// model/effort/tools: the agent system message, enabled global instructions,
	// and the prompts of the agent's pinned skills.
	for _, want := range []string{"You are the Builder.", "Be terse.", "Write a failing test first."} {
		if !strings.Contains(s.spec.SystemMessage, want) {
			t.Errorf("compiled system message missing %q:\n%s", want, s.spec.SystemMessage)
		}
	}
}

func TestAgentClearCompilesGlobalContext(t *testing.T) {
	s, _ := newTestServer()
	s.forge.Instructions = []ctxforge.Instruction{
		{ID: "house", Title: "House rules", Body: "Be terse.", Enabled: true, Priority: 0},
	}
	s.forge.Agents = []ctxforge.Agent{
		{ID: "builder", Name: "Builder", Model: "gpt-5", SystemMessage: "You are the Builder."},
	}
	s.config.DefaultAgent = "builder"
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	// Toggle the active agent off; the agent persona must drop out but global
	// instructions must still be compiled into the session.
	resp, err := http.PostForm(srv.URL+"/agents/builder/select", url.Values{})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if strings.Contains(s.spec.SystemMessage, "You are the Builder.") {
		t.Errorf("cleared agent persona still present:\n%s", s.spec.SystemMessage)
	}
	if !strings.Contains(s.spec.SystemMessage, "Be terse.") {
		t.Errorf("global instruction dropped on clear:\n%s", s.spec.SystemMessage)
	}
}

// TestAgentSelectRollsBackOnSaveFailure guards that a persistence failure on
// agent-select does not leave the live config pointing at an agent that was
// never written to disk. handleAgentSelect routes through editConfig, which
// snapshots → mutates → rolls back if the validating Save fails — the same
// discipline every other config mutation uses (REGRESSIONS: "edit through
// Server.editConfig"). Before the fix it mutated s.config.DefaultAgent directly
// and only logged a Save error, leaving the in-memory config drifted from disk.
func TestAgentSelectRollsBackOnSaveFailure(t *testing.T) {
	s, dir := newSettingsServer(t)
	s.forge.Agents = []ctxforge.Agent{{ID: "builder", Name: "Builder", Model: "gpt-5"}}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	// Sabotage persistence: replace the config dir with a regular file so any
	// Save() fails at MkdirAll ("not a directory") — a stand-in for a disk error
	// on the rare write path.
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	resp, err := http.PostForm(srv.URL+"/agents/builder/select", url.Values{})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// The Save failed, so the selection must NOT stick in memory: the live config
	// must match the (unwritten) disk state, not the attempted selection.
	if s.config.DefaultAgent != "" {
		t.Errorf("config drifted on save failure: DefaultAgent=%q, want empty (rolled back)", s.config.DefaultAgent)
	}
}

func TestAgentFormHasAllowedToolsField(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	body := get(t, srv, "/agents/new")
	if !strings.Contains(body, `name="allowedTools"`) {
		t.Errorf("agent form missing allowedTools field:\n%s", body)
	}
}

func TestAgentFormRoundTripsAllowedTools(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	resp, err := http.PostForm(srv.URL+"/agents", url.Values{
		"id": {"scoped"}, "name": {"Scoped"}, "model": {"gpt-5"},
		"reasoningEffort": {"medium"}, "allowedTools": {"bash, read , write"},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	a := s.forge.Agent("scoped")
	if a == nil || strings.Join(a.AllowedTools, ",") != "bash,read,write" {
		t.Errorf("allowedTools not parsed from form: %+v", a)
	}
}

func TestImportInstructionFiles(t *testing.T) {
	dir := t.TempDir()
	must := func(name, body string) {
		p := filepath.Join(dir, name)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	must("CLAUDE.md", "# Project rules\nbe concise")
	must(".github/copilot-instructions.md", "use go idioms")
	// AGENTS.md intentionally absent.

	got := importInstructionFiles(dir)
	if len(got) != 2 {
		t.Fatalf("expected 2 imported instructions, got %d: %+v", len(got), got)
	}
	byID := map[string]ctxforge.Instruction{}
	for _, in := range got {
		byID[in.ID] = in
	}
	if !strings.Contains(byID["import-claude-md"].Body, "be concise") {
		t.Errorf("CLAUDE.md body not imported: %+v", byID["import-claude-md"])
	}
	if !strings.Contains(byID["import-copilot-instructions"].Body, "go idioms") {
		t.Errorf("copilot-instructions body not imported: %+v", byID)
	}
	for _, in := range got {
		if !in.Enabled {
			t.Errorf("imported instruction should be enabled: %+v", in)
		}
	}
}

func TestInstructionImportHandlerAddsToForge(t *testing.T) {
	s, _ := newTestServer()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("follow the conventions"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.hub.workdir = dir
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/instructions/import", url.Values{})
	if err != nil {
		t.Fatal(err)
	}
	body := readBody(t, resp)
	if in := s.forge.Instruction("import-agents-md"); in == nil || !strings.Contains(in.Body, "conventions") {
		t.Errorf("import did not add the AGENTS.md instruction: %+v", in)
	}
	if !strings.Contains(body, "AGENTS.md") && !strings.Contains(body, "import-agents-md") {
		t.Logf("response (informational): %s", body)
	}
}
