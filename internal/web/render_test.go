package web

import (
	"strconv"
	"strings"
	"testing"

	"github.com/dotts-h/copilot-sdk/internal/convo"
	"github.com/dotts-h/copilot-sdk/internal/copilot"
)

func TestEscNewlinesAndEscaping(t *testing.T) {
	got := esc("line1\nline2 <x>")
	want := "line1<br>line2 &lt;x&gt;"
	if got != want {
		t.Errorf("esc = %q, want %q", got, want)
	}
}

func TestRenderTurnEscapes(t *testing.T) {
	cases := map[convo.Role][]string{
		convo.RoleUser:      {`class="turn user"`, "you"},
		convo.RoleAgent:     {`class="turn assistant"`, "orchestra"},
		convo.RoleReasoning: {`<details class="turn reasoning"`, "thinking"},
		convo.RoleSystem:    {`class="turn system"`},
	}
	for role, subs := range cases {
		html := renderTurn(convo.Turn{Role: role, Text: "hi <script>"})
		if strings.Contains(html, "<script>") {
			t.Errorf("role %d not escaped: %q", role, html)
		}
		for _, sub := range subs {
			if !strings.Contains(html, sub) {
				t.Errorf("role %d missing %q in %q", role, sub, html)
			}
		}
	}
}

func TestRenderToolCardStates(t *testing.T) {
	running := renderToolCard(&convo.ToolView{ID: "t1", Name: "bash", Args: "ls", Progress: "go"})
	if !strings.Contains(running, `id="tool-t1"`) || !strings.Contains(running, "running") ||
		!strings.Contains(running, "tool-progress") {
		t.Errorf("running card wrong: %q", running)
	}
	done := renderToolCard(&convo.ToolView{ID: "t1", Name: "bash", Done: true, Result: "ok"})
	if !strings.Contains(done, "done") || !strings.Contains(done, "✓") || !strings.Contains(done, "ok") {
		t.Errorf("done card wrong: %q", done)
	}
	failed := renderToolCard(&convo.ToolView{ID: "t1", Name: "bash", Done: true, Failed: true})
	if !strings.Contains(failed, "failed") || !strings.Contains(failed, "✗") {
		t.Errorf("failed card wrong: %q", failed)
	}
}

func TestRenderTimelineInnerHasCur(t *testing.T) {
	var st convo.State
	st.AddUser("hi")
	st.AppendDelta("partial answer")
	html := renderTimelineInner(&st)
	if !strings.Contains(html, `class="turn user"`) {
		t.Errorf("missing committed user turn: %q", html)
	}
	if !strings.Contains(html, `id="cur"`) || !strings.Contains(html, "partial answer") {
		t.Errorf("missing live #cur: %q", html)
	}
}

func TestRenderPermFormEscapes(t *testing.T) {
	html := renderPermForm(copilot.PermissionRequest{ID: "p1", Detail: "run shell: rm <x>"})
	if strings.Contains(html, "<x>") {
		t.Errorf("perm detail not escaped: %q", html)
	}
	for _, sub := range []string{`id="perm-p1"`, `hx-post="/perm/p1"`, `value="1"`, `value="0"`} {
		if !strings.Contains(html, sub) {
			t.Errorf("perm form missing %q: %q", sub, html)
		}
	}
	// A non-write request renders the compact form, not the review lane.
	if strings.Contains(html, "perm-review") || strings.Contains(html, "diff-line") {
		t.Errorf("non-write perm should not render a diff review lane: %q", html)
	}
}

func TestRenderPermFormReviewLane(t *testing.T) {
	req := copilot.PermissionRequest{
		ID: "p2", Kind: "write", Detail: "write file: app.go",
		FileName: "app.go", Intention: "add a guard",
		Diff: "@@ -1,2 +1,2 @@\n ctx\n-secret = \"<old>\"\n+secret = \"<new>\"\n",
	}
	html := renderPermForm(req)
	// Posts the decision through the existing /perm flow (no new route).
	for _, sub := range []string{`id="perm-p2"`, `hx-post="/perm/p2"`, "perm-review", "app.go", "add a guard", `name="approve"`} {
		if !strings.Contains(html, sub) {
			t.Errorf("review lane missing %q: %q", sub, html)
		}
	}
	// The diffstat summarizes the change.
	if !strings.Contains(html, "+1") || !strings.Contains(html, "−1") {
		t.Errorf("review lane missing diffstat: %q", html)
	}
	// Untrusted file content in the diff must be HTML-escaped (ADR-0001).
	if strings.Contains(html, "<old>") || strings.Contains(html, "<new>") {
		t.Errorf("diff content not escaped: %q", html)
	}
	if !strings.Contains(html, "&lt;old&gt;") {
		t.Errorf("expected escaped diff content: %q", html)
	}
	// Add/remove lines are classed for styling.
	if !strings.Contains(html, "diff-add") || !strings.Contains(html, "diff-del") {
		t.Errorf("review lane missing add/del line classes: %q", html)
	}
	// Screen readers get a non-color, textual add/remove cue (the +/- marker is
	// aria-hidden), so the diff's meaning isn't conveyed by color alone.
	for _, sub := range []string{`class="sr-only">added `, `class="sr-only">removed `} {
		if !strings.Contains(html, sub) {
			t.Errorf("review lane missing screen-reader label %q: %q", sub, html)
		}
	}
}

func TestDiffLineHelpers(t *testing.T) {
	// diffMarker/diffClass/diffLabel must omit a marker/label on hunk & meta rows
	// so headers don't render spurious +/- glyphs, and blank an absent gutter.
	cases := []struct {
		k                    diffLineKind
		marker, class, label string
	}{
		{diffContext, " ", "diff-context", ""},
		{diffAdd, "+", "diff-add", "added "},
		{diffDel, "-", "diff-del", "removed "},
		{diffHunk, "", "diff-hunk", ""},
		{diffMeta, "", "diff-meta", ""},
	}
	for _, c := range cases {
		if got := diffMarker(c.k); got != c.marker {
			t.Errorf("diffMarker(%v) = %q, want %q", c.k, got, c.marker)
		}
		if got := diffClass(c.k); got != c.class {
			t.Errorf("diffClass(%v) = %q, want %q", c.k, got, c.class)
		}
		if got := diffLabel(c.k); got != c.label {
			t.Errorf("diffLabel(%v) = %q, want %q", c.k, got, c.label)
		}
	}
	if gutterNum(0) != "" || gutterNum(-1) != "" || gutterNum(7) != "7" {
		t.Errorf("gutterNum: 0/-1 should blank, 7 should render; got %q/%q/%q",
			gutterNum(0), gutterNum(-1), gutterNum(7))
	}
}

func TestRenderPermFormBoundsHugeDiff(t *testing.T) {
	var b strings.Builder
	b.WriteString("@@ -1," + strconv.Itoa(maxDiffLines+50) + " +1,1 @@\n")
	for i := 0; i < maxDiffLines+50; i++ {
		b.WriteString("-line\n")
	}
	html := renderPermForm(copilot.PermissionRequest{ID: "big", FileName: "x", Diff: b.String()})
	if !strings.Contains(html, "more lines") {
		t.Errorf("expected an elision note for an oversized diff: %q", html[:min(len(html), 400)])
	}
	// The rendered rows are bounded (hunk header + maxDiffLines content + the
	// elision row), not the full input.
	if got := strings.Count(html, "diff-line"); got > maxDiffLines+2 {
		t.Errorf("diff not bounded: %d diff-line rows", got)
	}
}

func TestRenderPermFormWriteWithoutDiffStaysCompact(t *testing.T) {
	// A write request that somehow lacks a parseable diff falls back to the
	// compact form rather than rendering an empty review lane.
	req := copilot.PermissionRequest{ID: "p3", Kind: "write", Detail: "write file: x", FileName: "x"}
	html := renderPermForm(req)
	if strings.Contains(html, "perm-review") {
		t.Errorf("diffless write should stay compact: %q", html)
	}
}

func TestClampLines(t *testing.T) {
	in := strings.Repeat("x\n", 20)
	out := clampLines(in, 5)
	if strings.Count(out, "\n") > 6 || !strings.Contains(out, "more lines") {
		t.Errorf("clampLines did not bound: %q", out)
	}
}
