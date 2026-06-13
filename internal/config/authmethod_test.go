// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package config

import (
	"os"
	"path/filepath"
	"testing"
)

// AuthMethod selects how the app authenticates to Copilot (ADR-0039): "" (auto:
// the CLI's own chain), "token" (explicit ${VAR} token), "gh" (reuse the gh CLI
// credential). Validation is membership-only — a "token" method with an unset
// var degrades at dial time and is surfaced by the Connection page preflight,
// not rejected here (a Settings save clearing the var must not strand the file).
func TestAuthMethodValidate(t *testing.T) {
	for _, valid := range []string{"", "token", "gh"} {
		c := Default(t.TempDir())
		c.AuthMethod = valid
		if err := c.Validate(); err != nil {
			t.Errorf("AuthMethod %q should be valid: %v", valid, err)
		}
	}
	c := Default(t.TempDir())
	c.AuthMethod = "device"
	if err := c.Validate(); err == nil {
		t.Error("unknown AuthMethod should be rejected")
	}
}

func TestAuthMethodDefaultsEmptyAndOlderFilesLoadClean(t *testing.T) {
	dir := t.TempDir()
	if c := Default(dir); c.AuthMethod != "" {
		t.Errorf("AuthMethod should default to auto (empty), got %q", c.AuthMethod)
	}

	// A pre-0068 config file (no authMethod key) loads with the auto default.
	older := `{"defaultModel":"gpt-5","reasoningEffort":"medium","telemetry":{"monthlyCreditAllowance":1500,"warnFraction":0.8}}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(older), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.AuthMethod != "" {
		t.Errorf("older file should load AuthMethod auto, got %q", c.AuthMethod)
	}

	// Round-trip: a saved method persists and reloads.
	c.AuthMethod = "gh"
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.AuthMethod != "gh" {
		t.Errorf("AuthMethod not persisted, got %q", reloaded.AuthMethod)
	}
}
