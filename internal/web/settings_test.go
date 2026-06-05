package web

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/dotts-h/copilot-sdk/internal/config"
	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// newSettingsServer builds a session whose config is bound to a temp dir, so
// Save() actually persists and we can reload from disk to prove it.
func newSettingsServer(t *testing.T) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	c := config.Default(dir)
	hub := New(Options{
		Client: copilot.NewMockClient(),
		Forge:  &ctxforge.Forge{},
		Config: c,
		Meter:  telemetry.NewMeter(telemetry.DefaultPriceBook()),
		Logger: log.New(io.Discard, "", 0),
	})
	return hub.newSession("settings"), dir
}

func TestSettingsPageRendersEditableForm(t *testing.T) {
	s, _ := newSettingsServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := get(t, srv, "/page/settings")
	for _, sub := range []string{
		`<form`, `hx-post="/settings"`,
		`name="defaultModel"`, `value="gpt-5"`,
		`name="reasoningEffort"`, `name="streaming"`, `name="autoApproveTools"`,
		`name="allowance"`, `name="warnPercent"`, `name="hardCap"`, `name="otlpEndpoint"`,
		`name="githubTokenEnv"`,
	} {
		if !strings.Contains(body, sub) {
			t.Errorf("settings form missing %q\n%s", sub, body)
		}
	}
}

func TestSettingsSavePersistsAndApplies(t *testing.T) {
	s, dir := newSettingsServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/settings", url.Values{
		"defaultModel":     {"claude-opus-4-8"},
		"defaultAgent":     {"builder"},
		"reasoningEffort":  {"high"},
		"streaming":        {""}, // unchecked
		"autoApproveTools": {"on"},
		"allowance":        {"3000"},
		"warnPercent":      {"75"},
		"hardCap":          {"500"},
		"otlpEndpoint":     {"http://localhost:4317"},
		"githubTokenEnv":   {"GH_PAT"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "saved") {
		t.Errorf("expected a saved confirmation, got: %s", body)
	}

	// In-memory config applied.
	if s.config.DefaultModel != "claude-opus-4-8" || s.config.ReasoningEffort != "high" ||
		s.config.Streaming != false || s.config.AutoApproveTools != true ||
		s.config.DefaultAgent != "builder" || s.config.GitHubTokenEnv != "GH_PAT" {
		t.Errorf("config not applied in memory: %+v", s.config)
	}
	if s.config.Telemetry.MonthlyCreditAllowance != 3000 || s.config.Telemetry.WarnFraction != 0.75 ||
		s.config.Telemetry.HardCapCredits != 500 ||
		s.config.Telemetry.OTLPEndpoint != "http://localhost:4317" {
		t.Errorf("telemetry config not applied: %+v", s.config.Telemetry)
	}
	// The saved budget knobs are refreshed onto the live session immediately.
	if s.hardCap != 500 || s.allowance != 3000 || s.warnFraction != 0.75 {
		t.Errorf("budget not refreshed onto the live session: cap=%v allowance=%v warn=%v",
			s.hardCap, s.allowance, s.warnFraction)
	}

	// Persisted to disk: reload and confirm.
	reloaded, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.DefaultModel != "claude-opus-4-8" || reloaded.Telemetry.WarnFraction != 0.75 {
		t.Errorf("config not persisted: %+v", reloaded)
	}
}

func TestSettingsSaveInvalidRollsBack(t *testing.T) {
	s, dir := newSettingsServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/settings", url.Values{
		"defaultModel": {""}, // invalid: required
		"warnPercent":  {"50"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "defaultModel") && !strings.Contains(string(body), "required") {
		t.Errorf("expected a validation error, got: %s", body)
	}
	// In-memory config unchanged (rolled back).
	if s.config.DefaultModel != "gpt-5" {
		t.Errorf("invalid save mutated config: %q", s.config.DefaultModel)
	}
	// Nothing persisted: disk still default.
	reloaded, _ := config.Load(dir)
	if reloaded.DefaultModel != "gpt-5" {
		t.Errorf("invalid save persisted: %q", reloaded.DefaultModel)
	}
}
