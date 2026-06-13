// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package web

import "testing"

func TestParseUnifiedDiff(t *testing.T) {
	in := "--- a/foo.go\n" +
		"+++ b/foo.go\n" +
		"@@ -1,3 +1,4 @@ func main()\n" +
		" ctx line\n" +
		"-old line\n" +
		"+new line one\n" +
		"+new line two\n" +
		" trailing ctx\n"
	v := parseUnifiedDiff(in)
	if !v.OK {
		t.Fatalf("expected OK diff, got %+v", v)
	}
	if v.Adds != 2 || v.Dels != 1 {
		t.Errorf("tallies wrong: adds=%d dels=%d, want 2/1", v.Adds, v.Dels)
	}
	// Kinds in order: meta, meta, hunk, context, del, add, add, context.
	wantKinds := []diffLineKind{diffMeta, diffMeta, diffHunk, diffContext, diffDel, diffAdd, diffAdd, diffContext}
	if len(v.Lines) != len(wantKinds) {
		t.Fatalf("line count = %d, want %d: %+v", len(v.Lines), len(wantKinds), v.Lines)
	}
	for i, k := range wantKinds {
		if v.Lines[i].Kind != k {
			t.Errorf("line %d kind = %v, want %v (text %q)", i, v.Lines[i].Kind, k, v.Lines[i].Text)
		}
	}
	// Markers are stripped from the rendered text.
	if v.Lines[4].Text != "old line" {
		t.Errorf("del text not stripped: %q", v.Lines[4].Text)
	}
	if v.Lines[5].Text != "new line one" {
		t.Errorf("add text not stripped: %q", v.Lines[5].Text)
	}
}

func TestParseUnifiedDiffNumbersGutter(t *testing.T) {
	in := "@@ -10,3 +20,3 @@\n" +
		" keep\n" +
		"-drop\n" +
		"+gain\n" +
		" keep2\n"
	v := parseUnifiedDiff(in)
	// line index 1 = " keep": old 10, new 20.
	keep := v.Lines[1]
	if keep.OldNum != 10 || keep.NewNum != 20 {
		t.Errorf("context numbers = %d/%d, want 10/20", keep.OldNum, keep.NewNum)
	}
	// " -drop": old 11, no new.
	drop := v.Lines[2]
	if drop.OldNum != 11 || drop.NewNum != 0 {
		t.Errorf("del numbers = %d/%d, want 11/0", drop.OldNum, drop.NewNum)
	}
	// "+gain": new 21, no old.
	gain := v.Lines[3]
	if gain.OldNum != 0 || gain.NewNum != 21 {
		t.Errorf("add numbers = %d/%d, want 0/21", gain.OldNum, gain.NewNum)
	}
	// " keep2": old advanced past the deletion (12), new 22.
	keep2 := v.Lines[4]
	if keep2.OldNum != 12 || keep2.NewNum != 22 {
		t.Errorf("trailing context numbers = %d/%d, want 12/22", keep2.OldNum, keep2.NewNum)
	}
}

func TestParseUnifiedDiffNewFile(t *testing.T) {
	in := "@@ -0,0 +1,2 @@\n+line a\n+line b\n"
	v := parseUnifiedDiff(in)
	if !v.OK || v.Adds != 2 || v.Dels != 0 {
		t.Errorf("new-file diff wrong: ok=%v adds=%d dels=%d", v.OK, v.Adds, v.Dels)
	}
}

func TestParseUnifiedDiffRejectsNonDiff(t *testing.T) {
	for _, s := range []string{
		"",
		"just some prose",
		"- a markdown bullet\n- another bullet", // leading '-' but no hunk header
		"hello: world",
	} {
		if v := parseUnifiedDiff(s); v.OK {
			t.Errorf("expected non-diff for %q, got OK with %d lines", s, len(v.Lines))
		}
	}
}

func TestParseUnifiedDiffCommentMarkerInsideHunk(t *testing.T) {
	// Inside a hunk, a removed/added line whose own content starts with "-- "/"++ "
	// (a SQL/Lua/Haskell comment, say) renders as "--- …"/"+++ …" — it must be
	// counted as a deletion/addition, not misread as a file header.
	in := "--- a/q.sql\n" +
		"+++ b/q.sql\n" +
		"@@ -1,2 +1,2 @@\n" +
		"--- drop old table\n" + // del of the comment line "-- drop old table"
		"+++ create new table\n" + // add of the comment line "++ create new table"
		" SELECT 1;\n"
	v := parseUnifiedDiff(in)
	if v.Adds != 1 || v.Dels != 1 {
		t.Fatalf("comment-marker content miscounted: adds=%d dels=%d, want 1/1\n%+v", v.Adds, v.Dels, v.Lines)
	}
	// The two leading lines are still the file header (before the hunk).
	if v.Lines[0].Kind != diffMeta || v.Lines[1].Kind != diffMeta {
		t.Errorf("pre-hunk --- / +++ should be meta: %+v", v.Lines[:2])
	}
	// The in-hunk lines are del/add with their markers stripped.
	del, add := v.Lines[3], v.Lines[4]
	if del.Kind != diffDel || del.Text != "-- drop old table" {
		t.Errorf("in-hunk del wrong: kind=%v text=%q", del.Kind, del.Text)
	}
	if add.Kind != diffAdd || add.Text != "++ create new table" {
		t.Errorf("in-hunk add wrong: kind=%v text=%q", add.Kind, add.Text)
	}
}

func TestParseUnifiedDiffMultipleHunks(t *testing.T) {
	in := "@@ -1,1 +1,1 @@\n-a\n+b\n@@ -10,1 +10,1 @@\n-c\n+d\n"
	v := parseUnifiedDiff(in)
	if !v.OK || v.Adds != 2 || v.Dels != 2 {
		t.Errorf("multi-hunk wrong: ok=%v adds=%d dels=%d", v.OK, v.Adds, v.Dels)
	}
	hunks := 0
	for _, l := range v.Lines {
		if l.Kind == diffHunk {
			hunks++
		}
	}
	if hunks != 2 {
		t.Errorf("expected 2 hunk headers, got %d", hunks)
	}
}
