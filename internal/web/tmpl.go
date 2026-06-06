package web

import (
	"html"
	"html/template"
	"strings"
)

// This file wires the html/template engine the renderers use. Structural HTML
// (tags, attributes, labels) is emitted through templates, where html/template's
// contextual auto-escaping applies. Free-form transcript text additionally needs
// newline→<br> conversion, which the engine does not do; that flows through the
// richtext template function (the one audited escape+linebreak path, identical
// to esc()).

// funcMap is shared by every template parse.
var funcMap = template.FuncMap{
	// richtext escapes s and converts newlines to <br>, returning trusted HTML.
	// It is the only sanctioned way to drop multi-line text into a template.
	"richtext": func(s string) template.HTML {
		return template.HTML(esc(s)) //nolint:gosec // esc() escapes; this only adds <br>
	},
	// markdown renders a safe markdown subset for committed agent turns (ADR 0001).
	// renderMarkdown escapes all input first and only emits whitelisted tags.
	"markdown": func(s string) template.HTML {
		return template.HTML(renderMarkdown(s)) //nolint:gosec // renderMarkdown escapes input; whitelist-only tags
	},
	// clampLines bounds a long tool result before it is escaped by richtext.
	"clampLines":  clampLines,
	"humanTokens": humanTokens,
	// markGlyph renders the enabled/disabled checkbox glyph (trusted HTML).
	"markGlyph": func(on bool) template.HTML {
		if on {
			return template.HTML(`<span class="on">[x]</span>`)
		}
		return template.HTML(`<span class="off">[ ]</span>`)
	},
	// def returns s or a fallback when s is empty (shared with the package func).
	"def": def,
}

// trusted marks already-rendered HTML (produced by another renderer that has
// itself escaped its inputs) safe to inject verbatim into a template.
func trusted(s string) template.HTML { return template.HTML(s) } //nolint:gosec // composed from escaped fragments

// frag executes a named fragment template into a string. A template error (a
// programming mistake, since the data shapes are fixed here) renders as an HTML
// comment rather than panicking mid-stream.
func frag(name string, data any) string {
	var b strings.Builder
	if err := pageTemplates.ExecuteTemplate(&b, name, data); err != nil {
		return "<!-- render " + html.EscapeString(name) + ": " + html.EscapeString(err.Error()) + " -->"
	}
	return b.String()
}
