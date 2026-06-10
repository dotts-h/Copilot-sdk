package web

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotts-h/copilot-sdk/internal/config"
	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// newConnectionServer builds a session bound to a temp config dir (so saves
// persist to disk) with fakes on the impurity seams: lookPath (gh preflight),
// lookupEnv (token-var preflight) and setEnv (the paste flow) never touch the
// real process environment.
func newConnectionServer(t *testing.T) (*Server, *copilot.MockClient, string) {
	t.Helper()
	dir := t.TempDir()
	mock := copilot.NewMockClient()
	hub := New(Options{
		Client: mock,
		Forge:  &ctxforge.Forge{},
		Config: config.Default(dir),
		Meter:  telemetry.NewMeter(telemetry.DefaultPriceBook()),
		Logger: log.New(io.Discard, "", 0),
	})
	s := hub.newSession("connection")
	s.lookPath = func(string) (string, error) { return "", os.ErrNotExist }
	s.lookupEnv = func(string) string { return "" }
	s.setEnv = func(string, string) error { return nil }
	return s, mock, dir
}

func TestConnectionPageRendersStatusAndChooser(t *testing.T) {
	s, mock, _ := newConnectionServer(t)
	mock.Auth = copilot.AuthStatus{
		Authenticated: true, Method: "oauth", Login: "octocat", Host: "github.com",
	}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := get(t, srv, "/page/connection")
	for _, sub := range []string{
		"<h2>Connection</h2>",
		"octocat", "oauth", "github.com", // the live credential
		`hx-post="/connection"`,
		`name="authMethod"`, `value="auto"`, `value="token"`, `value="gh"`,
		`name="githubTokenEnv"`, `name="pasteToken"`,
		"next launch",                                      // method changes are not live (ADR-0039)
		"COPILOT_GITHUB_TOKEN", "GITHUB_TOKEN", "GH_TOKEN", // the precedence ladder
		"device flow",
	} {
		if !strings.Contains(body, sub) {
			t.Errorf("connection page missing %q", sub)
		}
	}
}

func TestConnectionPageOfflineMockShowsUnauthenticated(t *testing.T) {
	s, _, _ := newConnectionServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := get(t, srv, "/page/connection")
	if !strings.Contains(body, "not authenticated") {
		t.Errorf("offline mock should render an unauthenticated status\n%s", body)
	}
}

func TestConnectionPreflights(t *testing.T) {
	s, _, _ := newConnectionServer(t)
	s.config.AuthMethod = "gh"
	s.config.GitHubTokenEnv = "MY_TOKEN"
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	// gh missing on PATH and the token var unset → both warnings render.
	body := get(t, srv, "/page/connection")
	if !strings.Contains(body, "gh not found on PATH") {
		t.Errorf("missing gh preflight warning\n%s", body)
	}
	if !strings.Contains(body, "MY_TOKEN") || !strings.Contains(body, "unset") {
		t.Errorf("missing token-var unset preflight\n%s", body)
	}

	// Both resolve → no warnings.
	s.lookPath = func(string) (string, error) { return "/usr/bin/gh", nil }
	s.lookupEnv = func(name string) string {
		if name == "MY_TOKEN" {
			return "secret"
		}
		return ""
	}
	body = get(t, srv, "/page/connection")
	if strings.Contains(body, "gh not found on PATH") || strings.Contains(body, "unset") {
		t.Errorf("preflight warnings should clear when gh and the var resolve\n%s", body)
	}
	if strings.Contains(body, "secret") {
		t.Errorf("a resolved token value must never render\n%s", body)
	}
}

func TestConnectionSavePersistsMethod(t *testing.T) {
	s, _, dir := newConnectionServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/connection", url.Values{
		"authMethod": {"gh"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "saved") {
		t.Errorf("expected a saved confirmation, got: %s", body)
	}
	if s.config.AuthMethod != "gh" {
		t.Errorf("AuthMethod not applied in memory: %q", s.config.AuthMethod)
	}
	reloaded, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.AuthMethod != "gh" {
		t.Errorf("AuthMethod not persisted: %q", reloaded.AuthMethod)
	}

	// "auto" maps back to the empty (default) method.
	resp2, err := http.PostForm(srv.URL+"/connection", url.Values{"authMethod": {"auto"}})
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if s.config.AuthMethod != "" {
		t.Errorf("auto should persist as the empty method, got %q", s.config.AuthMethod)
	}
}

func TestConnectionSaveRejectsUnknownMethod(t *testing.T) {
	s, _, _ := newConnectionServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/connection", url.Values{"authMethod": {"device"}})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "error") && !strings.Contains(string(body), "invalid") {
		t.Errorf("unknown method should render a validation error, got: %s", body)
	}
	if s.config.AuthMethod != "" {
		t.Errorf("rejected save must roll back, got %q", s.config.AuthMethod)
	}
}

func TestConnectionTokenPasteNeverPersistsTheSecret(t *testing.T) {
	s, _, dir := newConnectionServer(t)
	var gotName, gotValue string
	s.setEnv = func(name, value string) error {
		gotName, gotValue = name, value
		return nil
	}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	const secret = "ghp_sekret_e2e_value"
	resp, err := http.PostForm(srv.URL+"/connection", url.Values{
		"authMethod":     {"token"},
		"githubTokenEnv": {"MY_ORCH_TOKEN"},
		"pasteToken":     {secret},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if gotName != "MY_ORCH_TOKEN" || gotValue != secret {
		t.Errorf("paste should land in the process env seam, got (%q, %q)", gotName, gotValue)
	}
	if strings.Contains(string(body), secret) {
		t.Error("the pasted secret must never be echoed back into HTML")
	}
	if s.config.GitHubTokenEnv != "MY_ORCH_TOKEN" || s.config.AuthMethod != "token" {
		t.Errorf("config should persist only the var NAME + method: %q %q",
			s.config.GitHubTokenEnv, s.config.AuthMethod)
	}
	// No secret at rest: the persisted config file carries the name, not the value.
	raw, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), secret) {
		t.Error("the pasted secret must never reach disk (ADR-0020/0039)")
	}
}

func TestConnectionTokenPasteRequiresAVarName(t *testing.T) {
	s, _, _ := newConnectionServer(t)
	called := false
	s.setEnv = func(string, string) error { called = true; return nil }
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/connection", url.Values{
		"authMethod": {"token"},
		"pasteToken": {"ghp_orphan"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if called {
		t.Error("a paste without a var name must not reach the env seam")
	}
	if !strings.Contains(string(body), "env var") {
		t.Errorf("expected an explanatory error, got: %s", body)
	}
}

// precedenceRows is the pure ladder renderer: the configured method marks its
// rung; auto marks the explicit-token rung only when a var is configured.
func TestPrecedenceRows(t *testing.T) {
	idx := func(rows []authRung) int {
		for i, r := range rows {
			if r.Active {
				return i
			}
		}
		return -1
	}

	if i := idx(precedenceRows("token", "MY_TOKEN")); i != 0 {
		t.Errorf("token method should mark the explicit-token rung, got %d", i)
	}
	rows := precedenceRows("gh", "")
	if i := idx(rows); i == -1 || !strings.Contains(rows[i].Label, "gh CLI") {
		t.Errorf("gh method should mark the gh rung, got %d (%+v)", i, rows)
	}
	if i := idx(precedenceRows("", "MY_TOKEN")); i != 0 {
		t.Errorf("auto with a configured var should mark the explicit-token rung, got %d", i)
	}
	if i := idx(precedenceRows("", "")); i != -1 {
		t.Errorf("plain auto marks no single rung (the CLI walks the ladder), got %d", i)
	}
}
