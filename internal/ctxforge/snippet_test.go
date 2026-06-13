// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package ctxforge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSnippetValidate(t *testing.T) {
	for _, tc := range []struct {
		name    string
		snip    Snippet
		wantErr string
	}{
		{"ok", Snippet{ID: "review-pr", Name: "Review PR", Body: "Review the diff."}, ""},
		{"bad id", Snippet{ID: "Review PR", Name: "x", Body: "y"}, "slug"},
		{"empty id", Snippet{ID: "", Name: "x", Body: "y"}, "slug"},
		{"no name", Snippet{ID: "ok", Name: "  ", Body: "y"}, "name is required"},
		{"no body", Snippet{ID: "ok", Name: "x", Body: "  "}, "body is required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.snip.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate() = %v, want error containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestForgeSnippetCRUD(t *testing.T) {
	f := New(t.TempDir())

	// Add
	if err := f.AddSnippet(Snippet{ID: "explain", Name: "Explain", Body: "Explain this code."}); err != nil {
		t.Fatalf("AddSnippet: %v", err)
	}
	if got := f.Snippet("explain"); got == nil || got.Name != "Explain" {
		t.Fatalf("Snippet(explain) = %v", got)
	}
	// Duplicate id is rejected.
	if err := f.AddSnippet(Snippet{ID: "explain", Name: "Dup", Body: "x"}); err == nil {
		t.Fatal("AddSnippet duplicate: want error, got nil")
	}
	// An invalid snippet is rejected at add.
	if err := f.AddSnippet(Snippet{ID: "bad", Name: "", Body: "x"}); err == nil {
		t.Fatal("AddSnippet invalid: want error, got nil")
	}

	// Update
	if err := f.UpdateSnippet("explain", Snippet{ID: "explain", Name: "Explain code", Body: "Explain it well."}); err != nil {
		t.Fatalf("UpdateSnippet: %v", err)
	}
	if got := f.Snippet("explain"); got == nil || got.Body != "Explain it well." {
		t.Fatalf("after update Snippet(explain) = %v", got)
	}
	// Updating an unknown snippet fails.
	if err := f.UpdateSnippet("nope", Snippet{ID: "nope", Name: "x", Body: "y"}); err == nil {
		t.Fatal("UpdateSnippet unknown: want error, got nil")
	}
	// An invalid update rolls back to the prior value.
	if err := f.UpdateSnippet("explain", Snippet{ID: "explain", Name: "", Body: "y"}); err == nil {
		t.Fatal("UpdateSnippet invalid: want error, got nil")
	}
	if got := f.Snippet("explain"); got == nil || got.Name != "Explain code" {
		t.Fatalf("after rolled-back update Snippet(explain) = %v", got)
	}

	// Remove
	if err := f.RemoveSnippet("explain"); err != nil {
		t.Fatalf("RemoveSnippet: %v", err)
	}
	if f.Snippet("explain") != nil {
		t.Fatal("Snippet(explain) still present after remove")
	}
	if err := f.RemoveSnippet("explain"); err == nil {
		t.Fatal("RemoveSnippet unknown: want error, got nil")
	}
}

func TestForgeValidateRejectsDuplicateSnippet(t *testing.T) {
	f := New(t.TempDir())
	f.Snippets = []Snippet{
		{ID: "dup", Name: "A", Body: "a"},
		{ID: "dup", Name: "B", Body: "b"},
	}
	if err := f.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate snippet") {
		t.Fatalf("Validate() = %v, want duplicate snippet error", err)
	}
}

// Snippets persist round-trip and are omitted from JSON when empty (older
// forge.json files without a snippets key stay readable).
func TestForgeSnippetsPersistAndOmitEmpty(t *testing.T) {
	dir := t.TempDir()
	f := New(dir)
	if err := f.AddSnippet(Snippet{ID: "s1", Name: "One", Body: "first"}); err != nil {
		t.Fatalf("AddSnippet: %v", err)
	}
	if err := f.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s := got.Snippet("s1"); s == nil || s.Body != "first" {
		t.Fatalf("reloaded Snippet(s1) = %v", s)
	}

	// Empty snippets omit the key.
	empty := New(t.TempDir())
	if err := empty.AddSkill(Skill{ID: "x", Name: "X", Prompt: "p", Enabled: true}); err != nil {
		t.Fatalf("AddSkill: %v", err)
	}
	if err := empty.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(empty.Dir, forgeFile))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.Contains(string(data), "\"snippets\"") {
		t.Fatalf("forge.json contains snippets key when empty: %s", data)
	}
}
