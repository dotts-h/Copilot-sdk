// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

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

func get(t *testing.T, srv *httptest.Server, path string) string {
	t.Helper()
	resp, err := http.Get(srv.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return string(body)
}

func TestSkillNewForm(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := get(t, srv, "/skills/new")
	for _, sub := range []string{`<form`, `hx-post="/skills"`, `name="id"`, `name="prompt"`, `name="name"`} {
		if !strings.Contains(body, sub) {
			t.Errorf("new skill form missing %q: %s", sub, body)
		}
	}
}

func TestSkillEditFormPrefilled(t *testing.T) {
	s, _ := newTestServer()
	s.forge.Skills = []ctxforge.Skill{{ID: "tdd", Name: "TDD", Prompt: "write tests", Enabled: true}}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := get(t, srv, "/skills/tdd/edit")
	if !strings.Contains(body, `hx-post="/skills/tdd"`) {
		t.Errorf("edit form should post to /skills/tdd: %s", body)
	}
	if !strings.Contains(body, "write tests") || !strings.Contains(body, `value="TDD"`) {
		t.Errorf("edit form not prefilled: %s", body)
	}
}

func TestSkillCreate(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/skills", url.Values{
		"id": {"review"}, "name": {"Review"}, "prompt": {"review carefully"}, "enabled": {"on"},
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if len(s.forge.Skills) != 1 || s.forge.Skills[0].ID != "review" {
		t.Fatalf("skill not created: %+v", s.forge.Skills)
	}
	if !s.forge.Skills[0].Enabled {
		t.Errorf("enabled checkbox not honored")
	}
	// Response is the refreshed list partial.
	if !strings.Contains(string(body), "Review") || !strings.Contains(string(body), "Skills") {
		t.Errorf("create did not return skills list: %s", body)
	}
}

func TestSkillCreateInvalidReshowsForm(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	// Missing prompt → validation error.
	resp, err := http.PostForm(srv.URL+"/skills", url.Values{"id": {"bad"}, "name": {"Bad"}})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if len(s.forge.Skills) != 0 {
		t.Fatalf("invalid skill should not be added: %+v", s.forge.Skills)
	}
	if !strings.Contains(string(body), `<form`) || !strings.Contains(string(body), "error") {
		t.Errorf("invalid create should reshow form with error: %s", body)
	}
}

func TestSkillUpdate(t *testing.T) {
	s, _ := newTestServer()
	s.forge.Skills = []ctxforge.Skill{{ID: "tdd", Name: "TDD", Prompt: "old", Enabled: true}}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/skills/tdd", url.Values{
		"id": {"tdd"}, "name": {"TDD v2"}, "prompt": {"new prompt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if s.forge.Skills[0].Name != "TDD v2" || s.forge.Skills[0].Prompt != "new prompt" {
		t.Fatalf("skill not updated: %+v", s.forge.Skills[0])
	}
}

func TestInstructionCreate(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/instructions", url.Values{
		"id": {"tests-first"}, "title": {"Tests first"}, "body": {"write tests"}, "priority": {"5"},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(s.forge.Instructions) != 1 || s.forge.Instructions[0].Priority != 5 {
		t.Fatalf("instruction not created: %+v", s.forge.Instructions)
	}
}

func TestAgentCreate(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/agents", url.Values{
		"id": {"builder"}, "name": {"Builder"}, "model": {"gpt-5"}, "reasoningEffort": {"high"},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(s.forge.Agents) != 1 || s.forge.Agents[0].Model != "gpt-5" {
		t.Fatalf("agent not created: %+v", s.forge.Agents)
	}
}

func TestAgentCreateUnknownSkillReshowsForm(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/agents", url.Values{
		"id": {"builder"}, "name": {"Builder"}, "model": {"gpt-5"}, "reasoningEffort": {"high"},
		"skills": {"ghost"},
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if len(s.forge.Agents) != 0 {
		t.Fatalf("agent with bad skill ref should not be added: %+v", s.forge.Agents)
	}
	if !strings.Contains(string(body), "error") {
		t.Errorf("expected error reshown: %s", body)
	}
}
