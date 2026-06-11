package web

import (
	"reflect"
	"strings"
	"testing"
)

// Container directives (R5, issue 0081, ADR-0047): an in-house `:::name{attrs}`
// … `:::` block resolved against a closed Go registry. The registry — not the
// parser — is the trust boundary: a name the registry does not hold never
// produces directive markup (it degrades to ordinary escaped text).

func TestParseBlocksDirective(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []Block
	}{
		{
			"card with body",
			":::card\nhello **world**\n:::",
			[]Block{directiveBlock{
				name:     "card",
				attrs:    map[string]string{},
				children: []Block{paragraphBlock{lines: []string{"hello **world**"}}},
			}},
		},
		{
			// Unquoted values are a single space-delimited token (the normative
			// grammar): `summary=Setup` → "Setup". Multi-word needs quotes.
			"details with single-token summary attr",
			":::details{summary=Setup}\nstep one\n:::",
			[]Block{directiveBlock{
				name:     "details",
				attrs:    map[string]string{"summary": "Setup"},
				children: []Block{paragraphBlock{lines: []string{"step one"}}},
			}},
		},
		{
			"details with quoted multi-word summary",
			`:::details{summary="Build steps"}` + "\nbody\n:::",
			[]Block{directiveBlock{
				name:     "details",
				attrs:    map[string]string{"summary": "Build steps"},
				children: []Block{paragraphBlock{lines: []string{"body"}}},
			}},
		},
		{
			"recursive body parses as blocks",
			":::card\n# Title\n- a\n- b\n:::",
			[]Block{directiveBlock{
				name:  "card",
				attrs: map[string]string{},
				children: []Block{
					headingBlock{level: 1, text: "Title"},
					listBlock{ordered: false, items: []string{"a", "b"}},
				},
			}},
		},
		{
			"nested directives",
			":::card\n:::details{summary=Inner}\nx\n:::\n:::",
			[]Block{directiveBlock{
				name:  "card",
				attrs: map[string]string{},
				children: []Block{directiveBlock{
					name:     "details",
					attrs:    map[string]string{"summary": "Inner"},
					children: []Block{paragraphBlock{lines: []string{"x"}}},
				}},
			}},
		},
		{
			"unregistered name is not a directive",
			":::mystery\nx\n:::",
			[]Block{paragraphBlock{lines: []string{":::mystery", "x", ":::"}}},
		},
		{
			// An unterminated open degrades: the marker line becomes its own
			// escaped-text block and the body after it parses as normal blocks.
			"unterminated directive degrades to text",
			":::card\nhello",
			[]Block{
				paragraphBlock{lines: []string{":::card"}},
				paragraphBlock{lines: []string{"hello"}},
			},
		},
		{
			// A `:::` line inside a fenced code block in the body is verbatim code,
			// not the container's close — the card holds the whole code block.
			"code fence in body is opaque to the close-scan",
			":::card\n```\n:::\n```\n:::",
			[]Block{directiveBlock{
				name:     "card",
				attrs:    map[string]string{},
				children: []Block{codeBlock{lang: "", code: ":::"}},
			}},
		},
		{
			"directive after a paragraph breaks it",
			"intro\n:::card\nbody\n:::",
			[]Block{
				paragraphBlock{lines: []string{"intro"}},
				directiveBlock{name: "card", attrs: map[string]string{}, children: []Block{paragraphBlock{lines: []string{"body"}}}},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseBlocks(c.src); !reflect.DeepEqual(got, c.want) {
				t.Errorf("parseBlocks(%q)\n  got:  %#v\n  want: %#v", c.src, got, c.want)
			}
		})
	}
}

// wantDirCard / wantDirDetails build the expected designed-directive HTML so the
// golden cases read against the template, not a hand-copied string.
func wantDirCard(body string) string {
	return `<div class="directive dir-card">` + body + `</div>`
}

func wantDirDetails(summary, body string) string {
	return `<details class="directive dir-details"><summary class="dir-summary">` + summary +
		`</summary><div class="dir-body">` + body + `</div></details>`
}

func TestRenderDirectiveDesigned(t *testing.T) {
	cases := []struct {
		name string
		blk  directiveBlock
		want string
	}{
		{
			"card body",
			directiveBlock{name: "card", attrs: map[string]string{}, children: []Block{paragraphBlock{lines: []string{"hi"}}}},
			wantDirCard("<p>hi</p>"),
		},
		{
			"details with summary",
			directiveBlock{name: "details", attrs: map[string]string{"summary": "More"}, children: []Block{paragraphBlock{lines: []string{"x"}}}},
			wantDirDetails("More", "<p>x</p>"),
		},
		{
			"details default summary when absent",
			directiveBlock{name: "details", attrs: map[string]string{}, children: []Block{paragraphBlock{lines: []string{"x"}}}},
			wantDirDetails("Details", "<p>x</p>"),
		},
		{
			"details summary rides the inline escape pass",
			directiveBlock{name: "details", attrs: map[string]string{"summary": "<img onerror=x>"}, children: nil},
			wantDirDetails("&lt;img onerror=x&gt;", ""),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := renderBlocks([]Block{c.blk}); got != c.want {
				t.Errorf("renderBlocks(%#v)\n  got:  %q\n  want: %q", c.blk, got, c.want)
			}
		})
	}
}

func TestRenderMarkdownDirective(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			"card end to end",
			":::card\nhello\n:::",
			wantDirCard("<p>hello</p>"),
		},
		{
			"details with attr end to end",
			":::details{summary=Notes}\n- one\n- two\n:::",
			wantDirDetails("Notes", "<ul><li>one</li><li>two</li></ul>"),
		},
		{
			"unregistered degrades to escaped text",
			":::mystery\nx\n:::",
			"<p>:::mystery<br>x<br>:::</p>",
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

// TestParseDirectiveAttrs pins the escape-first, bounded attribute grammar:
// space-separated key=value / key="quoted value" pairs into a map; an
// over-long brace span is dropped wholesale (the weaponization guard).
func TestParseDirectiveAttrs(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want map[string]string
	}{
		{"empty", "", map[string]string{}},
		{"single", "summary=Hi", map[string]string{"summary": "Hi"}},
		{"quoted with spaces", `summary="Build steps"`, map[string]string{"summary": "Build steps"}},
		{"two pairs", `a=1 b=2`, map[string]string{"a": "1", "b": "2"}},
		{"bare key dropped", "summary", map[string]string{}},
		{"oversized span dropped", "summary=" + strings.Repeat("x", 512), map[string]string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseDirectiveAttrs(c.in); !reflect.DeepEqual(got, c.want) {
				t.Errorf("parseDirectiveAttrs(%q)\n  got:  %#v\n  want: %#v", c.in, got, c.want)
			}
		})
	}
}
