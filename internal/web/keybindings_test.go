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
