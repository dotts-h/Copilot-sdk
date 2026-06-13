// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKeymapAppliesDefaultsAndOverrides(t *testing.T) {
	c := Default(t.TempDir())
	c.KeyBindings = map[string]string{"help": "h"}
	keymap := c.Keymap()
	if len(keymap) != len(KeyActions()) {
		t.Fatalf("keymap should cover every action: got %d, want %d", len(keymap), len(KeyActions()))
	}
	got := map[string]string{}
	for _, k := range keymap {
		got[k.ID] = k.Key
	}
	if got["help"] != "h" {
		t.Errorf("override not applied: help = %q", got["help"])
	}
	if got["focusComposer"] != "c" {
		t.Errorf("default not applied: focusComposer = %q", got["focusComposer"])
	}
}

func TestValidateRejectsBadKeyBindings(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name string
		mut  func(*Config)
	}{
		{"unknown action", func(c *Config) { c.KeyBindings = map[string]string{"nope": "z"} }},
		{"multi-char key", func(c *Config) { c.KeyBindings = map[string]string{"help": "ctrl"} }},
		{"duplicate of another default", func(c *Config) { c.KeyBindings = map[string]string{"help": "c"} }},
		{"two overrides collide", func(c *Config) { c.KeyBindings = map[string]string{"help": "z", "abort": "z"} }},
	}
	for _, tc := range cases {
		c := Default(dir)
		tc.mut(c)
		if err := c.Validate(); err == nil {
			t.Errorf("%s: expected a validation error", tc.name)
		}
	}
}

func TestKeyBindingNormalizeDropsEmptyAndRevertsToDefault(t *testing.T) {
	c := Default(t.TempDir())
	c.KeyBindings = map[string]string{"help": "  ", "abort": " z "}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	// A blank override is dropped (reverts to default); a padded one is trimmed.
	if _, ok := c.KeyBindings["help"]; ok {
		t.Errorf("blank override should be dropped, got %q", c.KeyBindings["help"])
	}
	if c.KeyBindings["abort"] != "z" {
		t.Errorf("override not trimmed: %q", c.KeyBindings["abort"])
	}
	if got := keyFor(c.Keymap(), "help"); got != "?" {
		t.Errorf("help should fall back to its default, got %q", got)
	}
}

func TestKeyBindingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	c := Default(dir)
	c.KeyBindings = map[string]string{"help": "h", "abort": "q"}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.KeyBindings["help"] != "h" || loaded.KeyBindings["abort"] != "q" {
		t.Fatalf("key bindings lost on round trip: %+v", loaded.KeyBindings)
	}
}

func TestEmptyKeyBindingsOmittedFromDisk(t *testing.T) {
	dir := t.TempDir()
	c := Default(dir)
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, configFile))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); contains(got, "keyBindings") {
		t.Errorf("empty key bindings should be omitted from disk:\n%s", got)
	}
}

func keyFor(keymap []ResolvedKey, id string) string {
	for _, k := range keymap {
		if k.ID == id {
			return k.Key
		}
	}
	return ""
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
