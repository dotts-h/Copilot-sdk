package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultIsValid(t *testing.T) {
	if err := Default(t.TempDir()).Validate(); err != nil {
		t.Fatalf("default config invalid: %v", err)
	}
}

func TestLoadMissingReturnsDefault(t *testing.T) {
	dir := t.TempDir()
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.DefaultModel != "gpt-5" {
		t.Fatalf("expected default model, got %q", c.DefaultModel)
	}
	if c.Dir() != dir {
		t.Fatalf("dir not set: %q", c.Dir())
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	c := Default(dir)
	c.DefaultModel = "claude-opus-4.7"
	c.Theme = ThemeDark
	c.Telemetry.MonthlyCreditAllowance = 7000
	c.Telemetry.PriceOverrides = map[string][3]float64{"gpt-5": {1, 2, 3}}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json")); err != nil {
		t.Fatalf("config file not written: %v", err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.DefaultModel != "claude-opus-4.7" || loaded.Theme != ThemeDark {
		t.Fatalf("round trip lost fields: %+v", loaded)
	}
	if loaded.Telemetry.MonthlyCreditAllowance != 7000 {
		t.Fatalf("telemetry lost: %v", loaded.Telemetry)
	}
	if got := loaded.Telemetry.PriceOverrides["gpt-5"]; got != [3]float64{1, 2, 3} {
		t.Fatalf("price overrides lost: %v", got)
	}
}

func TestValidateRejectsBadValues(t *testing.T) {
	dir := t.TempDir()
	cases := []func(*Config){
		func(c *Config) { c.DefaultModel = "" },
		func(c *Config) { c.Theme = "neon" },
		func(c *Config) { c.ReasoningEffort = "infinite" },
		func(c *Config) { c.Telemetry.MonthlyCreditAllowance = -1 },
		func(c *Config) { c.Telemetry.WarnFraction = 2 },
	}
	for i, mut := range cases {
		c := Default(dir)
		mut(c)
		if err := c.Validate(); err == nil {
			t.Fatalf("case %d: expected validation error", i)
		}
	}
}

func TestLoadPopulatesNewDefaultsOnUpgrade(t *testing.T) {
	dir := t.TempDir()
	// Simulate an old config file missing newer fields.
	old := `{"defaultModel":"gpt-5","theme":"dark"}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.GitHubTokenEnv != "GITHUB_TOKEN" {
		t.Fatalf("upgrade did not backfill GitHubTokenEnv: %q", c.GitHubTokenEnv)
	}
	if c.Keybindings["quit"] == "" {
		t.Fatal("upgrade did not backfill keybindings")
	}
}

func TestGitHubTokenFromEnv(t *testing.T) {
	c := Default(t.TempDir())
	c.GitHubTokenEnv = "MY_ORCHESTRA_TEST_TOKEN"
	t.Setenv("MY_ORCHESTRA_TEST_TOKEN", "secret-value")
	if c.GitHubToken() != "secret-value" {
		t.Fatalf("token not resolved from env, got %q", c.GitHubToken())
	}
}

func TestKeyLookup(t *testing.T) {
	c := Default(t.TempDir())
	if c.Key("quit") != "ctrl+c" {
		t.Fatalf("unexpected quit binding %q", c.Key("quit"))
	}
	if c.Key("nonexistent") != "" {
		t.Fatal("unknown action should return empty binding")
	}
}
