// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

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
	c.Telemetry.MonthlyCreditAllowance = 7000
	c.Telemetry.HardCapCredits = 250
	c.Telemetry.RunCapCredits = 40
	c.Telemetry.PriceOverrides = map[string][]float64{"gpt-5": {1, 2, 3}}
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
	if loaded.DefaultModel != "claude-opus-4.7" {
		t.Fatalf("round trip lost fields: %+v", loaded)
	}
	if loaded.Telemetry.MonthlyCreditAllowance != 7000 {
		t.Fatalf("telemetry lost: %v", loaded.Telemetry)
	}
	if loaded.Telemetry.HardCapCredits != 250 {
		t.Fatalf("hard cap lost on round trip: %v", loaded.Telemetry)
	}
	if loaded.Telemetry.RunCapCredits != 40 {
		t.Fatalf("run cap lost on round trip: %v", loaded.Telemetry)
	}
	if got := loaded.Telemetry.PriceOverrides["gpt-5"]; len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Fatalf("price overrides lost: %v", got)
	}
}

func TestValidateRejectsBadValues(t *testing.T) {
	dir := t.TempDir()
	cases := []func(*Config){
		func(c *Config) { c.DefaultModel = "" },
		func(c *Config) { c.ReasoningEffort = "infinite" },
		func(c *Config) { c.Telemetry.MonthlyCreditAllowance = -1 },
		func(c *Config) { c.Telemetry.WarnFraction = 2 },
		func(c *Config) { c.Telemetry.HardCapCredits = -1 },
		func(c *Config) { c.Telemetry.RunCapCredits = -1 },
		func(c *Config) { c.Telemetry.PriceOverrides = map[string][]float64{"gpt-5": {-1, 0, 0}} },
		func(c *Config) { c.Telemetry.PriceOverrides = map[string][]float64{"gpt-5": {1, 2, -3}} },
		func(c *Config) { c.Telemetry.PriceOverrides = map[string][]float64{"gpt-5": {1, 2}} },          // too short (ADR-0034)
		func(c *Config) { c.Telemetry.PriceOverrides = map[string][]float64{"gpt-5": {1, 2, 3, 4, 5}} }, // too long
		func(c *Config) { c.Telemetry.PriceOverrides = map[string][]float64{"gpt-5": {1, 2, 3, -4}} },   // negative cache-write
	}
	for i, mut := range cases {
		c := Default(dir)
		mut(c)
		if err := c.Validate(); err == nil {
			t.Fatalf("case %d: expected validation error", i)
		}
	}
}

func TestValidateAcceptsPriceOverrides(t *testing.T) {
	dir := t.TempDir()

	// A non-negative per-model rate table is valid.
	c := Default(dir)
	c.Telemetry.PriceOverrides = map[string][]float64{
		"gpt-5":             {1.25, 0.125, 10},  // legacy 3-element row stays valid
		"claude-opus-4.7":   {0, 0, 0},          // zero rates are non-negative → valid
		"claude-sonnet-4.6": {3, 0.3, 15, 3.75}, // 4-element row with explicit cache-write (ADR-0034)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("non-negative price overrides should validate: %v", err)
	}

	// A legacy 3-element override JSON loads (backward-readable migration, ADR-0034).
	if err := os.WriteFile(filepath.Join(dir, "config.json"),
		[]byte(`{"defaultModel":"gpt-5","telemetry":{"priceOverrides":{"gpt-5":[1.25,0.125,10]}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	legacy, err := Load(dir)
	if err != nil {
		t.Fatalf("legacy 3-element priceOverrides must load: %v", err)
	}
	if got := legacy.Telemetry.PriceOverrides["gpt-5"]; len(got) != 3 {
		t.Fatalf("legacy override should read back 3 elements, got %v", got)
	}

	// Absent overrides (older config without priceOverrides) stay valid.
	c2 := Default(dir)
	c2.Telemetry.PriceOverrides = nil
	if err := c2.Validate(); err != nil {
		t.Fatalf("absent price overrides should validate: %v", err)
	}
}

func TestLoadPopulatesNewDefaultsOnUpgrade(t *testing.T) {
	dir := t.TempDir()
	// Simulate an old config file missing newer fields.
	old := `{"defaultModel":"gpt-5"}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	// GitHubTokenEnv stays empty by default: the app uses the logged-in copilot
	// CLI session rather than auto-binding to an ambient GITHUB_TOKEN.
	if c.GitHubTokenEnv != "" {
		t.Fatalf("GitHubTokenEnv should default to empty, got %q", c.GitHubTokenEnv)
	}
	// A field absent from an older file is backfilled from defaults.
	if c.ReasoningEffort != "medium" {
		t.Fatalf("upgrade did not backfill reasoningEffort, got %q", c.ReasoningEffort)
	}
	if c.ForgeDir == "" {
		t.Fatal("upgrade did not backfill forgeDir")
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
