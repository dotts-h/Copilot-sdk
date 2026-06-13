// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package web

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// TestIndexRendersGroupedSidebar checks the shell renders the nav as a grouped
// left sidebar (V22, ADR-0026): a single <header> banner styled as a .sidebar,
// the four intent groups in order (Primary → Build → Observe → Config) with Help
// last, and every page rendered as a nav link under its group.
func TestIndexRendersGroupedSidebar(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := get(t, srv, "/")

	// The banner is laid out as a sidebar, still containing the <nav>.
	if !strings.Contains(body, "sidebar") {
		t.Errorf("index missing the sidebar layout class:\n%s", body)
	}

	// The four group labels render in intent order, Help pinned after Config.
	for _, label := range []string{"Primary", "Build", "Observe", "Config"} {
		if !strings.Contains(body, `nav-group-label">`+label) {
			t.Errorf("index missing nav-group-label %q", label)
		}
	}
	order := []string{
		`nav-group-label">Primary`,
		`nav-group-label">Build`,
		`nav-group-label">Observe`,
		`nav-group-label">Config`,
	}
	prev := -1
	for _, marker := range order {
		i := strings.Index(body, marker)
		if i < 0 {
			t.Fatalf("missing group marker %q", marker)
		}
		if i <= prev {
			t.Errorf("group %q out of order (at %d, want > %d)", marker, i, prev)
		}
		prev = i
	}
	// Help is the last nav destination, after the Config group.
	help := strings.Index(body, `hx-get="/page/help"`)
	if help < prev {
		t.Errorf("Help link (at %d) should come after the Config group (at %d)", help, prev)
	}

	// Every page is still a nav link under a group (sample one per group).
	for _, slug := range []string{"chat", "agents", "runs", "models", "help"} {
		if !strings.Contains(body, `hx-get="/page/`+slug+`"`) {
			t.Errorf("index missing nav link for %q", slug)
		}
	}

	// The theme toggle and cost footer stay reachable in the sidebar.
	for _, sub := range []string{`class="theme-toggle"`, `id="cost-footer"`} {
		if !strings.Contains(body, sub) {
			t.Errorf("index missing %q after the sidebar refactor", sub)
		}
	}
}

// TestIndexRendersCommandPalette checks the ⌘K command palette overlay is
// rendered into the shell (mirroring the help overlay): an aria-modal dialog
// with a filter input, one item per page carrying its slug/label/group, the
// toggle function, and the fixed ⌘/Ctrl-K binding.
func TestIndexRendersCommandPalette(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := get(t, srv, "/")

	for _, sub := range []string{
		`id="cmdk-overlay"`, `aria-modal="true"`, `class="cmdk-input"`,
		`class="cmdk-item"`, `data-slug="telemetry"`, `data-group="Observe"`,
		`function toggleCmdk`, `function cmdkFilter`,
	} {
		if !strings.Contains(body, sub) {
			t.Errorf("index missing command-palette piece %q:\n%s", sub, body)
		}
	}

	// ⌘/Ctrl-K is a fixed chord wired in the keydown dispatcher.
	if !strings.Contains(body, "metaKey") || !strings.Contains(body, "ctrlKey") {
		t.Errorf("index missing the ⌘/Ctrl-K modifier wiring")
	}
}
