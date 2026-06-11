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
		{"code escapes", []Block{codeBlock{lang: "", code: "<b>"}}, "<pre><code>&lt;b&gt;</code></pre>"},
		{"code lang escaped", []Block{codeBlock{lang: `x"y`, code: "c"}}, `<pre><code class="language-x&#34;y">c</code></pre>`},
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
