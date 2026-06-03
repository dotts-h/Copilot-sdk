package tui

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// truncate must count runes, not bytes, so multibyte text is never split mid-
// rune into invalid UTF-8.
func TestTruncateIsRuneSafe(t *testing.T) {
	cases := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"ascii under limit", "hello", 10, "hello"},
		{"ascii cut", "hello world", 5, "hell…"},
		{"multibyte under limit", "café ☕", 10, "café ☕"},
		{"multibyte cut keeps whole runes", "café ☕ latte", 6, "café …"},
		{"emoji cut", "🚀🚀🚀🚀", 2, "🚀…"},
	}
	for _, tc := range cases {
		got := truncate(tc.in, tc.n)
		if got != tc.want {
			t.Errorf("%s: truncate(%q,%d)=%q, want %q", tc.name, tc.in, tc.n, got, tc.want)
		}
		if !utf8.ValidString(got) {
			t.Errorf("%s: truncate produced invalid UTF-8: %q", tc.name, got)
		}
	}
}

// clampLines bounds a large tool result and notes how much it elided.
func TestClampLines(t *testing.T) {
	in := strings.Join([]string{"a", "b", "c", "d", "e"}, "\n")
	got := clampLines(in, 2)
	if !strings.HasPrefix(got, "a\nb\n") {
		t.Fatalf("kept lines wrong: %q", got)
	}
	if !strings.Contains(got, "+3 more lines") {
		t.Fatalf("elision note missing: %q", got)
	}
	if clampLines("x\ny", 5) != "x\ny" {
		t.Fatal("should not clamp when under the limit")
	}
}
