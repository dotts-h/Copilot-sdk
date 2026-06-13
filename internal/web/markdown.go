// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package web

import (
	"html"
	"html/template"
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
	// mdCallout matches a GitHub-alert marker as the first line of a blockquote:
	// `[!KIND]` optionally followed by a custom title. The kind is validated
	// against calloutKinds before a callout block is produced.
	mdCallout = regexp.MustCompile(`^\[!([A-Za-z]+)\]\s*(.*)$`)
)

// calloutKinds maps each recognized alert kind to its default label and glyph.
// The glyph plus the label both carry the kind, so meaning never rides on color
// alone (WCAG); the kind also names the CSS class that maps to a semantic token.
// An unrecognized kind is not a callout — the blockquote degrades unchanged.
var calloutKinds = map[string]struct{ label, glyph string }{
	"note":      {"Note", "ℹ"},
	"tip":       {"Tip", "✦"},
	"important": {"Important", "❖"},
	"warning":   {"Warning", "⚠"},
	"caution":   {"Caution", "⛔"},
}

// Block is one structural element of the markdown subset (ADR 0045). The
// interface is sealed: every implementation lives in this file, and each
// renderer may emit only whitelisted or templated HTML, with every dynamic
// string escape-first via inline()/html.EscapeString. The citation scope (ADR
// 0049) is threaded in so an inline [n] resolves against the document's defined
// sources; a nil scope means no citations (every [n] stays literal).
type Block interface {
	renderTo(b *strings.Builder, cs *citeScope)
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

// calloutBlock is a GitHub-alert blockquote rendered as a designed callout
// (ADR 0046): kind is one of calloutKinds, title is the raw label/custom text
// for the inline pass, and children are the recursively-parsed body.
type calloutBlock struct {
	kind     string
	title    string
	children []Block
}

// tableBlock is a GFM pipe table (ADR 0045 / R4, issue 0080). headers and the
// cells in rows are raw markdown for the inline pass; aligns holds the
// per-column alignment ("", "left", "center", "right") parsed from the
// delimiter row. Rendered to a token-styled <table> via frag("table", …);
// ragged rows are padded/truncated to the header width so the grid stays
// rectangular and never leaks markup.
type tableBlock struct {
	headers []string
	aligns  []string
	rows    [][]string
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
// Reference-style citation definitions (ADR 0049) are lifted out first — they
// never render as prose — and, when any resolve, a single sourcesBlock is
// appended so the rendered source cards close the document and the citation
// scope can be derived from the block list (keeping renderMarkdown exactly
// renderBlocks∘parseBlocks).
func parseBlocks(src string) []Block {
	lines, sources := extractCitations(strings.Split(src, "\n"))
	blocks := parseLines(lines)
	if len(sources) > 0 {
		blocks = append(blocks, sourcesBlock{sources: sources})
	}
	return blocks
}

func parseLines(lines []string) []Block { return parseLinesDepth(lines, 0) }

// parseLinesDepth is parseLines threaded with the current container-directive
// nesting depth, so directive recursion is bounded (ADR-0047). Blockquote and
// callout bodies carry the same depth (they don't nest a directive fence);
// a directive body recurses at depth+1.
func parseLinesDepth(lines []string, depth int) []Block {
	var blocks []Block
	i := 0
	for i < len(lines) {
		line := lines[i]

		// Container directive (R5, issue 0081): `:::name{attrs}` … `:::` resolved
		// against the registry allowlist. Recognized only when the name is
		// registered and a matching close is found within the depth budget; an
		// unregistered name / over-depth / unterminated open is not a directive and
		// falls through to ordinary parsing (degrade to escaped text).
		if name, attrs, body, next, ok := directiveAt(lines, i, depth); ok {
			blocks = append(blocks, directiveBlock{
				name:     name,
				attrs:    attrs,
				children: parseLinesDepth(body, depth+1),
			})
			i = next
			continue
		}
		// A registered open with no usable close (unterminated, or past the depth
		// budget) is not a directive: degrade the marker line to escaped text.
		// Consuming just this line — rather than letting the paragraph loop break
		// on it forever — keeps the loop progressing and the body after it parses
		// as normal blocks (ADR-0047).
		if isDirectiveOpenLine(line) {
			blocks = append(blocks, paragraphBlock{lines: []string{line}})
			i++
			continue
		}

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
			// A leading [!KIND] marker of a known kind promotes the blockquote to
			// a callout; anything else stays a plain quote (degrade-safe).
			if len(inner) > 0 {
				if m := mdCallout.FindStringSubmatch(strings.TrimSpace(inner[0])); m != nil {
					if k, ok := calloutKinds[strings.ToLower(m[1])]; ok {
						title := strings.TrimSpace(m[2])
						if title == "" {
							title = k.label
						}
						blocks = append(blocks, calloutBlock{
							kind:     strings.ToLower(m[1]),
							title:    title,
							children: parseLinesDepth(inner[1:], depth),
						})
						continue
					}
				}
			}
			blocks = append(blocks, quoteBlock{children: parseLinesDepth(inner, depth)})
			continue
		}

		// GFM pipe table: a pipe-bearing header row followed by a delimiter row
		// with a matching cell count. The delimiter sets per-column alignment;
		// body rows continue until a blank line, a non-pipe line, or another
		// block start.
		if startsTable(lines, i) {
			headers := splitTableRow(lines[i])
			aligns := tableAligns(lines[i+1])
			i += 2
			var rows [][]string
			for i < len(lines) {
				if strings.TrimSpace(lines[i]) == "" || !strings.Contains(lines[i], "|") || isBlockStart(lines[i]) || isDirectiveOpenLine(lines[i]) {
					break
				}
				rows = append(rows, splitTableRow(lines[i]))
				i++
			}
			blocks = append(blocks, tableBlock{headers: headers, aligns: aligns, rows: rows})
			continue
		}

		// List: a run of consecutive items, unordered (-/*/+) or ordered (1.).
		if itemRe := listItemRe(line); itemRe != nil {
			var items []string
			for i < len(lines) && itemRe.MatchString(lines[i]) {
				items = append(items, itemRe.FindStringSubmatch(lines[i])[1])
				i++
			}
			blocks = append(blocks, listBlock{ordered: itemRe == mdOLItem, items: items})
			continue
		}

		// Paragraph: consecutive non-blank lines that don't start another block.
		var para []string
		for i < len(lines) {
			cur := lines[i]
			if strings.TrimSpace(cur) == "" || isBlockStart(cur) || startsTable(lines, i) || isDirectiveOpenLine(cur) {
				break
			}
			para = append(para, cur)
			i++
		}
		blocks = append(blocks, paragraphBlock{lines: para})
	}
	return blocks
}

// listItemRe returns the list-item regex a line matches, or nil. The two
// regexes are disjoint (a marker can't be both a bullet and a number).
func listItemRe(line string) *regexp.Regexp {
	switch {
	case mdULItem.MatchString(line):
		return mdULItem
	case mdOLItem.MatchString(line):
		return mdOLItem
	}
	return nil
}

// startsTable reports whether lines[i] opens a GFM pipe table: a pipe-bearing
// header row whose next line is a delimiter row with a matching cell count.
// The cell-count match is GFM's own rule — a mismatched delimiter is not a
// table — and it keeps pipe-bearing prose from being misread as a header.
func startsTable(lines []string, i int) bool {
	if i+1 >= len(lines) || !strings.Contains(lines[i], "|") {
		return false
	}
	if !isTableDelimiter(lines[i+1]) {
		return false
	}
	return len(splitTableRow(lines[i])) == len(splitTableRow(lines[i+1]))
}

// splitTableRow splits a pipe-table row into trimmed cells, tolerating the
// optional leading/trailing pipes GFM allows.
func splitTableRow(line string) []string {
	s := strings.TrimSpace(line)
	s = strings.TrimPrefix(s, "|")
	s = strings.TrimSuffix(s, "|")
	parts := strings.Split(s, "|")
	cells := make([]string, len(parts))
	for i, p := range parts {
		cells[i] = strings.TrimSpace(p)
	}
	return cells
}

// isTableDelimiter reports whether a line is a GFM table delimiter row: a
// pipe-separated run of cells, each dashes with optional leading/trailing
// colons (the alignment markers).
func isTableDelimiter(line string) bool {
	if !strings.Contains(line, "|") {
		return false
	}
	cells := splitTableRow(line)
	if len(cells) == 0 {
		return false
	}
	for _, c := range cells {
		if !isDelimCell(c) {
			return false
		}
	}
	return true
}

// isDelimCell reports whether a delimiter cell is `:?-+:?` — at least one dash,
// optional alignment colons.
func isDelimCell(c string) bool {
	c = strings.TrimPrefix(c, ":")
	c = strings.TrimSuffix(c, ":")
	if c == "" {
		return false
	}
	for i := 0; i < len(c); i++ {
		if c[i] != '-' {
			return false
		}
	}
	return true
}

// tableAligns maps each delimiter cell to its column alignment: ":-:" center,
// "-:" right, ":-" left, "-" none ("").
func tableAligns(line string) []string {
	cells := splitTableRow(line)
	aligns := make([]string, len(cells))
	for i, c := range cells {
		left := strings.HasPrefix(c, ":")
		right := strings.HasSuffix(c, ":")
		switch {
		case left && right:
			aligns[i] = "center"
		case right:
			aligns[i] = "right"
		case left:
			aligns[i] = "left"
		}
	}
	return aligns
}

// renderBlocks emits the sanitized HTML for a block list. The citation scope is
// derived from the blocks themselves (the sourcesBlock parseBlocks appended), so
// this stays a pure []Block→string seam — renderMarkdown is exactly its
// composition with parseBlocks, and a caller passing hand-built blocks gets
// citation resolution iff those blocks carry a sourcesBlock.
func renderBlocks(blocks []Block) string {
	return renderBlocksScoped(blocks, scopeOf(blocks))
}

// renderBlocksScoped renders a block list under an explicit citation scope, so a
// nested container (quote/callout/directive body) resolves markers against the
// document's sources rather than re-deriving an empty scope from its children.
func renderBlocksScoped(blocks []Block, cs *citeScope) string {
	var b strings.Builder
	renderBlocksTo(&b, blocks, cs)
	return b.String()
}

func renderBlocksTo(b *strings.Builder, blocks []Block, cs *citeScope) {
	for _, blk := range blocks {
		blk.renderTo(b, cs)
	}
}

func (h headingBlock) renderTo(b *strings.Builder, cs *citeScope) {
	lv := strconv.Itoa(h.level)
	b.WriteString("<h" + lv + ">" + inline(h.text, cs) + "</h" + lv + ">")
}

func (c codeBlock) renderTo(b *strings.Builder, _ *citeScope) {
	// Designed code-block frame (R3, issue 0079): the language label + a copy
	// affordance over a token-styled surface, rendered through the frag() template
	// the rest of the UI uses instead of hand-built <pre> string concat. The code
	// stays escape-first — html.EscapeString runs before it rides into the template
	// as trusted HTML — so the <pre> can never carry live markup; .Lang is escaped
	// contextually by html/template in both the label and the language-* class.
	b.WriteString(frag("codeBlock", struct {
		Lang string
		Code template.HTML
	}{
		Lang: c.lang,
		Code: template.HTML(html.EscapeString(c.code)), //nolint:gosec // EscapeString escapes first; verbatim code only
	}))
}

func (l listBlock) renderTo(b *strings.Builder, cs *citeScope) {
	tag := "ul"
	if l.ordered {
		tag = "ol"
	}
	b.WriteString("<" + tag + ">")
	for _, item := range l.items {
		b.WriteString("<li>" + inline(item, cs) + "</li>")
	}
	b.WriteString("</" + tag + ">")
}

func (q quoteBlock) renderTo(b *strings.Builder, cs *citeScope) {
	b.WriteString("<blockquote>")
	renderBlocksTo(b, q.children, cs)
	b.WriteString("</blockquote>")
}

func (c calloutBlock) renderTo(b *strings.Builder, cs *citeScope) {
	// Kind is validated against calloutKinds at parse time, so the glyph lookup
	// always hits; the title rides the inline pass (escape-first), the body is
	// already-rendered trusted HTML under the document citation scope. All dynamic
	// parts are escaped before assembly.
	b.WriteString(frag("callout", struct {
		Kind  string
		Glyph string
		Title template.HTML
		Body  template.HTML
	}{
		Kind:  c.kind,
		Glyph: calloutKinds[c.kind].glyph,
		Title: template.HTML(inline(c.title, cs)),                //nolint:gosec // inline() escapes first, whitelist-only tags
		Body:  template.HTML(renderBlocksScoped(c.children, cs)), //nolint:gosec // composed from escaped block renderers
	}))
}

func (t tableBlock) renderTo(b *strings.Builder, cs *citeScope) {
	// Each cell rides the inline pass (escape-first) before it enters the
	// template as trusted HTML; the alignment class is one of a fixed set keyed
	// off the parsed delimiter, so no dynamic attribute is attacker-controlled.
	// Rows are padded/truncated to the header width — a ragged row can never
	// emit a cell outside the templated grid.
	type tcell struct {
		Class string
		HTML  template.HTML
	}
	alignClass := func(i int) string {
		if i < len(t.aligns) {
			switch t.aligns[i] {
			case "left":
				return "ta-left"
			case "center":
				return "ta-center"
			case "right":
				return "ta-right"
			}
		}
		return ""
	}
	head := make([]tcell, len(t.headers))
	for i, h := range t.headers {
		head[i] = tcell{Class: alignClass(i), HTML: template.HTML(inline(h, cs))} //nolint:gosec // inline() escapes first, whitelist-only tags
	}
	type trow struct{ Cells []tcell }
	rows := make([]trow, 0, len(t.rows))
	for _, r := range t.rows {
		cells := make([]tcell, len(t.headers))
		for i := range t.headers {
			var raw string
			if i < len(r) {
				raw = r[i]
			}
			cells[i] = tcell{Class: alignClass(i), HTML: template.HTML(inline(raw, cs))} //nolint:gosec // inline() escapes first, whitelist-only tags
		}
		rows = append(rows, trow{Cells: cells})
	}
	b.WriteString(frag("table", struct {
		Head []tcell
		Rows []trow
	}{Head: head, Rows: rows}))
}

func (hrBlock) renderTo(b *strings.Builder, _ *citeScope) {
	b.WriteString("<hr>")
}

func (p paragraphBlock) renderTo(b *strings.Builder, cs *citeScope) {
	b.WriteString("<p>")
	for j, line := range p.lines {
		if j > 0 {
			b.WriteString("<br>")
		}
		b.WriteString(inline(line, cs))
	}
	b.WriteString("</p>")
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

// inline escapes a line of text and layers inline markup (code, links,
// citations, emphasis) on top. Code spans are extracted to placeholders first so
// their contents are never re-processed by the later passes. The citation scope
// (ADR 0049) is consulted only when non-nil: an inline [n] whose number has a
// defined source becomes a marker, otherwise it stays literal escaped text.
func inline(s string, cs *citeScope) string {
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

	// Citation markers — after code/links are stashed, so only bare [n] in prose
	// remain. A number with a defined source becomes a stashed marker fragment;
	// any other [n] is left as escaped literal text (a broken citation degrades,
	// never fabricates). Skipped entirely when the document has no sources.
	if cs != nil {
		s = mdCiteRef.ReplaceAllStringFunc(s, func(m string) string {
			n, err := strconv.Atoi(mdCiteRef.FindStringSubmatch(m)[1])
			if err != nil {
				return m
			}
			src, ok := cs.byNum[n]
			if !ok {
				return m // no matching source: degrade to literal
			}
			return stash(renderCiteRef(src))
		})
	}

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
