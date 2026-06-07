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

func TestSettingsFormRendersPriceOverrideRows(t *testing.T) {
	s, _ := newSettingsServer(t)
	// Seed an override so the row pre-fills from config.
	s.config.Telemetry.PriceOverrides = map[string][3]float64{"gpt-5": {99, 9, 88}}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := get(t, srv, "/page/settings")
	for _, sub := range []string{
		"Per-model price overrides",
		`name="price.`,              // index-keyed rows exist
		`.model"`,                   // each row carries its hidden model id
		`.in"`, `.cached"`, `.out"`, // three numeric fields per model
		"gpt-5",              // a default model is listed
		`value="99"`,         // the seeded override pre-fills the input field
		`placeholder="1.25"`, // the built-in default shows as a placeholder
	} {
		if !strings.Contains(body, sub) {
			t.Errorf("price-override section missing %q\n%s", sub, body)
		}
	}
}

func TestSettingsSaveRoundTripsPriceOverrideAndRepricesLive(t *testing.T) {
	s, dir := newSettingsServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	// Post a single override row for gpt-5 (index 0 is enough — the parser walks
	// contiguous indices from the POST, independent of the rendered model count).
	resp, err := http.PostForm(srv.URL+"/settings", url.Values{
		"defaultModel":   {"gpt-5"},
		"price.0.model":  {"gpt-5"},
		"price.0.in":     {"99"},
		"price.0.cached": {"9"},
		"price.0.out":    {"88"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "saved") {
		t.Fatalf("expected a saved confirmation, got: %s", body)
	}

	// Persisted into config (in memory and on disk).
	if got := s.config.Telemetry.PriceOverrides["gpt-5"]; got != [3]float64{99, 9, 88} {
		t.Errorf("override not applied in memory: %v", got)
	}
	reloaded, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Telemetry.PriceOverrides["gpt-5"]; got != [3]float64{99, 9, 88} {
		t.Errorf("override not persisted: %v", got)
	}

	// The live meter reprices the next turn — both the account meter (gate/cap) and
	// the per-session meter (statusline), which share the one price book.
	if got := s.meter.EstimateTurn("gpt-5", 1_000_000).InputUSD; got != 99 {
		t.Errorf("account meter did not reprice: got $%v/Mt, want $99", got)
	}
	if got := s.sessionMeter.EstimateTurn("gpt-5", 1_000_000).InputUSD; got != 99 {
		t.Errorf("session meter did not reprice (drift): got $%v/Mt, want $99", got)
	}
}

func TestSettingsSaveBlankPriceRowPersistsNoOverride(t *testing.T) {
	s, dir := newSettingsServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	// A row with all-blank rate fields must NOT persist a $0-rate override; the
	// model falls back to its default.
	resp, err := http.PostForm(srv.URL+"/settings", url.Values{
		"defaultModel":   {"gpt-5"},
		"price.0.model":  {"gpt-5"},
		"price.0.in":     {""},
		"price.0.cached": {""},
		"price.0.out":    {""},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if _, ok := s.config.Telemetry.PriceOverrides["gpt-5"]; ok {
		t.Errorf("a blank row must not persist an override: %v", s.config.Telemetry.PriceOverrides)
	}
	// The live meter still prices gpt-5 at the built-in default ($1.25/Mt in).
	if got := s.meter.EstimateTurn("gpt-5", 1_000_000).InputUSD; got != 1.25 {
		t.Errorf("blank row must leave the default rate: got $%v/Mt, want $1.25", got)
	}
	reloaded, _ := config.Load(dir)
	if len(reloaded.Telemetry.PriceOverrides) != 0 {
		t.Errorf("blank row persisted an override: %v", reloaded.Telemetry.PriceOverrides)
	}
}

func TestSettingsSavePartialPriceRowFillsBlanksWithDefault(t *testing.T) {
	s, dir := newSettingsServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	// Edit only the output rate for gpt-5, leaving input/cached blank: the blanks
	// must keep gpt-5's built-in defaults (1.25 / 0.125), NOT silently become $0.
	resp, err := http.PostForm(srv.URL+"/settings", url.Values{
		"defaultModel":   {"gpt-5"},
		"price.0.model":  {"gpt-5"},
		"price.0.in":     {""},
		"price.0.cached": {""},
		"price.0.out":    {"88"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	want := [3]float64{1.25, 0.125, 88}
	if got := s.config.Telemetry.PriceOverrides["gpt-5"]; got != want {
		t.Errorf("partial row: blanks should keep defaults, got %v want %v", got, want)
	}
	reloaded, _ := config.Load(dir)
	if got := reloaded.Telemetry.PriceOverrides["gpt-5"]; got != want {
		t.Errorf("partial row not persisted with defaults: %v", got)
	}
	// The live meter still prices input at the default (1.25), not $0.
	if got := s.meter.EstimateTurn("gpt-5", 1_000_000).InputUSD; got != 1.25 {
		t.Errorf("blank input cell underpriced to $%v/Mt, want the default $1.25", got)
	}
}

func TestSettingsSaveNegativeRateRollsBack(t *testing.T) {
	s, dir := newSettingsServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/settings", url.Values{
		"defaultModel":   {"gpt-5"},
		"price.0.model":  {"gpt-5"},
		"price.0.in":     {"-5"}, // invalid: negative rate
		"price.0.cached": {"0"},
		"price.0.out":    {"0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "priceOverrides") && !strings.Contains(string(body), ">= 0") {
		t.Errorf("expected a validation error, got: %s", body)
	}

	// Rolled back: config stays clean (no override) and the live meter is untouched.
	if len(s.config.Telemetry.PriceOverrides) != 0 {
		t.Errorf("invalid save mutated config: %v", s.config.Telemetry.PriceOverrides)
	}
	if got := s.meter.EstimateTurn("gpt-5", 1_000_000).InputUSD; got != 1.25 {
		t.Errorf("invalid save repriced the live meter: got $%v/Mt, want $1.25", got)
	}
	reloaded, _ := config.Load(dir)
	if len(reloaded.Telemetry.PriceOverrides) != 0 {
		t.Errorf("invalid save persisted an override: %v", reloaded.Telemetry.PriceOverrides)
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
