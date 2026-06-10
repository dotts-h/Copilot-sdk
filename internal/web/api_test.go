package web

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
)

// This file is the consolidated HTTP API contract suite — the SDET layer that
// pins the router's externally-observable behaviour (status codes, content
// types, output escaping, cookie security, and per-session isolation) against
// the full mux returned by Handler(), as opposed to the per-handler unit tests
// in the sibling *_test.go files.

// --- content-type & status contract across every GET page ---

func TestAllPagesRenderHTML(t *testing.T) {
	s, _ := newTestServer()
	// Seed one of each forge entity so list pages render rows, not just shells.
	s.forge.Skills = []ctxforge.Skill{{ID: "tdd", Name: "TDD", Prompt: "x", Enabled: true}}
	s.forge.Instructions = []ctxforge.Instruction{{ID: "sec", Title: "Sec", Priority: 1, Body: "b", Enabled: true}}
	s.forge.Agents = []ctxforge.Agent{{ID: "builder", Name: "Builder", Model: "gpt-5"}}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	pages := []string{"chat", "telemetry", "skills", "instructions", "agents", "connection", "models", "settings", "help"}
	for _, p := range pages {
		t.Run(p, func(t *testing.T) {
			resp, err := http.Get(srv.URL + "/page/" + p)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}
			if ct := resp.Header.Get("Content-Type"); ct != "text/html; charset=utf-8" {
				t.Errorf("content-type = %q, want text/html; charset=utf-8", ct)
			}
			body, _ := io.ReadAll(resp.Body)
			if len(strings.TrimSpace(string(body))) == 0 {
				t.Error("empty page body")
			}
		})
	}
}

// An unknown page slug degrades to the chat page rather than 404-ing — the nav
// is a known-finite set, so an unknown slug is a client bug, not a dead end.
func TestUnknownPageSlugFallsBackToChat(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/page/does-not-exist")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `id="composer"`) {
		t.Errorf("unknown slug should render the chat composer: %q", body)
	}
}

// The shell catch-all (GET /) absorbs unmapped GET paths — including a GET to a
// POST-only route — and serves the app shell instead of 500-ing. This pins that
// the fallthrough is intentional and side-effect-free.
func TestGetOnPostRouteServesShellNotError(t *testing.T) {
	s, mock := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	for _, path := range []string{"/send", "/abort", "/unmapped/path"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s = %d, want 200 (shell catch-all)", path, resp.StatusCode)
		}
		if !strings.Contains(string(body), "my-orchestra") {
			t.Errorf("GET %s should serve the shell", path)
		}
	}
	if len(mock.Sent) != 0 || len(mock.Aborted) != 0 {
		t.Errorf("a GET fallthrough must not dispatch: sent=%v aborted=%v", mock.Sent, mock.Aborted)
	}
}

// --- output-encoding (XSS) contract ---

// Every model- or user-supplied string the UI echoes must be HTML-escaped. A
// prompt carrying an injection payload must never round-trip as live markup.
func TestUserPromptIsHTMLEscaped(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	const payload = `<script>alert('xss')</script>`
	resp, err := http.PostForm(srv.URL+"/send", url.Values{"prompt": {payload}})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(body), "<script>alert") {
		t.Errorf("raw <script> reflected unescaped: %q", body)
	}
	if !strings.Contains(string(body), "&lt;script&gt;") {
		t.Errorf("payload should appear escaped: %q", body)
	}
}

// A reflected slash-command argument is escaped the same way.
func TestSlashCommandEchoIsEscaped(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/send", url.Values{"prompt": {`/attach <img src=x onerror=alert(1)>`}})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(body), "<img src=x onerror") {
		t.Errorf("slash-command arg reflected unescaped: %q", body)
	}
}

// --- session cookie security contract ---

func TestSessionCookieIsHardened(t *testing.T) {
	hub, _ := newTestHub()
	srv := httptest.NewServer(hub.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var c *http.Cookie
	for _, ck := range resp.Cookies() {
		if ck.Name == cookieName {
			c = ck
		}
	}
	if c == nil {
		t.Fatal("no session cookie set")
	}
	if !c.HttpOnly {
		t.Error("session cookie must be HttpOnly (JS must not read it)")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v, want Lax", c.SameSite)
	}
	if c.Path != "/" {
		t.Errorf("cookie path = %q, want /", c.Path)
	}
}

// --- per-session isolation over HTTP (two browsers, two cookie jars) ---

func TestTwoBrowsersGetIsolatedTranscripts(t *testing.T) {
	hub, _ := newTestHub()
	// Pre-create two distinct sessions so the lone-session fallback does not
	// collapse two cookieless browsers onto one conversation.
	hub.newSession("alice")
	hub.newSession("bob")
	srv := httptest.NewServer(hub.Handler())
	defer srv.Close()

	post := func(sid, prompt string) string {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/send",
			strings.NewReader(url.Values{"prompt": {prompt}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: cookieName, Value: sid})
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}

	if got := post("alice", "alice-secret"); !strings.Contains(got, "alice-secret") {
		t.Fatalf("alice should see her own prompt: %q", got)
	}
	bobView := post("bob", "bob-secret")
	if strings.Contains(bobView, "alice-secret") {
		t.Errorf("bob's transcript leaked alice's prompt: %q", bobView)
	}
	if !strings.Contains(bobView, "bob-secret") {
		t.Errorf("bob should see his own prompt: %q", bobView)
	}
}

// A request whose cookie names an unknown session is not trusted blindly when
// other sessions exist: it must not be able to read another session's state.
func TestUnknownCookieDoesNotHijackAnotherSession(t *testing.T) {
	hub, _ := newTestHub()
	alice := hub.newSession("alice")
	alice.state.AddUser("alice-secret")
	hub.newSession("bob") // a second session so there is no lone-session fallback
	srv := httptest.NewServer(hub.Handler())
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/page/chat", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: "ghost-not-a-real-id"})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(body), "alice-secret") {
		t.Errorf("a forged cookie read alice's transcript: %q", body)
	}
}

// --- static assets ---

func TestStaticContentTypesAndMissing(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/static/htmx.min.js")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("htmx.min.js = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Errorf("htmx content-type = %q, want a javascript type", ct)
	}

	resp, err = http.Get(srv.URL + "/static/nope.js")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("missing static asset = %d, want 404", resp.StatusCode)
	}
}

// --- bridge routes tolerate unknown ids (no panic, graceful 200) ---

func TestBridgeRoutesToleranceForUnknownIDs(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	cases := []struct {
		path string
		form url.Values
	}{
		{"/perm/ghost", url.Values{"approve": {"1"}}},
		{"/ask/ghost", url.Values{"answer": {"hi"}}},
		{"/plan/ghost", url.Values{"action": {"proceed"}}},
		{"/elicit/ghost", url.Values{"action": {"accept"}}},
		{"/perm/ghost", url.Values{"approve": {"0"}}},
		{"/ask/ghost", url.Values{}}, // empty answer
	}
	for _, c := range cases {
		resp, err := http.PostForm(srv.URL+c.path, c.form)
		if err != nil {
			t.Fatalf("%s: %v", c.path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s status = %d, want 200", c.path, resp.StatusCode)
		}
		// The OOB timeline refresh is the contract for all four bridges.
		if !strings.Contains(string(body), `id="timeline"`) {
			t.Errorf("%s missing OOB timeline: %q", c.path, body)
		}
	}
}

// --- malformed / hostile payloads must not 500 ---

func TestMalformedSendPayloads(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	bodies := []string{
		"",                                     // no body
		"prompt",                               // key without value
		"prompt=%ZZ",                           // invalid percent-encoding
		"prompt=" + strings.Repeat("A", 1<<16), // 64 KiB prompt
		"prompt=%F0%9F%98%80",                  // 4-byte emoji
		"notprompt=hello",                      // wrong field
	}
	for _, b := range bodies {
		resp, err := http.Post(srv.URL+"/send", "application/x-www-form-urlencoded", strings.NewReader(b))
		if err != nil {
			t.Fatalf("body %q: %v", truncForLog(b), err)
		}
		resp.Body.Close()
		if resp.StatusCode >= 500 {
			t.Errorf("body %q produced %d (server error)", truncForLog(b), resp.StatusCode)
		}
	}
}

func truncForLog(s string) string {
	if len(s) > 40 {
		return s[:40] + "…"
	}
	return s
}

// --- SSE transport contract ---

// The /events stream must open with a "ready" greeting and then carry the
// reducer's fragments for events the client emits, in order.
func TestEventStreamGreetsThenStreams(t *testing.T) {
	s, mock := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type = %q, want text/event-stream", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "no-cache") {
		t.Errorf("Cache-Control = %q, want no-cache", cc)
	}

	sc := bufio.NewScanner(resp.Body)
	// First event line must be the greeting.
	sawReady := false
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "event: ready") {
			sawReady = true
			break
		}
	}
	if !sawReady {
		t.Fatal("stream did not greet with event: ready")
	}

	// The first message delta re-renders the whole timeline (no prior live kind);
	// a second same-kind delta takes the incremental append fast path (event:
	// delta). Emit both and assert the incremental fragment arrives with its text.
	mock.Emit(copilot.Event{Type: copilot.EvMessageDelta, Text: "hello-"})
	mock.Emit(copilot.Event{Type: copilot.EvMessageDelta, Text: "sse"})
	var sawDelta, gotText bool
	deadline := time.Now().Add(2 * time.Second)
	for sc.Scan() && time.Now().Before(deadline) {
		line := sc.Text()
		if strings.HasPrefix(line, "event: delta") {
			sawDelta = true
		}
		if strings.Contains(line, "sse") {
			gotText = true
		}
		if sawDelta && gotText {
			break
		}
	}
	if !sawDelta || !gotText {
		t.Errorf("did not receive the incremental delta fragment over SSE (delta=%v text=%v)", sawDelta, gotText)
	}
}
