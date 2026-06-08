package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/dotts-h/copilot-sdk/internal/convo"
	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
)

func TestHookNewForm(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := get(t, srv, "/hooks/new")
	for _, sub := range []string{
		`<form`, `hx-post="/hooks"`, `name="id"`, `name="event"`, `name="toolKind"`,
		`name="pattern"`, `name="outsideWorkspace"`, `name="action"`, `name="reason"`,
		`name="modes"`, `name="enabled"`,
		// The preflight tester posts to /hooks/preflight.
		`hx-post="/hooks/preflight"`, `name="sample"`,
	} {
		if !strings.Contains(body, sub) {
			t.Errorf("new hook form missing %q", sub)
		}
	}
}

func TestHookCreateRoundTrips(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/hooks", url.Values{
		"id": {"no-prod-writes"}, "event": {"pre-tool-use"}, "toolKind": {"write"},
		"pattern": {"*/prod/*"}, "action": {"deny"}, "reason": {"no writes under prod"},
		"modes": {"autopilot"}, "enabled": {"1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	h := s.forge.Hook("no-prod-writes")
	if h == nil {
		t.Fatalf("hook not created: %+v", s.forge.Hooks)
	}
	if h.Action != "deny" || h.Match.ToolKind != "write" || h.Match.Pattern != "*/prod/*" {
		t.Errorf("hook fields not persisted: %+v", h)
	}
	if len(h.Modes) != 1 || h.Modes[0] != "autopilot" {
		t.Errorf("mode binding not persisted: %+v", h.Modes)
	}
	if !strings.Contains(string(body), "Hooks") {
		t.Errorf("create did not return the hooks list: %s", body)
	}
}

// The "any" tool-kind sentinel persists as an empty ToolKind (match any kind).
func TestHookCreateAnyKind(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	_, err := http.PostForm(srv.URL+"/hooks", url.Values{
		"id": {"audit-all"}, "event": {"pre-tool-use"}, "toolKind": {"any"},
		"pattern": {"secret"}, "action": {"ask"}, "enabled": {"1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	h := s.forge.Hook("audit-all")
	if h == nil || h.Match.ToolKind != "" {
		t.Fatalf("'any' should persist as empty ToolKind: %+v", h)
	}
}

func TestHookCreateInvalidReshowsForm(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	// Invalid action → rollback-on-invalid, re-render the form with the error.
	resp, err := http.PostForm(srv.URL+"/hooks", url.Values{
		"id": {"bad"}, "event": {"pre-tool-use"}, "toolKind": {"read"}, "action": {"nuke"},
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if len(s.forge.Hooks) != 0 {
		t.Fatalf("invalid hook must not be added: %+v", s.forge.Hooks)
	}
	if !strings.Contains(string(body), `<form`) || !strings.Contains(string(body), "error") {
		t.Errorf("invalid create should reshow the form with an error: %s", body)
	}
}

// An id that collides with a literal /hooks route segment (e.g. "preflight")
// is rejected at create, so it can never become an un-editable hook.
func TestHookCreateRejectsReservedRouteID(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/hooks", url.Values{
		"id": {"preflight"}, "event": {"pre-tool-use"}, "toolKind": {"read"}, "action": {"allow"}, "enabled": {"1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if s.forge.Hook("preflight") != nil {
		t.Fatal("a hook with the reserved id 'preflight' must not be created")
	}
	if !strings.Contains(string(body), "reserved") {
		t.Errorf("create should reshow the form with a 'reserved' error: %s", body)
	}
}

func TestHookEditFormPrefilled(t *testing.T) {
	s, _ := newTestServer()
	s.forge.Hooks = []ctxforge.Hook{{
		ID: "h1", Event: "pre-tool-use", Match: ctxforge.HookMatch{ToolKind: "shell", Pattern: "rm -rf *"},
		Action: "deny", Reason: "blocked", Modes: []string{"autopilot"}, Enabled: true,
	}}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := get(t, srv, "/hooks/h1/edit")
	if !strings.Contains(body, `hx-post="/hooks/h1"`) {
		t.Errorf("edit form should post to /hooks/h1: %s", body)
	}
	if !strings.Contains(body, `value="rm -rf *"`) || !strings.Contains(body, `value="blocked"`) {
		t.Errorf("edit form not prefilled: %s", body)
	}
	// The autopilot mode checkbox is pre-checked.
	if !strings.Contains(body, `value="autopilot" checked`) {
		t.Errorf("mode binding not prefilled in the form: %s", body)
	}
}

func TestHookToggleAndDelete(t *testing.T) {
	s, _ := newTestServer()
	s.forge.Hooks = []ctxforge.Hook{{
		ID: "h1", Event: "pre-tool-use", Match: ctxforge.HookMatch{ToolKind: "read"}, Action: "allow", Enabled: false,
	}}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	if _, err := http.Post(srv.URL+"/hooks/h1/toggle", "application/x-www-form-urlencoded", nil); err != nil {
		t.Fatal(err)
	}
	if !s.forge.Hook("h1").Enabled {
		t.Error("toggle should have enabled the hook")
	}
	if _, err := http.PostForm(srv.URL+"/hooks/h1/delete", url.Values{}); err != nil {
		t.Fatal(err)
	}
	if s.forge.Hook("h1") != nil {
		t.Error("hook should be deleted")
	}
}

// The page lists the built-in policy read-only: a built-in row shows the
// "built-in" badge and has NO edit/toggle/delete controls, while a user hook does.
func TestHooksPageListsBuiltinsReadOnly(t *testing.T) {
	s, _ := newTestServer()
	s.forge.Hooks = []ctxforge.Hook{{
		ID: "mine", Event: "pre-tool-use", Match: ctxforge.HookMatch{ToolKind: "write"}, Action: "ask", Enabled: true,
	}}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := get(t, srv, "/page/hooks")
	if !strings.Contains(body, "read-only tool auto-approved by default") {
		t.Errorf("built-in safe-read hook not listed: %s", body)
	}
	if !strings.Contains(body, ">built-in</span>") {
		t.Errorf("built-in badge missing: %s", body)
	}
	// A mandatory dangerous rule is badged unbypassable.
	if !strings.Contains(body, ">unbypassable</span>") {
		t.Errorf("mandatory built-in should be badged unbypassable: %s", body)
	}
	// No CRUD control targets a built-in id.
	if strings.Contains(body, `/hooks/builtin-allow-read/edit`) ||
		strings.Contains(body, `/hooks/builtin-allow-read/delete`) ||
		strings.Contains(body, `/hooks/builtin-allow-read/toggle`) {
		t.Errorf("built-in hook must be read-only (no CRUD controls): %s", body)
	}
	// The user hook does have CRUD controls.
	if !strings.Contains(body, `/hooks/mine/edit`) {
		t.Errorf("user hook should have an edit control: %s", body)
	}
}

// The preflight reports the pattern form and whether a sample command matches —
// the pure matcher behind the form's tester.
func TestHookPreflightMatcher(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	// A glob pattern that matches the sample.
	resp, err := http.PostForm(srv.URL+"/hooks/preflight", url.Values{
		"pattern": {"rm -rf *"}, "sample": {"sudo rm -rf /tmp"},
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "glob") || !strings.Contains(string(body), "✓ matches") {
		t.Errorf("preflight should report a matching glob: %s", body)
	}

	// A substring pattern that does NOT match.
	resp2, _ := http.PostForm(srv.URL+"/hooks/preflight", url.Values{
		"pattern": {"curl"}, "sample": {"echo hi"},
	})
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if !strings.Contains(string(body2), "substring") || !strings.Contains(string(body2), "✗ no match") {
		t.Errorf("preflight should report a non-matching substring: %s", body2)
	}
}

// Timeline "why" (ADR-0031): an EvToolDecision reduces into an inline annotation
// on the rendered timeline — a deny shows "denied" + the reason, an allow shows
// "auto-approved by <hook>".
func TestToolDecisionRendersInTimeline(t *testing.T) {
	s, _ := newTestServer()

	s.handleEvent(copilot.Event{Type: copilot.EvToolDecision, Decision: &copilot.ToolDecision{
		Kind: "deny", HookID: "builtin-deny-rm-root", Reason: "blocked: recursive force-delete", Detail: "run: rm -rf /",
	}})
	s.handleEvent(copilot.Event{Type: copilot.EvToolDecision, Decision: &copilot.ToolDecision{
		Kind: "allow", HookID: "my-allow", Reason: "trusted", Detail: "read foo.go",
	}})

	body := s.chatPartial()
	if !strings.Contains(body, "denied") || !strings.Contains(body, "blocked: recursive force-delete") {
		t.Errorf("deny annotation missing from timeline: %s", body)
	}
	if !strings.Contains(body, "auto-approved") || !strings.Contains(body, "my-allow") {
		t.Errorf("allow annotation missing from timeline: %s", body)
	}
}

// The decision render helper maps kind→glyph/label without a server.
func TestRenderDecisionNote(t *testing.T) {
	deny := renderDecisionNote(&convo.DecisionView{Kind: "deny", Reason: "nope"})
	if !strings.Contains(deny, "⛔") || !strings.Contains(deny, "denied") {
		t.Errorf("deny note = %s", deny)
	}
	allow := renderDecisionNote(&convo.DecisionView{Kind: "allow", HookID: "x"})
	if !strings.Contains(allow, "✓") || !strings.Contains(allow, "auto-approved") {
		t.Errorf("allow note = %s", allow)
	}
	if renderDecisionNote(nil) != "" {
		t.Error("nil decision should render empty")
	}
}
