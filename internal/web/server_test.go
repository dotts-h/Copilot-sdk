package web

import (
	"bufio"
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/config"
	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// newTestServer builds a Hub wired with a mock client and minimal deps, and
// returns its single default session (the lone-session fallback routes
// cookieless test requests to it).
func newTestServer() (*Server, *copilot.MockClient) {
	mock := copilot.NewMockClient()
	// A writable config dir so config-mutating handlers' Save() succeeds (the
	// in-memory config never drifts from disk). An empty-dir config would make
	// every Save() fail, which previously masked the now-fixed select/delete
	// drift-on-save-failure. The temp dir is OS-reclaimed.
	dir, _ := os.MkdirTemp("", "mo-web-test-")
	c := config.Default(dir)
	c.DefaultModel = "gpt-5"
	hub := New(Options{
		Client: mock,
		Forge:  &ctxforge.Forge{},
		Config: c,
		Meter:  telemetry.NewMeter(telemetry.DefaultPriceBook()),
		Logger: log.New(io.Discard, "", 0),
	})
	return hub.newSession("test"), mock
}

func TestWriteSSE(t *testing.T) {
	var b strings.Builder
	writeSSE(&b, "delta", "<span>hi</span>")
	if got := b.String(); got != "event: delta\ndata: <span>hi</span>\n\n" {
		t.Errorf("writeSSE = %q", got)
	}
	b.Reset()
	writeSSE(&b, "status", "")
	if got := b.String(); got != "event: status\ndata: \n\n" {
		t.Errorf("empty-data SSE = %q", got)
	}
	b.Reset()
	writeSSE(&b, "x", "a\nb")
	if got := b.String(); got != "event: x\ndata: a\ndata: b\n\n" {
		t.Errorf("multiline SSE = %q", got)
	}
}

func TestIndexServes(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	for _, sub := range []string{"my-orchestra", `sse-connect="/events"`, "/static/htmx.min.js",
		`id="cur"`, `id="cost-footer"`, `hx-get="/page/telemetry"`} {
		if !strings.Contains(string(body), sub) {
			t.Errorf("index missing %q", sub)
		}
	}
}

func TestStaticAssetsServed(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/static/htmx.min.js")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("htmx status = %d", resp.StatusCode)
	}
}

func TestSendRecordsAndRefreshesTimeline(t *testing.T) {
	s, mock := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/send", url.Values{"prompt": {"hello there"}})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `hx-swap-oob="innerHTML"`) || !strings.Contains(string(body), "hello there") {
		t.Errorf("send did not return OOB timeline with the prompt: %q", body)
	}
	if len(mock.Sent) != 1 || mock.Sent[0] != "hello there" {
		t.Errorf("prompt not forwarded: %v", mock.Sent)
	}
}

func TestSendEmptyPromptNoContent(t *testing.T) {
	s, mock := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/send", url.Values{"prompt": {"   "}})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("empty prompt status = %d, want 204", resp.StatusCode)
	}
	if len(mock.Sent) != 0 {
		t.Errorf("empty prompt should not be sent: %v", mock.Sent)
	}
}

func TestPermResolves(t *testing.T) {
	s, mock := newTestServer()
	// Seed a pending permission.
	s.handleEvent(copilot.Event{Type: copilot.EvPermission,
		Permission: &copilot.PermissionRequest{ID: "p1", Detail: "run shell"}})
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/perm/p1", url.Values{"approve": {"1"}})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `hx-swap-oob="innerHTML"`) {
		t.Errorf("perm response missing OOB timeline: %q", body)
	}
	if len(mock.Responded) != 1 || mock.Responded[0].ID != "p1" || !mock.Responded[0].Approve {
		t.Errorf("permission not resolved: %v", mock.Responded)
	}
	if len(s.perms) != 0 {
		t.Errorf("perm queue not drained: %v", s.perms)
	}
}

func TestPermissionWithDiffEmitsReviewLane(t *testing.T) {
	s, _ := newTestServer()
	frags := s.handleEvent(copilot.Event{Type: copilot.EvPermission,
		Permission: &copilot.PermissionRequest{
			ID: "w1", Kind: "write", Detail: "write file: x.go", FileName: "x.go",
			Intention: "tweak", Diff: "@@ -1 +1 @@\n-a\n+b\n",
		}})
	var perm string
	for _, f := range frags {
		if f.Event == "perm" {
			perm = f.HTML
		}
	}
	if perm == "" {
		t.Fatal("no perm fragment emitted")
	}
	for _, sub := range []string{"perm-review", "x.go", "diff-add", "diff-del", `hx-post="/perm/w1"`} {
		if !strings.Contains(perm, sub) {
			t.Errorf("review-lane perm fragment missing %q: %q", sub, perm)
		}
	}
	// The pending request is retained so a full chat re-render rebuilds the lane.
	if len(s.perms) != 1 || s.perms[0].Diff == "" {
		t.Errorf("write permission not retained with diff: %+v", s.perms)
	}
}

func TestPageRoutes(t *testing.T) {
	s, _ := newTestServer()
	s.forge.Skills = []ctxforge.Skill{{ID: "tdd", Name: "TDD", Enabled: true}}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	for _, tc := range []struct{ slug, want string }{
		{"telemetry", "Token Telemetry"},
		{"skills", "TDD"},
		{"settings", "Settings"},
	} {
		resp, err := http.Get(srv.URL + "/page/" + tc.slug)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if !strings.Contains(string(body), tc.want) {
			t.Errorf("page %s missing %q: %q", tc.slug, tc.want, body)
		}
	}
}

func TestSkillToggle(t *testing.T) {
	s, _ := newTestServer()
	s.forge.Skills = []ctxforge.Skill{{ID: "tdd", Name: "TDD", Prompt: "x", Enabled: false}}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/skills/tdd/toggle", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if !s.forge.Skills[0].Enabled {
		t.Errorf("skill not toggled on")
	}
}

func TestAgentSelectToggles(t *testing.T) {
	s, _ := newTestServer()
	s.forge.Agents = []ctxforge.Agent{{ID: "builder", Name: "Builder", Model: "gpt-5"}}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	_, _ = http.PostForm(srv.URL+"/agents/builder/select", nil)
	if s.config.DefaultAgent != "builder" {
		t.Errorf("agent not selected: %q", s.config.DefaultAgent)
	}
	_, _ = http.PostForm(srv.URL+"/agents/builder/select", nil)
	if s.config.DefaultAgent != "" {
		t.Errorf("agent not deselected: %q", s.config.DefaultAgent)
	}
}

// TestEventsStreamsFragments drives the mock event stream and asserts the SSE
// endpoint renders the message contract end-to-end (the walking skeleton).
func TestEventsStreamsFragments(t *testing.T) {
	s, mock := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type = %q", ct)
	}

	go func() {
		mock.Emit(copilot.Event{Type: copilot.EvReasoningDelta, Text: "think "})
		mock.Emit(copilot.Event{Type: copilot.EvMessageDelta, Text: "hi "})   // kind switch → timeline
		mock.Emit(copilot.Event{Type: copilot.EvMessageDelta, Text: "there"}) // same kind → delta
		mock.Emit(copilot.Event{Type: copilot.EvMessage, Text: "hi there"})
		mock.Emit(copilot.Event{Type: copilot.EvUsage, Usage: copilot.UsageData{Model: "gpt-5", OutputTokens: 10}})
		mock.Emit(copilot.Event{Type: copilot.EvIdle})
	}()

	want := map[string]bool{"timeline": false, "delta": false, "cost": false, "status": false}
	remaining := len(want)
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		name, ok := strings.CutPrefix(sc.Text(), "event: ")
		if !ok {
			continue
		}
		if seen, tracked := want[name]; tracked && !seen {
			want[name] = true
			remaining--
			if remaining == 0 {
				return
			}
		}
	}
	t.Fatalf("did not observe all SSE fragments: %v (scanner err: %v)", want, sc.Err())
}

func TestHandleEventReducer(t *testing.T) {
	s, _ := newTestServer()

	// First message delta with no prior live kind → full timeline re-render.
	frags := s.handleEvent(copilot.Event{Type: copilot.EvMessageDelta, Text: "a"})
	if len(frags) != 1 || frags[0].Event != "timeline" {
		t.Fatalf("first delta should re-render timeline: %+v", frags)
	}
	// Subsequent same-kind delta → incremental append.
	frags = s.handleEvent(copilot.Event{Type: copilot.EvMessageDelta, Text: "b"})
	if len(frags) != 1 || frags[0].Event != "delta" || !strings.Contains(frags[0].HTML, "b") {
		t.Fatalf("second delta should append: %+v", frags)
	}
	// Switching to reasoning → re-render (kind changed).
	frags = s.handleEvent(copilot.Event{Type: copilot.EvReasoningDelta, Text: "r"})
	if frags[0].Event != "timeline" {
		t.Fatalf("kind switch should re-render: %+v", frags)
	}
}
