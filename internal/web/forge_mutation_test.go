package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
)

// seedForge installs a skill pinned by an agent so deletion-ordering rules can
// be exercised through the HTTP handlers.
func seedForge(s *Server) {
	s.forge.Skills = []ctxforge.Skill{{ID: "tdd", Name: "TDD", Prompt: "test first", Enabled: true}}
	s.forge.Agents = []ctxforge.Agent{{ID: "b", Name: "B", Model: "gpt-5", ReasoningEffort: "high", Skills: []string{"tdd"}}}
}

func TestDeletePinnedSkillRejected(t *testing.T) {
	s, _ := newTestServer()
	seedForge(s)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/skills/tdd/delete", "application/x-www-form-urlencoded", nil)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	// The pinned skill must survive (the validated RemoveSkill rolled back) and
	// still appear in the re-rendered list.
	if s.forge.Skill("tdd") == nil {
		t.Fatal("pinned skill should not have been deleted")
	}
	if !strings.Contains(string(body), "TDD") {
		t.Errorf("skills list should still show the skill: %s", string(body))
	}
}

func TestToggleInstructionViaHandler(t *testing.T) {
	s, _ := newTestServer()
	s.forge.Instructions = []ctxforge.Instruction{{ID: "i1", Title: "T", Body: "b", Enabled: true}}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	_, err := http.Post(srv.URL+"/instructions/i1/toggle", "application/x-www-form-urlencoded", nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.forge.Instruction("i1").Enabled {
		t.Error("toggle should have disabled the instruction")
	}
}

func TestDeleteActiveAgentClearsDefault(t *testing.T) {
	s, _ := newTestServer()
	s.forge.Agents = []ctxforge.Agent{{ID: "a1", Name: "A", Model: "gpt-5", ReasoningEffort: "high"}}
	s.config.DefaultAgent = "a1"
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	_, err := http.PostForm(srv.URL+"/agents/a1/delete", url.Values{})
	if err != nil {
		t.Fatal(err)
	}
	if s.forge.Agent("a1") != nil {
		t.Error("agent should be deleted")
	}
	if s.config.DefaultAgent != "" {
		t.Errorf("deleting the active agent should clear DefaultAgent, got %q", s.config.DefaultAgent)
	}
}
