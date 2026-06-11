package web

import (
	"html"
	"regexp"
	"strconv"
	"strings"
)

// This file renders a deliberately small, XSS-safe markdown subset for committed
// agent turns (ADR 0001). The contract: every byte of input is HTML-escaped
// first, then a fixed whitelist of structural tags is layered on top, so output
// can never contain attacker-controlled markup. Anything outside the subset
// degrades to escaped plain text. Streaming text and all other roles keep using
// richtext (escape + <br>); only turnAgent calls this.

var (
	mdHeading    = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	mdULItem     = regexp.MustCompile(`^\s*[-*+]\s+(.*)$`)
	mdOLItem     = regexp.MustCompile(`^\s*\d+\.\s+(.*)$`)
	mdInlineCode = regexp.MustCompile("`([^`]+)`")
	mdLink       = regexp.MustCompile(`\[([^\]]*)\]\(([^)\s]+)\)`)
	// Underscore emphasis requires word boundaries (\b) so intraword underscores —
	// snake_case identifiers in prose — are left literal, matching CommonMark.
	// Asterisk emphasis stays intraword-capable.
	mdBold   = regexp.MustCompile(`\*\*([^*]+)\*\*|\b__([^_]+)__\b`)
	mdItalic = regexp.MustCompile(`\*([^*]+)\*|\b_([^_]+)_\b`)
)

// Block is one structural element of the markdown subset (ADR 0045). The
// interface is sealed: every implementation lives in this file, and each
// renderer may emit only whitelisted or templated HTML, with every dynamic
// string escape-first via inline()/html.EscapeString.
type Block interface {
	renderTo(b *strings.Builder)
}

// headingBlock is an ATX heading; text is raw markdown for the inline pass.
type headingBlock struct {
	level int
	text  string
}

// codeBlock is a fenced code block; code is rendered verbatim (escaped, no
// inline pass), lang becomes the language-* class when present.
type codeBlock struct {
	lang string
	code string
}

// listBlock is a run of consecutive list items; items are raw markdown.
type listBlock struct {
	ordered bool
	items   []string
}

// quoteBlock is a blockquote; its inner lines are parsed recursively, so the
// AST is a tree.
type quoteBlock struct {
	children []Block
}

// hrBlock is a horizontal rule.
type hrBlock struct{}

// paragraphBlock is consecutive non-blank, non-block lines joined by soft
// breaks; lines are raw markdown for the inline pass.
type paragraphBlock struct {
	lines []string
}

// renderMarkdown turns a markdown subset into sanitized HTML. The output is a
// trusted HTML string (every dynamic part is escaped before assembly). It is
// exactly the composition of the block-AST seam: parse, then render.
func renderMarkdown(src string) string {
	// Strip the placeholder sentinel so input can never forge a placeholder.
	src = strings.ReplaceAll(src, "\x00", "")
	return renderBlocks(parseBlocks(src))
}

// parseBlocks recognizes the markdown subset as a typed block list. No HTML is
// produced here; raw markdown text rides inside the blocks for the renderers.
func parseBlocks(src string) []Block {
	lines := strings.Split(src, "\n")

	var blocks []Block
	i := 0
	for i < len(lines) {
		line := lines[i]

		// Fenced code block: ``` … ``` — content kept verbatim.
		if fence := strings.TrimSpace(line); strings.HasPrefix(fence, "```") {
			lang := strings.TrimSpace(strings.TrimPrefix(fence, "```"))
			var code []string
			i++
			for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
				code = append(code, lines[i])
				i++
			}
			i++ // consume closing fence (or run past EOF)
			blocks = append(blocks, codeBlock{lang: lang, code: strings.Join(code, "\n")})
			continue
		}

		// Blank line: paragraph/block separator.
		if strings.TrimSpace(line) == "" {
			i++
			continue
		}

		// Horizontal rule.
		if isHR(line) {
			blocks = append(blocks, hrBlock{})
			i++
			continue
		}

		// ATX heading.
		if m := mdHeading.FindStringSubmatch(line); m != nil {
			blocks = append(blocks, headingBlock{level: len(m[1]), text: m[2]})
			i++
			continue
		}

		// Blockquote: gather consecutive `>` lines, parse the inner block.
		if strings.HasPrefix(strings.TrimSpace(line), ">") {
			var inner []string
			for i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), ">") {
				t := strings.TrimSpace(lines[i])
				t = strings.TrimPrefix(t, ">")
				inner = append(inner, strings.TrimPrefix(t, " "))
				i++
			}
			blocks = append(blocks, quoteBlock{children: parseBlocks(strings.Join(inner, "\n"))})
			continue
		}

		// Unordered list.
		if mdULItem.MatchString(line) {
			var items []string
			for i < len(lines) && mdULItem.MatchString(lines[i]) {
				items = append(items, mdULItem.FindStringSubmatch(lines[i])[1])
				i++
			}
			blocks = append(blocks, listBlock{ordered: false, items: items})
			continue
		}

		// Ordered list.
		if mdOLItem.MatchString(line) {
			var items []string
			for i < len(lines) && mdOLItem.MatchString(lines[i]) {
				items = append(items, mdOLItem.FindStringSubmatch(lines[i])[1])
				i++
			}
			blocks = append(blocks, listBlock{ordered: true, items: items})
			continue
		}

		// Paragraph: consecutive non-blank lines that don't start another block.
		var para []string
		for i < len(lines) {
			cur := lines[i]
			if strings.TrimSpace(cur) == "" || isBlockStart(cur) {
				break
			}
			para = append(para, cur)
			i++
		}
		blocks = append(blocks, paragraphBlock{lines: para})
	}
	return blocks
}

// renderBlocks emits the sanitized HTML for a block list.
func renderBlocks(blocks []Block) string {
	var b strings.Builder
	for _, blk := range blocks {
		blk.renderTo(&b)
	}
	return b.String()
}

func (h headingBlock) renderTo(b *strings.Builder) {
	lv := strconv.Itoa(h.level)
	b.WriteString("<h" + lv + ">" + inline(h.text) + "</h" + lv + ">")
}

func (c codeBlock) renderTo(b *strings.Builder) {
	b.WriteString("<pre><code")
	if c.lang != "" {
		b.WriteString(` class="language-` + html.EscapeString(c.lang) + `"`)
	}
	b.WriteString(">" + html.EscapeString(c.code) + "</code></pre>")
}

func (l listBlock) renderTo(b *strings.Builder) {
	tag := "ul"
	if l.ordered {
		tag = "ol"
	}
	b.WriteString("<" + tag + ">")
	for _, item := range l.items {
		b.WriteString("<li>" + inline(item) + "</li>")
	}
	b.WriteString("</" + tag + ">")
}

func (q quoteBlock) renderTo(b *strings.Builder) {
	b.WriteString("<blockquote>" + renderBlocks(q.children) + "</blockquote>")
}

func (hrBlock) renderTo(b *strings.Builder) {
	b.WriteString("<hr>")
}

func (p paragraphBlock) renderTo(b *strings.Builder) {
	var para []string
	for _, line := range p.lines {
		para = append(para, inline(line))
	}
	b.WriteString("<p>" + strings.Join(para, "<br>") + "</p>")
}

// isBlockStart reports whether a line opens a non-paragraph block, so paragraph
// accumulation stops at it.
func isBlockStart(line string) bool {
	t := strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(t, "```"),
		strings.HasPrefix(t, ">"),
		isHR(line),
		mdHeading.MatchString(line),
		mdULItem.MatchString(line),
		mdOLItem.MatchString(line):
		return true
	}
	return false
}

// isHR reports whether a line is a horizontal rule: three or more of the same
// marker (-, *, _), ignoring interspersed spaces. RE2 lacks backreferences, so
// this is checked directly rather than with a regex.
func isHR(line string) bool {
	t := strings.ReplaceAll(strings.TrimSpace(line), " ", "")
	if len(t) < 3 {
		return false
	}
	c := t[0]
	if c != '-' && c != '*' && c != '_' {
		return false
	}
	for i := 0; i < len(t); i++ {
		if t[i] != c {
			return false
		}
	}
	return true
}

// inline escapes a line of text and layers inline markup (code, links, emphasis)
// on top. Code spans are extracted to placeholders first so their contents are
// never re-processed by the emphasis/link passes.
func inline(s string) string {
	s = html.EscapeString(s)

	var ph []string
	stash := func(htmlFrag string) string {
		ph = append(ph, htmlFrag)
		return "\x00" + strconv.Itoa(len(ph)-1) + "\x00"
	}

	// Inline code first — its contents are already escaped and must be opaque.
	s = mdInlineCode.ReplaceAllStringFunc(s, func(m string) string {
		inner := mdInlineCode.FindStringSubmatch(m)[1]
		return stash("<code>" + inner + "</code>")
	})

	// Links — validate the scheme, then stash the whole anchor.
	s = mdLink.ReplaceAllStringFunc(s, func(m string) string {
		sm := mdLink.FindStringSubmatch(m)
		text, url := sm[1], sm[2]
		if !safeURL(url) {
			return m // degrade to escaped literal text
		}
		return stash(`<a href="` + url + `">` + text + `</a>`)
	})

	// Emphasis: bold before italic so ** wins over *.
	s = mdBold.ReplaceAllString(s, "<strong>$1$2</strong>")
	s = mdItalic.ReplaceAllString(s, "<em>$1$2</em>")

	// Restore stashed fragments with a single left-to-right scan. Every NUL in s
	// is structural (input NULs are stripped before parsing), so NULs alternate
	// open/close around an index; a global ReplaceAll instead matched fake
	// placeholders spanning two real ones when literal digits sat between them.
	// A stashed fragment can itself hold placeholders (a link whose text/url
	// captured a code span), always with smaller indices, so expansion recurses
	// and terminates.
	var restore func(s string) string
	restore = func(s string) string {
		var out strings.Builder
		for {
			open := strings.IndexByte(s, '\x00')
			if open < 0 {
				out.WriteString(s)
				return out.String()
			}
			out.WriteString(s[:open])
			rest := s[open+1:]
			end := strings.IndexByte(rest, '\x00')
			if end < 0 {
				// Unreachable by construction (stash emits balanced pairs); if a
				// caller ever violates it, drop the sentinel rather than leak it.
				out.WriteString(rest)
				return out.String()
			}
			if idx, err := strconv.Atoi(rest[:end]); err == nil && idx >= 0 && idx < len(ph) {
				out.WriteString(restore(ph[idx]))
			}
			s = rest[end+1:]
		}
	}
	return restore(s)
}

// safeURL rejects any scheme that can execute script. Escaping has already run,
// so a scheme like "javascript:" survives literally; we match on that.
func safeURL(url string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(url))
	for _, bad := range []string{"javascript:", "data:", "vbscript:"} {
		if strings.HasPrefix(trimmed, bad) {
			return false
		}
	}
	return true
}
