// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package web

import (
	"html"
	"html/template"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Citation cards (R6, issue 0082, ADR-0049): the inline + block half of the
// designed-output epic. A reference-style line `[n]: url "title"` registers a
// **source** (the def line itself never renders as prose); an inline `[n]`
// whose number has a matching source becomes a citation **marker** — a `<sup>`
// anchor to a server-rendered **source card**, with a CSS-only hover/focus
// preview (the Perplexity progressive-disclosure pattern, no JS). A marker with
// no matching source degrades to plain escaped text — the research rule: a
// broken citation is shown literally, never fabricated.
//
// Like every slice on the ADR-0045 seam this is escape-first: the def URL is
// validated by safeURL at parse time (an unsafe scheme registers no source), and
// every dynamic string rides inline()/html.EscapeString or the template engine's
// contextual escaping before it becomes trusted HTML.

var (
	// mdCiteDef matches a reference-style citation definition line (already
	// trimmed): `[n]: <url> "optional title"`. The URL is whitespace-free; the
	// quoted title is optional. A line of this shape is a definition (stripped
	// from the rendered flow) — it registers a source only when n≥1, the URL is
	// safe, and n is not already defined.
	mdCiteDef = regexp.MustCompile(`^\[(\d+)\]:\s+(\S+)(?:\s+"([^"]*)")?$`)
	// mdCiteRef matches an inline citation marker `[n]`. It runs in the inline
	// pass after code spans and links have been stashed, so only bare markers in
	// prose remain to match.
	mdCiteRef = regexp.MustCompile(`\[(\d+)\]`)
)

// citeSource is one resolved citation source: the number it answers, the
// validated (safe-scheme) target URL, the display title (the quoted title, or
// the origin when none was given), and the origin shown beneath it.
type citeSource struct {
	num    int
	url    string
	title  string
	origin string
}

// sourcesBlock is the document-level list of citation sources, appended once at
// the end of the block list by parseBlocks when any definition was found. It
// renders the token-styled source cards the inline markers anchor to; it also
// carries the set of defined numbers that scopeOf reads to resolve markers.
type sourcesBlock struct {
	sources []citeSource
}

// citeScope is the per-document citation context threaded through rendering: the
// sources keyed by number, so an inline [n] resolves to a marker only when n is
// defined. A nil scope means no citations in the document — every [n] stays
// literal, and the renderer's output is byte-identical to the pre-R6 subset.
type citeScope struct {
	byNum map[int]citeSource
}

// scopeOf derives the citation scope from a block list by finding the
// sourcesBlock parseBlocks appended (top level). Returns nil when there are no
// citations, so the inline pass skips its citation work entirely.
func scopeOf(blocks []Block) *citeScope {
	for _, blk := range blocks {
		if sb, ok := blk.(sourcesBlock); ok {
			m := make(map[int]citeSource, len(sb.sources))
			for _, s := range sb.sources {
				m[s.num] = s
			}
			return &citeScope{byNum: m}
		}
	}
	return nil
}

// extractCitations removes every reference-style definition line from the input
// and returns the remaining lines plus the resolved sources, sorted by number. A
// definition registers a source only when its number is ≥1, not already defined,
// and its URL passes safeURL — an unsafe scheme registers nothing (its markers
// will degrade), but the line is still removed so the raw definition never leaks
// into the rendered text.
//
// Two boundaries keep the lift faithful to how the block parser sees the
// document:
//   - Fenced code is verbatim, so a `[n]: …` line inside a ``` block is NOT a
//     definition — a code sample documenting the citation syntax must survive
//     intact (and must not fabricate a phantom source). The fence is tracked with
//     the same toggle the block parser / directiveAt use.
//   - A definition line is dropped *entirely* (not blanked), so a definition
//     sitting between two soft-wrapped prose lines doesn't inject a blank that
//     reflows them into separate paragraphs — it behaves as invisible metadata.
func extractCitations(lines []string) ([]string, []citeSource) {
	out := make([]string, 0, len(lines))
	seen := map[int]bool{}
	var sources []citeSource
	inCode := false
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inCode = !inCode
			out = append(out, line)
			continue
		}
		if inCode {
			out = append(out, line) // verbatim: never a definition
			continue
		}
		m := mdCiteDef.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			out = append(out, line) // ordinary content: keep it
			continue
		}
		// A definition-shaped line is metadata, not prose: drop it (don't append).
		n, err := strconv.Atoi(m[1])
		if err != nil || n < 1 || seen[n] {
			continue
		}
		rawURL := m[2]
		if !safeURL(rawURL) {
			continue // unsafe scheme: register nothing, markers degrade
		}
		origin := citeOrigin(rawURL)
		title := strings.TrimSpace(m[3])
		if title == "" {
			title = origin
		}
		seen[n] = true
		sources = append(sources, citeSource{num: n, url: rawURL, title: title, origin: origin})
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].num < sources[j].num })
	return out, sources
}

// citeOrigin is the human-readable origin shown on a source card: the URL host
// when there is one, else the raw target (relative links have no host).
func citeOrigin(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil && u.Host != "" {
		return u.Host
	}
	return rawURL
}

// renderCiteRef renders one inline citation marker: a <sup> anchor to the source
// card (#cite-N) plus a CSS-only preview popover. It is hand-built — like the
// inline code/link spans it sits beside — rather than going through frag(),
// because inline() must not statically reference the template engine (frag →
// pageTemplates → funcMap → renderMarkdown → … → inline would be a package-init
// cycle; the renderTo methods escape it only via interface dispatch). Every
// dynamic part is still escape-first: N is an integer, the title rides the
// inline escape pass (whitelist-only tags), and the origin is html-escaped.
func renderCiteRef(src citeSource) string {
	ns := strconv.Itoa(src.num)
	return `<sup class="cite"><a href="#cite-` + ns + `">` + ns + `</a>` +
		`<span class="cite-pop">` +
		`<span class="cite-pop-title">` + inline(src.title, nil) + `</span>` +
		`<span class="cite-pop-origin">` + html.EscapeString(src.origin) + `</span>` +
		`</span></sup>`
}

func (s sourcesBlock) renderTo(b *strings.Builder, _ *citeScope) {
	type card struct {
		N      int
		URL    string
		Title  template.HTML
		Origin string
	}
	cards := make([]card, len(s.sources))
	for i, src := range s.sources {
		cards[i] = card{
			N:      src.num,
			URL:    src.url,
			Title:  template.HTML(inline(src.title, nil)), //nolint:gosec // inline() escapes first, whitelist-only tags
			Origin: src.origin,
		}
	}
	b.WriteString(frag("citations", struct{ Sources []card }{Sources: cards}))
}
