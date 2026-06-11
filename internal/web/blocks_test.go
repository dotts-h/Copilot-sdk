package web

import (
	"reflect"
	"testing"
)

// The block-AST seam (ADR 0045): parseBlocks recognizes the markdown subset as a
// typed []Block, renderBlocks emits the HTML, and renderMarkdown is exactly their
// composition — byte-identical to the pre-seam renderer.

func TestParseBlocksStructure(t *testing.T) {
	src := "# Title\n" +
		"intro line1\nline2\n" +
		"\n" +
		"```go\nx := 1\n```\n" +
		"- a\n- b\n" +
		"1. one\n" +
		"> quoted\n" +
		"---"
	got := parseBlocks(src)
	want := []Block{
		headingBlock{level: 1, text: "Title"},
		paragraphBlock{lines: []string{"intro line1", "line2"}},
		codeBlock{lang: "go", code: "x := 1"},
		listBlock{ordered: false, items: []string{"a", "b"}},
		listBlock{ordered: true, items: []string{"one"}},
		quoteBlock{children: []Block{paragraphBlock{lines: []string{"quoted"}}}},
		hrBlock{},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseBlocks(%q)\n  got:  %#v\n  want: %#v", src, got, want)
	}
}

func TestParseBlocksCallout(t *testing.T) {
	// A blockquote whose first line is a known [!KIND] marker becomes a callout
	// block; the remaining lines are the recursively-parsed body. An optional
	// trailing title overrides the default per-kind label.
	cases := []struct {
		name string
		src  string
		want []Block
	}{
		{
			"default title",
			"> [!NOTE]\n> heads up",
			[]Block{calloutBlock{kind: "note", title: "Note", children: []Block{paragraphBlock{lines: []string{"heads up"}}}}},
		},
		{
			"custom title",
			"> [!WARNING] Watch out\n> danger",
			[]Block{calloutBlock{kind: "warning", title: "Watch out", children: []Block{paragraphBlock{lines: []string{"danger"}}}}},
		},
		{
			"case-insensitive marker",
			"> [!tip]\n> do this",
			[]Block{calloutBlock{kind: "tip", title: "Tip", children: []Block{paragraphBlock{lines: []string{"do this"}}}}},
		},
		{
			"unknown kind degrades to quote",
			"> [!FOO]\n> x",
			[]Block{quoteBlock{children: []Block{paragraphBlock{lines: []string{"[!FOO]", "x"}}}}},
		},
		{
			"plain quote unaffected",
			"> just a quote",
			[]Block{quoteBlock{children: []Block{paragraphBlock{lines: []string{"just a quote"}}}}},
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

func TestParseBlocksNestedQuote(t *testing.T) {
	got := parseBlocks("> # h\n> - x")
	want := []Block{
		quoteBlock{children: []Block{
			headingBlock{level: 1, text: "h"},
			listBlock{ordered: false, items: []string{"x"}},
		}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseBlocks nested quote\n  got:  %#v\n  want: %#v", got, want)
	}
}

func TestRenderBlocks(t *testing.T) {
	cases := []struct {
		name   string
		blocks []Block
		want   string
	}{
		{"heading", []Block{headingBlock{level: 2, text: "**hi**"}}, "<h2><strong>hi</strong></h2>"},
		{"code escapes", []Block{codeBlock{lang: "", code: "<b>"}}, wantCodeBlock("", "", "&lt;b&gt;")},
		{"code lang escaped", []Block{codeBlock{lang: `x"y`, code: "c"}}, wantCodeBlock(`x&#34;y`, `x&#34;y`, "c")},
		{"paragraph soft break", []Block{paragraphBlock{lines: []string{"a", "b"}}}, "<p>a<br>b</p>"},
		{"ordered list", []Block{listBlock{ordered: true, items: []string{"a"}}}, "<ol><li>a</li></ol>"},
		{"quote wraps children", []Block{quoteBlock{children: []Block{hrBlock{}}}}, "<blockquote><hr></blockquote>"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := renderBlocks(c.blocks); got != c.want {
				t.Errorf("renderBlocks(%#v)\n  got:  %q\n  want: %q", c.blocks, got, c.want)
			}
		})
	}
}

// wantCodeBlock builds the expected designed-code-block HTML (R3, issue 0079):
// the token-styled frame with a language label, a progressive copy affordance,
// and the escape-first <pre><code> body. langAttr is the escaped language hint
// for the `language-*` class (empty → no class), langLabel its escaped visible
// label, and code the already-escaped source.
func wantCodeBlock(langAttr, langLabel, code string) string {
	cls := ""
	if langAttr != "" {
		cls = ` class="language-` + langAttr + `"`
	}
	return `<div class="code-block"><div class="code-head"><span class="code-lang">` + langLabel +
		`</span><button type="button" class="code-copy" onclick="copyCode(this)" aria-label="Copy code">` +
		`<span class="code-copy-label">Copy</span></button></div><pre><code` + cls + `>` + code + `</code></pre></div>`
}

// TestRenderCodeBlockDesigned pins the R3 frame: a code block renders through the
// codeBlock frag as a designed component (header with language label + copy
// button, token-styled body), with the code escaped before it rides into the
// template — never hand-built <pre> string concat.
func TestRenderCodeBlockDesigned(t *testing.T) {
	cases := []struct {
		name string
		blk  codeBlock
		want string
	}{
		{"with language", codeBlock{lang: "go", code: "x := 1"}, wantCodeBlock("go", "go", "x := 1")},
		{"no language", codeBlock{lang: "", code: "plain"}, wantCodeBlock("", "", "plain")},
		{"body escaped", codeBlock{lang: "html", code: "<img onerror=x>"}, wantCodeBlock("html", "html", "&lt;img onerror=x&gt;")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := renderBlocks([]Block{c.blk}); got != c.want {
				t.Errorf("renderBlocks(%#v)\n  got:  %q\n  want: %q", c.blk, got, c.want)
			}
		})
	}
}

// TestRenderMarkdownIsComposition pins renderMarkdown to the seam: it must be
// byte-for-byte renderBlocks∘parseBlocks (after sentinel stripping, which the
// corpus below doesn't contain).
func TestRenderMarkdownIsComposition(t *testing.T) {
	for _, src := range []string{
		"",
		"# h\n\npara *em* `code` [x](/l)\n\n- a\n- b\n\n> q\n> > deep\n\n---\n\n```go\nx\n```",
		"text\n####### not a heading\n1. one",
	} {
		if got, want := renderMarkdown(src), renderBlocks(parseBlocks(src)); got != want {
			t.Errorf("renderMarkdown(%q) = %q, want composition output %q", src, got, want)
		}
	}
}
