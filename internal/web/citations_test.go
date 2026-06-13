// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package web

import (
	"reflect"
	"strings"
	"testing"
)

// Citation cards (R6, issue 0082, ADR-0049): reference-style definitions
// `[n]: url "title"` register a source; an inline `[n]` whose number has a
// matching definition becomes a citation marker linking to a server-rendered
// source card; a marker with no matching source degrades to plain escaped text.

func TestExtractCitations(t *testing.T) {
	cases := []struct {
		name      string
		in        []string
		wantLines []string
		wantSrc   []citeSource
	}{
		{
			"single def is removed, the marker line is kept",
			[]string{"see [1].", `[1]: https://go.dev "Go"`},
			[]string{"see [1]."},
			[]citeSource{{num: 1, url: "https://go.dev", title: "Go", origin: "go.dev"}},
		},
		{
			"a def between two soft-wrapped lines does not split them",
			[]string{"line one", `[1]: https://go.dev "G"`, "line two"},
			[]string{"line one", "line two"},
			[]citeSource{{num: 1, url: "https://go.dev", title: "G", origin: "go.dev"}},
		},
		{
			"a def-shaped line inside a code fence is verbatim, not a definition",
			[]string{"```", `[1]: https://e.com "E"`, "```", "use [1]"},
			[]string{"```", `[1]: https://e.com "E"`, "```", "use [1]"},
			nil,
		},
		{
			"missing title defaults to origin host",
			[]string{"[2]: https://example.com/docs"},
			[]string{},
			[]citeSource{{num: 2, url: "https://example.com/docs", title: "example.com", origin: "example.com"}},
		},
		{
			"sources sort by number regardless of definition order",
			[]string{`[3]: https://c.dev "C"`, `[1]: https://a.dev "A"`},
			[]string{},
			[]citeSource{
				{num: 1, url: "https://a.dev", title: "A", origin: "a.dev"},
				{num: 3, url: "https://c.dev", title: "C", origin: "c.dev"},
			},
		},
		{
			"duplicate number keeps the first, both lines removed",
			[]string{`[1]: https://a.dev "A"`, `[1]: https://b.dev "B"`},
			[]string{},
			[]citeSource{{num: 1, url: "https://a.dev", title: "A", origin: "a.dev"}},
		},
		{
			"unsafe scheme registers no source but the def line is still removed",
			[]string{`[1]: javascript:alert(1) "x"`},
			[]string{},
			nil,
		},
		{
			"relative url keeps the raw target as origin",
			[]string{`[1]: /local "Local"`},
			[]string{},
			[]citeSource{{num: 1, url: "/local", title: "Local", origin: "/local"}},
		},
		{
			"a non-definition line is left untouched",
			[]string{"just [1] a marker"},
			[]string{"just [1] a marker"},
			nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotLines, gotSrc := extractCitations(c.in)
			if !reflect.DeepEqual(gotLines, c.wantLines) {
				t.Errorf("lines\n  got:  %#v\n  want: %#v", gotLines, c.wantLines)
			}
			if !reflect.DeepEqual(gotSrc, c.wantSrc) {
				t.Errorf("sources\n  got:  %#v\n  want: %#v", gotSrc, c.wantSrc)
			}
		})
	}
}

func TestRenderMarkdownCitations(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			"marker resolves to a card and the sources block renders",
			"See Go[1].\n\n[1]: https://go.dev \"Go\"",
			`<p>See Go<sup class="cite"><a href="#cite-1">1</a>` +
				`<span class="cite-pop"><span class="cite-pop-title">Go</span>` +
				`<span class="cite-pop-origin">go.dev</span></span></sup>.</p>` +
				`<ol class="citations"><li id="cite-1" class="cite-card">` +
				`<a class="cite-link" href="https://go.dev">Go</a>` +
				`<span class="cite-origin">go.dev</span></li></ol>`,
		},
		{
			"a marker with no matching source degrades to escaped text",
			"Only one[1] but not[9].\n\n[1]: https://go.dev \"Go\"",
			`<p>Only one<sup class="cite"><a href="#cite-1">1</a>` +
				`<span class="cite-pop"><span class="cite-pop-title">Go</span>` +
				`<span class="cite-pop-origin">go.dev</span></span></sup> but not[9].</p>` +
				`<ol class="citations"><li id="cite-1" class="cite-card">` +
				`<a class="cite-link" href="https://go.dev">Go</a>` +
				`<span class="cite-origin">go.dev</span></li></ol>`,
		},
		{
			"no definitions: every marker stays literal and no sources block appears",
			"plain[1] text",
			"<p>plain[1] text</p>",
		},
		{
			"title carrying html is escaped in both the marker and the card",
			"x[1]\n\n[1]: https://e.com \"<b>bold</b>\"",
			`<p>x<sup class="cite"><a href="#cite-1">1</a>` +
				`<span class="cite-pop"><span class="cite-pop-title">&lt;b&gt;bold&lt;/b&gt;</span>` +
				`<span class="cite-pop-origin">e.com</span></span></sup></p>` +
				`<ol class="citations"><li id="cite-1" class="cite-card">` +
				`<a class="cite-link" href="https://e.com">&lt;b&gt;bold&lt;/b&gt;</a>` +
				`<span class="cite-origin">e.com</span></li></ol>`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := renderMarkdown(c.in); got != c.want {
				t.Errorf("renderMarkdown(%q)\n  got:  %q\n  want: %q", c.in, got, c.want)
			}
		})
	}
}

func TestRenderMarkdownCitationDefInCodeFence(t *testing.T) {
	// A definition-shaped line inside a fenced code block is verbatim, not a
	// citation: the code sample documenting the [n]: syntax survives intact, no
	// phantom source is registered, and the later [1] marker degrades to text.
	got := renderMarkdown("```\n[1]: https://e.com \"E\"\n```\n\nuse [1]")
	if !strings.Contains(got, "[1]: https://e.com") {
		t.Errorf("code-fenced definition line was dropped:\n  got: %q", got)
	}
	if strings.Contains(got, `class="citations"`) {
		t.Errorf("a phantom source card was rendered from a code-fenced def:\n  got: %q", got)
	}
	if !strings.Contains(got, "use [1]") || strings.Contains(got, `class="cite"`) {
		t.Errorf("the marker should degrade to literal text (no source):\n  got: %q", got)
	}
}
