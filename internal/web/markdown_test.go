package web

import (
	"regexp"
	"strings"
	"testing"
)

// renderMarkdown renders a safe markdown subset (ADR 0001). These tests pin the
// supported elements and, above all, the escape-everything-first XSS guarantee.

func TestRenderMarkdown(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"blank", "   \n\t\n", ""},
		{"paragraph", "hello world", "<p>hello world</p>"},
		{"soft break", "line1\nline2", "<p>line1<br>line2</p>"},
		{"two paragraphs", "p1\n\np2", "<p>p1</p><p>p2</p>"},
		{"h1", "# Hi", "<h1>Hi</h1>"},
		{"h3", "### Three", "<h3>Three</h3>"},
		{"h6", "###### Six", "<h6>Six</h6>"},
		{"seven hashes is not a heading", "####### nope", "<p>####### nope</p>"},
		{"bold star", "**bold**", "<p><strong>bold</strong></p>"},
		{"bold underscore", "__bold__", "<p><strong>bold</strong></p>"},
		{"italic star", "*it*", "<p><em>it</em></p>"},
		{"mixed emphasis", "**a** and *b*", "<p><strong>a</strong> and <em>b</em></p>"},
		{"intraword underscore stays literal", "use my_var_name here", "<p>use my_var_name here</p>"},
		{"double intraword underscore stays literal", "a__b__c style", "<p>a__b__c style</p>"},
		{"dunder at word boundary is bold", "the __init__ method", "<p>the <strong>init</strong> method</p>"},
		{"underscore emphasis at boundary", "an _italic_ word", "<p>an <em>italic</em> word</p>"},
		{"inline code", "`code`", "<p><code>code</code></p>"},
		{"inline code escapes html", "`<script>`", "<p><code>&lt;script&gt;</code></p>"},
		{"emphasis ignored inside code", "`a**b**c`", "<p><code>a**b**c</code></p>"},
		{"ul dash", "- a\n- b", "<ul><li>a</li><li>b</li></ul>"},
		{"ul star", "* a\n* b", "<ul><li>a</li><li>b</li></ul>"},
		{"ol", "1. a\n2. b", "<ol><li>a</li><li>b</li></ol>"},
		{"blockquote", "> quoted", "<blockquote><p>quoted</p></blockquote>"},
		{"hr dashes", "---", "<hr>"},
		{"hr stars", "***", "<hr>"},
		{"link http", "[x](https://e.com)", `<p><a href="https://e.com">x</a></p>`},
		{"link relative", "[x](/local)", `<p><a href="/local">x</a></p>`},
		{"fenced code", "```go\nx := 1\n```", wantCodeBlock("go", "go", "x := 1")},
		{"fenced code no lang", "```\nplain\n```", wantCodeBlock("", "", "plain")},
		{"fenced code escapes html", "```\n<b>hi</b>\n```", wantCodeBlock("", "", "&lt;b&gt;hi&lt;/b&gt;")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderMarkdown(c.in)
			if got != c.want {
				t.Errorf("renderMarkdown(%q)\n  got:  %q\n  want: %q", c.in, got, c.want)
			}
		})
	}
}

func TestRenderMarkdownCallout(t *testing.T) {
	// GitHub-alert blockquotes render as designed callout components: a
	// kind-classed surface with a glyph + text label and a recursively-rendered
	// body. The kind class drives the token mapping in app.css.
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			"note default label",
			"> [!NOTE]\n> heads up",
			`<div class="callout callout-note"><div class="callout-head"><span class="callout-glyph" aria-hidden="true">ℹ</span><span class="callout-title">Note</span></div><div class="callout-body"><p>heads up</p></div></div>`,
		},
		{
			"warning custom title with emphasis in body",
			"> [!WARNING] Watch out\n> be **careful**",
			`<div class="callout callout-warning"><div class="callout-head"><span class="callout-glyph" aria-hidden="true">⚠</span><span class="callout-title">Watch out</span></div><div class="callout-body"><p>be <strong>careful</strong></p></div></div>`,
		},
		{
			"unknown kind degrades to blockquote",
			"> [!FOO]\n> x",
			"<blockquote><p>[!FOO]<br>x</p></blockquote>",
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

func TestRenderMarkdownTable(t *testing.T) {
	// GFM pipe tables render as a token-styled <table> component: a header row,
	// a delimiter row that sets per-column alignment, then body rows. Cells ride
	// the inline pass (escape-first). A pipe-bearing line not followed by a valid
	// delimiter degrades to plain text.
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			"aligned table with inline markup",
			"| Name | Qty |\n| --- | ---: |\n| **Apples** | 3 |",
			`<table class="md-table"><thead><tr><th>Name</th><th class="ta-right">Qty</th></tr></thead>` +
				`<tbody><tr><td><strong>Apples</strong></td><td class="ta-right">3</td></tr></tbody></table>`,
		},
		{
			"header only, empty body",
			"| a | b |\n| --- | --- |",
			`<table class="md-table"><thead><tr><th>a</th><th>b</th></tr></thead><tbody></tbody></table>`,
		},
		{
			"second line not a delimiter degrades to paragraph",
			"| a | b |\njust text",
			"<p>| a | b |<br>just text</p>",
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

func TestRenderMarkdownXSS(t *testing.T) {
	// Inputs that must never produce *live* markup. The invariant is structural:
	// no dangerous opening tag and no href with an executable scheme. Escaped text
	// like "&lt;img onerror=…&gt;" is inert and explicitly allowed.
	cases := []struct {
		name      string
		in        string
		forbidden []string // live-markup substrings (lowercased) that must be absent
	}{
		{"raw script tag", "<script>alert(1)</script>", []string{"<script", "</script>"}},
		{"img onerror", `<img src=x onerror=alert(1)>`, []string{"<img"}},
		{"js link", "[x](javascript:alert(1))", []string{`href="javascript`, `href='javascript`}},
		{"js link uppercase", "[x](JavaScript:alert(1))", []string{`href="javascript`}},
		{"data link", "[x](data:text/html,foo)", []string{`href="data:`}},
		{"vbscript link", "[x](vbscript:msgbox)", []string{`href="vbscript`}},
		{"html in code", "`<img src=x onerror=alert(1)>`", []string{"<img"}},
		{"html in fence", "```\n<img onerror=alert(1)>\n```", []string{"<img"}},
		{"callout title html", "> [!NOTE] <script>alert(1)</script>\n> hi", []string{"<script"}},
		{"callout body html", "> [!WARNING]\n> <img src=x onerror=alert(1)>", []string{"<img"}},
		{"table cell html", "| h |\n| --- |\n| <img src=x onerror=alert(1)> |", []string{"<img"}},
		{"table header html", "| <script>alert(1)</script> |\n| --- |\n| x |", []string{"<script"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderMarkdown(c.in)
			low := strings.ToLower(got)
			for _, f := range c.forbidden {
				if strings.Contains(low, f) {
					t.Errorf("renderMarkdown(%q) leaked live markup %q\n  got: %q", c.in, f, got)
				}
			}
			// Safe links degrade to escaped text, so a rejected scheme must leave
			// no anchor at all.
			if strings.HasPrefix(c.name, "js") || strings.HasPrefix(c.name, "data") || strings.HasPrefix(c.name, "vbscript") {
				if strings.Contains(low, "<a ") {
					t.Errorf("renderMarkdown(%q) produced an anchor for an unsafe scheme: %q", c.in, got)
				}
			}
		})
	}
}

// allowedTag matches an opening/closing/self-closing tag and captures its name.
var tagRe = regexp.MustCompile(`</?([a-zA-Z0-9]+)`)

var allowedTags = map[string]bool{
	"p": true, "br": true, "strong": true, "em": true, "code": true,
	"pre": true, "a": true, "ul": true, "ol": true, "li": true,
	"blockquote": true, "hr": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	// callout components (R2, issue 0078): the callout frag emits div/span with
	// fixed classes derived from a validated kind set — never attacker markup.
	"div": true, "span": true,
	// designed code blocks (R3, issue 0079): the codeBlock frag wraps <pre> in a
	// framed component whose head carries the language label + a copy <button>.
	"button": true,
	// designed tables (R4, issue 0080): the table frag emits a fixed table/section/
	// row/cell skeleton; cells carry only inline-escaped content + alignment classes.
	"table": true, "thead": true, "tbody": true, "tr": true, "th": true, "td": true,
}

// assertOnlyAllowedTags fails if the rendered HTML contains any tag outside the
// whitelist — the core XSS invariant the renderer must hold for ANY input.
func assertOnlyAllowedTags(t *testing.T, out string) {
	t.Helper()
	for _, m := range tagRe.FindAllStringSubmatch(out, -1) {
		name := strings.ToLower(m[1])
		if !allowedTags[name] {
			t.Errorf("disallowed tag <%s> in output: %q", name, out)
		}
	}
}

func TestRenderMarkdownTagWhitelist(t *testing.T) {
	for _, in := range []string{
		"<svg/onload=alert(1)>", "<a href=javascript:alert(1)>x</a>",
		"# <h1>nested</h1>", "**<b>x</b>**", "<iframe src=evil>",
	} {
		assertOnlyAllowedTags(t, renderMarkdown(in))
	}
}

// hrefRe captures every href value the renderer emits.
var hrefRe = regexp.MustCompile(`href="([^"]*)"`)

// FuzzRenderMarkdown asserts the core XSS invariants for arbitrary input: only
// whitelisted tags appear, and every emitted href uses a safe scheme.
func FuzzRenderMarkdown(f *testing.F) {
	for _, s := range []string{
		"", "# h", "**b**", "`c`", "- a\n- b", "> q", "---",
		"[x](javascript:alert(1))", "<script>x</script>", "```\n<b>\n```",
		"*\x00*", "[a](b)[c](d)", "###### \x00 # nested",
		"> [!NOTE]\n> body", "> [!WARNING] <script>\n> <img onerror=x>", "> [!FOO]\n> x",
		"| a | b |\n| --- | :-: |\n| 1 | 2 |", "| h |\n| --- |\n| <img onerror=x> |",
		"|\n|\n|", "a|b\n-|-\n*c*|`d`",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		out := renderMarkdown(src)
		assertOnlyAllowedTags(t, out)
		for _, m := range hrefRe.FindAllStringSubmatch(out, -1) {
			if !safeURL(m[1]) {
				t.Errorf("renderMarkdown(%q) emitted unsafe href %q", src, m[1])
			}
		}
		// The placeholder sentinel must never survive into output.
		if strings.Contains(out, "\x00") {
			t.Errorf("renderMarkdown(%q) leaked a NUL placeholder: %q", src, out)
		}
	})
}
