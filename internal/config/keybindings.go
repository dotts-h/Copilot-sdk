// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package config

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// Keyboard shortcuts. The set of rebindable actions is fixed in code (the
// frontend dispatches on each action id), so it can't drift out from under the
// JS dispatcher; only the chosen keys are persisted (Config.KeyBindings holds
// the per-action overrides). The Settings page edits them and the help overlay
// lists them. Validation lives here so the keymap stays consistent before save.

// KeyAction is one rebindable UI action: a stable id, a human label for the help
// overlay and Settings form, and the built-in default key.
type KeyAction struct {
	ID      string
	Label   string
	Default string
}

// KeyActions is the canonical, ordered set of rebindable actions. The order is
// stable so the overlay, the Settings form, and the dispatch map render
// deterministically.
func KeyActions() []KeyAction {
	return []KeyAction{
		{ID: "help", Label: "Show this keyboard-shortcuts overlay", Default: "?"},
		{ID: "focusComposer", Label: "Jump to the chat composer", Default: "c"},
		{ID: "newChat", Label: "Start a fresh chat session", Default: "n"},
		{ID: "abort", Label: "Stop the in-flight turn", Default: "x"},
		{ID: "gotoTelemetry", Label: "Open the Telemetry page", Default: "t"},
		{ID: "gotoSettings", Label: "Open Settings", Default: ","},
	}
}

// ResolvedKey is one action paired with its effective key (the override when
// set, otherwise the default), for rendering the surfaces and the JS map.
type ResolvedKey struct {
	ID    string
	Label string
	Key   string
}

// Keymap resolves the full ordered action set to its effective keys, applying
// any per-action override in KeyBindings over the built-in default.
func (c *Config) Keymap() []ResolvedKey {
	actions := KeyActions()
	out := make([]ResolvedKey, 0, len(actions))
	for _, a := range actions {
		key := a.Default
		if ov, ok := c.KeyBindings[a.ID]; ok {
			if t := strings.TrimSpace(ov); t != "" {
				key = t
			}
		}
		out = append(out, ResolvedKey{ID: a.ID, Label: a.Label, Key: key})
	}
	return out
}

// knownKeyAction reports whether id names a defined rebindable action.
func knownKeyAction(id string) bool {
	for _, a := range KeyActions() {
		if a.ID == id {
			return true
		}
	}
	return false
}

// normalizeKeyBindings trims each override and drops empties (a blank reverts to
// the default), collapsing an empty map to nil so older files stay clean.
func (c *Config) normalizeKeyBindings() {
	for id, v := range c.KeyBindings {
		if t := strings.TrimSpace(v); t == "" {
			delete(c.KeyBindings, id)
		} else {
			c.KeyBindings[id] = t
		}
	}
	if len(c.KeyBindings) == 0 {
		c.KeyBindings = nil
	}
}

// validateKeyBindings enforces the invariants the UI and the JS dispatcher rely
// on: every override names a known action; every effective key is a single
// character (the dispatcher compares against event.key); and no two actions
// resolve to the same key, which would make a keystroke ambiguous.
func (c *Config) validateKeyBindings() error {
	ids := make([]string, 0, len(c.KeyBindings))
	for id := range c.KeyBindings {
		ids = append(ids, id)
	}
	sort.Strings(ids) // deterministic error message regardless of map order
	for _, id := range ids {
		if !knownKeyAction(id) {
			return fmt.Errorf("unknown key binding %q", id)
		}
	}
	seen := make(map[string]string, len(KeyActions()))
	for _, b := range c.Keymap() {
		if utf8.RuneCountInString(b.Key) != 1 || strings.TrimSpace(b.Key) == "" {
			return fmt.Errorf("key for %q must be a single character", b.ID)
		}
		if prev, dup := seen[b.Key]; dup {
			return fmt.Errorf("key %q is bound to both %q and %q", b.Key, prev, b.ID)
		}
		seen[b.Key] = b.ID
	}
	return nil
}
