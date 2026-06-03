package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMatchCommands(t *testing.T) {
	for _, tc := range []struct {
		in      string
		wantAny []string // command names that must appear
		wantLen int      // -1 means "don't care about exact count"
	}{
		{"/", nil, -1},                           // bare slash → all commands
		{"/mo", []string{"model", "models"}, 2},  // prefix match (command + nav slug)
		{"/c", []string{"clear", "cost"}, -1},    // multiple prefix matches
		{"/model gpt-5", nil, 0},                 // space → typing args, no menu
		{"hello", nil, 0},                        // not a command
		{"/telemetry", []string{"telemetry"}, 1}, // nav slug is a command too
		{"/zzz", nil, 0},                         // no match
	} {
		got := matchCommands(tc.in)
		if tc.wantLen >= 0 && len(got) != tc.wantLen {
			t.Errorf("matchCommands(%q) len = %d, want %d (%v)", tc.in, len(got), tc.wantLen, got)
		}
		for _, name := range tc.wantAny {
			found := false
			for _, c := range got {
				if c.Name == name {
					found = true
				}
			}
			if !found {
				t.Errorf("matchCommands(%q) missing %q, got %v", tc.in, name, got)
			}
		}
	}
}

func TestCommandsEndpoint(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	get := func(q string) string {
		resp, err := http.Get(srv.URL + "/commands?prompt=" + q)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}

	// A prefix that matches yields a menu with a clickable item.
	body := get("%2Fmo") // "/mo"
	if !strings.Contains(body, "cmd-menu") || !strings.Contains(body, "/model") {
		t.Errorf("autocomplete for /mo = %q", body)
	}
	if !strings.Contains(body, "fillCmd") {
		t.Errorf("autocomplete item not clickable: %q", body)
	}

	// A non-command input yields an empty menu (clears it).
	if body := get("hello"); strings.TrimSpace(body) != "" {
		t.Errorf("non-command should clear menu, got %q", body)
	}
}
