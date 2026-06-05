package web

import (
	"strconv"
	"strings"
)

// This file is the pure, deterministic core of the diff review lane (item 3.1):
// it turns a unified-diff string (the SDK's PermissionRequestWrite.Diff) into
// typed lines a template can render as an inline, side-numbered review. It has no
// IO and no browser dependency, so the parsing is unit-tested on its own; the
// rendering (render.go) and the escaping (html/template) sit on top of it.

// diffLineKind classifies one rendered line of a unified diff.
type diffLineKind int

const (
	diffContext diffLineKind = iota // unchanged line (leading space)
	diffAdd                         // added line (leading '+')
	diffDel                         // removed line (leading '-')
	diffHunk                        // @@ … @@ hunk header
	diffMeta                        // file header (--- / +++ / diff --git / index / no-newline)
)

// diffLine is one displayable line of a parsed unified diff. Text carries the
// line content with its leading marker stripped (for add/del/context) or the full
// header text (for hunk/meta); the renderer adds its own glyph and escapes Text.
// OldNum/NewNum are the 1-based source line numbers for the gutter, 0 when the
// line has no number on that side.
type diffLine struct {
	Kind   diffLineKind
	Text   string
	OldNum int
	NewNum int
}

// diffView is a parsed unified diff ready for rendering: its typed lines, the
// add/remove tallies for the header summary, and whether the input actually
// looked like a unified diff (OK). A non-diff string yields OK=false so the
// caller can fall back to a plain rendering.
type diffView struct {
	Lines []diffLine
	Adds  int
	Dels  int
	OK    bool
}

// parseUnifiedDiff parses a unified-diff string into a diffView. It is total and
// deterministic: any input yields a value, and OK reports whether at least one
// hunk header (@@ … @@) was found — the precise signal that distinguishes a real
// diff from prose that merely starts lines with '-' (e.g. a markdown bullet), so
// the review lane never hijacks an ordinary permission summary.
func parseUnifiedDiff(s string) diffView {
	var v diffView
	if s == "" {
		return v
	}
	oldNum, newNum := 0, 0
	// inHunk disambiguates the only context-sensitive lines in a unified diff:
	// `--- ` and `+++ ` are file headers *before* a hunk, but inside a hunk an
	// ordinary removed/added line whose content starts with "-- "/"++ " (a SQL,
	// Lua, or Haskell comment, say) renders the same three-char prefix. We treat
	// those as content once inside a hunk, matching how diff tools read them.
	inHunk := false
	for _, raw := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		switch {
		case isHunkHeader(raw):
			oldNum, newNum = hunkStarts(raw)
			inHunk = true
			v.Lines = append(v.Lines, diffLine{Kind: diffHunk, Text: raw})
			v.OK = true
		case isFileHeader(raw):
			// An unambiguous file/extended header (no leading +/-/space possible)
			// — and the start of a new file's header section, so leave the hunk.
			inHunk = false
			v.Lines = append(v.Lines, diffLine{Kind: diffMeta, Text: raw})
		case !inHunk && (strings.HasPrefix(raw, "--- ") || strings.HasPrefix(raw, "+++ ")):
			v.Lines = append(v.Lines, diffLine{Kind: diffMeta, Text: raw})
		case strings.HasPrefix(raw, `\ No newline`):
			v.Lines = append(v.Lines, diffLine{Kind: diffMeta, Text: raw})
		case strings.HasPrefix(raw, "+"):
			v.Lines = append(v.Lines, diffLine{Kind: diffAdd, Text: raw[1:], NewNum: newNum})
			newNum++
			v.Adds++
		case strings.HasPrefix(raw, "-"):
			v.Lines = append(v.Lines, diffLine{Kind: diffDel, Text: raw[1:], OldNum: oldNum})
			oldNum++
			v.Dels++
		case strings.HasPrefix(raw, " "):
			v.Lines = append(v.Lines, diffLine{Kind: diffContext, Text: raw[1:], OldNum: oldNum, NewNum: newNum})
			oldNum++
			newNum++
		default:
			// An unmarked line (e.g. a blank separator); treat as context so the
			// gutter stays aligned without inventing a marker.
			v.Lines = append(v.Lines, diffLine{Kind: diffContext, Text: raw, OldNum: oldNum, NewNum: newNum})
			oldNum++
			newNum++
		}
	}
	return v
}

// isHunkHeader reports whether a line is a unified-diff hunk header
// (`@@ -a,b +c,d @@ optional`). It requires the opening and a second `@@` so a
// line that merely starts with "@@" isn't misread.
func isHunkHeader(s string) bool {
	if !strings.HasPrefix(s, "@@ ") {
		return false
	}
	return strings.Contains(s[2:], "@@")
}

// hunkStarts extracts the 1-based old/new start line numbers from a hunk header.
// `@@ -oldStart[,oldCount] +newStart[,newCount] @@` — counts are optional. On any
// parse miss it returns the best it found (0 means unknown), staying total.
func hunkStarts(s string) (oldStart, newStart int) {
	body := s[len("@@ "):]
	if i := strings.Index(body, " @@"); i >= 0 {
		body = body[:i]
	}
	for _, tok := range strings.Fields(body) {
		switch {
		case strings.HasPrefix(tok, "-"):
			oldStart = leadingInt(tok[1:])
		case strings.HasPrefix(tok, "+"):
			newStart = leadingInt(tok[1:])
		}
	}
	return oldStart, newStart
}

// leadingInt parses the integer before an optional ",count" suffix.
func leadingInt(s string) int {
	if i := strings.IndexByte(s, ','); i >= 0 {
		s = s[:i]
	}
	n, _ := strconv.Atoi(s)
	return n
}

// isFileHeader reports whether a line is unambiguous unified-diff plumbing — a
// git extended header that can never carry a +/-/space content marker, so it is
// safe to classify as meta regardless of hunk position. The context-sensitive
// `--- `/`+++ ` headers are handled separately (only before a hunk), and the
// "no newline" marker likewise, since those forms can collide with content.
func isFileHeader(s string) bool {
	switch {
	case strings.HasPrefix(s, "diff --git "), strings.HasPrefix(s, "index "):
		return true
	case strings.HasPrefix(s, "new file mode "), strings.HasPrefix(s, "deleted file mode "):
		return true
	case strings.HasPrefix(s, "rename "), strings.HasPrefix(s, "similarity index "):
		return true
	}
	return false
}
