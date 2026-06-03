package web

import (
	"strings"
	"testing"

	"github.com/dotts-h/copilot-sdk/internal/convo"
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
	html := renderPermForm("p1", "run shell: rm <x>")
	if strings.Contains(html, "<x>") {
		t.Errorf("perm detail not escaped: %q", html)
	}
	for _, sub := range []string{`id="perm-p1"`, `hx-post="/perm/p1"`, `value="1"`, `value="0"`} {
		if !strings.Contains(html, sub) {
			t.Errorf("perm form missing %q: %q", sub, html)
		}
	}
}

func TestClampLines(t *testing.T) {
	in := strings.Repeat("x\n", 20)
	out := clampLines(in, 5)
	if strings.Count(out, "\n") > 6 || !strings.Contains(out, "more lines") {
		t.Errorf("clampLines did not bound: %q", out)
	}
}
