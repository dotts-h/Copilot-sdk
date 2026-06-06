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

// renderMarkdown turns a markdown subset into sanitized HTML. The output is a
// trusted HTML string (every dynamic part is escaped before assembly).
func renderMarkdown(src string) string {
	// Strip the placeholder sentinel so input can never forge a placeholder.
	src = strings.ReplaceAll(src, "\x00", "")
	lines := strings.Split(src, "\n")

	var b strings.Builder
	i := 0
	for i < len(lines) {
		line := lines[i]

		// Fenced code block: ``` … ``` — content escaped verbatim, no inline pass.
		if fence := strings.TrimSpace(line); strings.HasPrefix(fence, "```") {
			lang := strings.TrimSpace(strings.TrimPrefix(fence, "```"))
			var code []string
			i++
			for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
				code = append(code, lines[i])
				i++
			}
			i++ // consume closing fence (or run past EOF)
			b.WriteString("<pre><code")
			if lang != "" {
				b.WriteString(` class="language-` + html.EscapeString(lang) + `"`)
			}
			b.WriteString(">" + html.EscapeString(strings.Join(code, "\n")) + "</code></pre>")
			continue
		}

		// Blank line: paragraph/block separator.
		if strings.TrimSpace(line) == "" {
			i++
			continue
		}

		// Horizontal rule.
		if isHR(line) {
			b.WriteString("<hr>")
			i++
			continue
		}

		// ATX heading.
		if m := mdHeading.FindStringSubmatch(line); m != nil {
			level := len(m[1])
			b.WriteString("<h" + strconv.Itoa(level) + ">" + inline(m[2]) + "</h" + strconv.Itoa(level) + ">")
			i++
			continue
		}

		// Blockquote: gather consecutive `>` lines, render the inner block.
		if strings.HasPrefix(strings.TrimSpace(line), ">") {
			var inner []string
			for i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), ">") {
				t := strings.TrimSpace(lines[i])
				t = strings.TrimPrefix(t, ">")
				inner = append(inner, strings.TrimPrefix(t, " "))
				i++
			}
			b.WriteString("<blockquote>" + renderMarkdown(strings.Join(inner, "\n")) + "</blockquote>")
			continue
		}

		// Unordered list.
		if mdULItem.MatchString(line) {
			b.WriteString("<ul>")
			for i < len(lines) && mdULItem.MatchString(lines[i]) {
				item := mdULItem.FindStringSubmatch(lines[i])[1]
				b.WriteString("<li>" + inline(item) + "</li>")
				i++
			}
			b.WriteString("</ul>")
			continue
		}

		// Ordered list.
		if mdOLItem.MatchString(line) {
			b.WriteString("<ol>")
			for i < len(lines) && mdOLItem.MatchString(lines[i]) {
				item := mdOLItem.FindStringSubmatch(lines[i])[1]
				b.WriteString("<li>" + inline(item) + "</li>")
				i++
			}
			b.WriteString("</ol>")
			continue
		}

		// Paragraph: consecutive non-blank lines that don't start another block,
		// joined with soft <br> breaks.
		var para []string
		for i < len(lines) {
			cur := lines[i]
			if strings.TrimSpace(cur) == "" || isBlockStart(cur) {
				break
			}
			para = append(para, inline(cur))
			i++
		}
		b.WriteString("<p>" + strings.Join(para, "<br>") + "</p>")
	}
	return b.String()
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

	// Restore stashed fragments.
	for idx := len(ph) - 1; idx >= 0; idx-- {
		s = strings.ReplaceAll(s, "\x00"+strconv.Itoa(idx)+"\x00", ph[idx])
	}
	return s
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
