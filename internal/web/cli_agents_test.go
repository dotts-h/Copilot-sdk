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
