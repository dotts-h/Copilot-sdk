// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/dotts-h/copilot-sdk/internal/config"
)

// TestIndexRendersKeymapAndOverlay checks the page shell carries the live
// action→key map (for the JS dispatcher) and the body-level help overlay.
func TestIndexRendersKeymapAndOverlay(t *testing.T) {
	s, _ := newSettingsServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := get(t, srv, "/")
	for _, sub := range []string{
		`data-keymap=`, `&#34;help&#34;:&#34;?&#34;`, // JSON escaped in the attribute
		`id="help-overlay"`, `Keyboard shortcuts`,
		`function toggleHelpOverlay`,
	} {
		if !strings.Contains(body, sub) {
			t.Errorf("index missing %q\n%s", sub, body)
		}
	}
}

// TestHelpPageListsShortcuts checks the Help page surfaces the shortcut table.
func TestHelpPageListsShortcuts(t *testing.T) {
	s, _ := newSettingsServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := get(t, srv, "/page/help")
	for _, sub := range []string{`Keyboard shortcuts`, `<kbd>?</kbd>`, `Jump to the chat composer`, `<kbd>Esc</kbd>`} {
		if !strings.Contains(body, sub) {
			t.Errorf("help page missing %q\n%s", sub, body)
		}
	}
}

// TestSettingsFormHasKeybindingFields checks the Settings form exposes one field
// per rebindable action, prefilled with the effective key.
func TestSettingsFormHasKeybindingFields(t *testing.T) {
	s, _ := newSettingsServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := get(t, srv, "/page/settings")
	for _, sub := range []string{
		`Keyboard shortcuts`, `name="key_help"`, `value="?"`,
		`name="key_focusComposer"`, `name="key_abort"`,
	} {
		if !strings.Contains(body, sub) {
			t.Errorf("settings form missing %q\n%s", sub, body)
		}
	}
}

// TestSettingsSaveAppliesKeybindingOverride checks a changed key is persisted as
// an override and reflected in the live keymap and the overlay.
func TestSettingsSaveAppliesKeybindingOverride(t *testing.T) {
	s, _ := newSettingsServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	form := url.Values{"defaultModel": {"gpt-5"}}
	// Rebind help to "h"; leave the rest at their defaults (sent as-is).
	form.Set("key_help", "h")
	form.Set("key_focusComposer", "c")
	form.Set("key_newChat", "n")
	form.Set("key_abort", "x")
	form.Set("key_gotoTelemetry", "t")
	form.Set("key_gotoSettings", ",")
	resp, err := http.PostForm(srv.URL+"/settings", form)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "saved") {
		t.Fatalf("expected saved confirmation, got: %s", body)
	}
	if s.config.KeyBindings["help"] != "h" {
		t.Fatalf("override not stored: %+v", s.config.KeyBindings)
	}
	// A default-valued field is not stored as an override.
	if _, ok := s.config.KeyBindings["focusComposer"]; ok {
		t.Errorf("default-valued field should not be stored: %+v", s.config.KeyBindings)
	}
	// The overlay reflects the new key.
	if !strings.Contains(get(t, srv, "/"), `<kbd>h</kbd>`) {
		t.Errorf("overlay did not pick up the rebound key")
	}
}

// TestSettingsSaveLiveAppliesKeymapViaOOB checks the V10 live-apply: a successful
// rebind POST returns, alongside the persisted settings partial, an hx-swap-oob
// re-render of the help overlay AND an applyKeymap script carrying the NEW binding,
// so the JS dispatcher picks it up without a full page reload (TECH_DEBT #13).
func TestSettingsSaveLiveAppliesKeymapViaOOB(t *testing.T) {
	s, _ := newSettingsServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	form := url.Values{"defaultModel": {"gpt-5"}}
	form.Set("key_help", "h") // rebind help "?" → "h"
	form.Set("key_focusComposer", "c")
	form.Set("key_newChat", "n")
	form.Set("key_abort", "x")
	form.Set("key_gotoTelemetry", "t")
	form.Set("key_gotoSettings", ",")
	resp, err := http.PostForm(srv.URL+"/settings", form)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	got := string(body)
	for _, sub := range []string{
		"saved",                // the persisted settings partial is still returned
		`hx-swap-oob="true"`,   // the OOB help-overlay re-render
		`id="help-overlay"`,    //   matched by its body-level id
		`<kbd>h</kbd>`,         // the overlay reflects the new binding
		`<script>applyKeymap(`, // the <body data-keymap> live update
		`"help":"h"`,           // … carrying the NEW binding (raw JSON in the script)
	} {
		if !strings.Contains(got, sub) {
			t.Errorf("live-apply response missing %q\n%s", sub, got)
		}
	}
}

// TestSettingsSaveLiveApplyMatchesPersistedKeymap checks the live-apply payload
// always carries the PERSISTED keymap, so the live <body data-keymap> can never
// desync from disk: a no-op save (bindings sent unchanged) re-emits the default
// keymap, and a rolled-back invalid save emits NO keymap at all (the error branch),
// leaving the browser's live attribute matching the still-default persisted config.
func TestSettingsSaveLiveApplyMatchesPersistedKeymap(t *testing.T) {
	s, _ := newSettingsServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	defaults := url.Values{"defaultModel": {"gpt-5"}}
	defaults.Set("key_help", "?")
	defaults.Set("key_focusComposer", "c")
	defaults.Set("key_newChat", "n")
	defaults.Set("key_abort", "x")
	defaults.Set("key_gotoTelemetry", "t")
	defaults.Set("key_gotoSettings", ",")

	// No-op save (all defaults): the persisted keymap is unchanged, so the emitted
	// applyKeymap carries the default help "?", never a stale/desynced value.
	resp, err := http.PostForm(srv.URL+"/settings", defaults)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if got := string(body); !strings.Contains(got, `"help":"?"`) {
		t.Errorf("no-op save should re-emit the persisted (default) keymap\n%s", got)
	}
	if len(s.config.KeyBindings) != 0 {
		t.Errorf("a no-op save must persist no overrides: %+v", s.config.KeyBindings)
	}

	// Rolled-back invalid save (duplicate key): the error branch returns no
	// live-apply payload, so the browser's live keymap stays the persisted default.
	bad := url.Values{"defaultModel": {"gpt-5"}}
	bad.Set("key_help", "c") // collides with focusComposer's "c"
	bad.Set("key_focusComposer", "c")
	resp2, err := http.PostForm(srv.URL+"/settings", bad)
	if err != nil {
		t.Fatal(err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if got := string(body2); strings.Contains(got, "applyKeymap(") || strings.Contains(got, `hx-swap-oob`) {
		t.Errorf("a rolled-back invalid save must not emit a (desynced) live keymap\n%s", got)
	}
}

// TestSettingsSaveLiveApplyEscapesBinding checks a binding key that is itself an
// HTML metacharacter (a single "<" / "&" is a valid single-char rebind) is escaped
// on every live-apply surface — the overlay through html/template/esc and the
// applyKeymap JSON through encoding/json — so config/user-originated keymap text is
// never emitted raw (ADR-0001), mirroring the escape-test family.
func TestSettingsSaveLiveApplyEscapesBinding(t *testing.T) {
	s, _ := newSettingsServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	form := url.Values{"defaultModel": {"gpt-5"}}
	form.Set("key_help", "<") // a single "<" — a valid single-char key, an HTML metachar
	form.Set("key_focusComposer", "&")
	form.Set("key_newChat", "n")
	form.Set("key_abort", "x")
	form.Set("key_gotoTelemetry", "t")
	form.Set("key_gotoSettings", ",")
	resp, err := http.PostForm(srv.URL+"/settings", form)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	got := string(body)
	// The overlay renders the keys HTML-escaped, never raw.
	if !strings.Contains(got, `<kbd>&lt;</kbd>`) || !strings.Contains(got, `<kbd>&amp;</kbd>`) {
		t.Errorf("overlay did not HTML-escape the bound metacharacters\n%s", got)
	}
	if strings.Contains(got, `<kbd><</kbd>`) {
		t.Errorf("overlay emitted a raw '<' binding (XSS): \n%s", got)
	}
	// The applyKeymap JSON unicode-escapes <, >, & (encoding/json), so no literal
	// </script> can form from a bound metacharacter and the script stays safe.
	if !strings.Contains(got, `applyKeymap(`) {
		t.Fatalf("expected an applyKeymap live-update script\n%s", got)
	}
	if !strings.Contains(got, `"help":"\u003c"`) || !strings.Contains(got, `"focusComposer":"\u0026"`) {
		t.Errorf("applyKeymap JSON did not unicode-escape the bound metacharacters\n%s", got)
	}
}

// TestSettingsSavePreservesBindingsOnPartialPost checks a save that omits the
// keyboard-shortcut section leaves existing overrides intact (preserve-on-absent),
// rather than wiping them — distinct from a present-but-blank field reverting.
func TestSettingsSavePreservesBindingsOnPartialPost(t *testing.T) {
	s, _ := newSettingsServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	// Seed an override directly, then POST only the model field (no key_* fields).
	if err := s.editConfig(func(c *config.Config) {
		c.KeyBindings = map[string]string{"help": "h"}
	}); err != nil {
		t.Fatal(err)
	}
	resp, err := http.PostForm(srv.URL+"/settings", url.Values{"defaultModel": {"gpt-5"}})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if s.config.KeyBindings["help"] != "h" {
		t.Fatalf("partial save wiped the binding: %+v", s.config.KeyBindings)
	}
}

// TestSettingsSaveRejectsDuplicateKey checks a colliding binding is rejected and
// rolled back, leaving the live config untouched.
func TestSettingsSaveRejectsDuplicateKey(t *testing.T) {
	s, _ := newSettingsServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	form := url.Values{"defaultModel": {"gpt-5"}}
	form.Set("key_help", "c") // collides with focusComposer's default "c"
	form.Set("key_focusComposer", "c")
	resp, err := http.PostForm(srv.URL+"/settings", form)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "⚠") && !strings.Contains(string(body), "bound to both") {
		t.Errorf("expected a validation error for the duplicate key, got: %s", body)
	}
	if _, ok := s.config.KeyBindings["help"]; ok {
		t.Errorf("invalid save mutated config: %+v", s.config.KeyBindings)
	}
}
