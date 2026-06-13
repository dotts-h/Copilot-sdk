// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package web

import "strings"

// commandPalette renders the body-level ⌘/Ctrl-K command palette (V22,
// ADR-0026), mirroring the help overlay: a hidden aria-modal dialog with a
// filter input over a server-rendered list of {slug,label,group} items. The
// list is the same pageNames source the sidebar groups; it is filtered
// client-side and navigates the match through the existing keymap dispatch
// (navClick) — no new server route. It lives in the page shell so it survives
// htmx #main swaps. Each item carries its slug in a data-slug attribute (read by
// the delegated click handler and the Enter path), so the navigation target is
// never interpolated into a JS-string context. Labels/slugs/groups are static
// but HTML-escaped in their attribute/text contexts via esc() (ADR-0001).
func commandPalette() string {
	var b strings.Builder
	b.WriteString(`<div id="cmdk-overlay" class="overlay cmdk" hidden role="dialog" aria-modal="true" aria-label="Command palette">`)
	b.WriteString(`<div class="overlay-card cmdk-card">`)
	b.WriteString(`<input type="text" class="cmdk-input" placeholder="Jump to a page…" aria-label="Filter pages" ` +
		`autocomplete="off" spellcheck="false" oninput="cmdkFilter()" onkeydown="cmdkKeydown(event)">`)
	b.WriteString(`<ul class="cmdk-list">`)
	for _, p := range pageNames {
		slug, label, group := esc(p.slug), esc(p.label), esc(p.group)
		b.WriteString(`<li><button type="button" class="cmdk-item" ` +
			`data-slug="` + slug + `" data-label="` + label + `" data-group="` + group + `">` +
			`<span class="cmdk-item-label">` + label + `</span> ` +
			`<span class="cmdk-item-group">` + group + `</span></button></li>`)
	}
	b.WriteString(`</ul>`)
	b.WriteString(`<button type="button" class="overlay-close" onclick="toggleCmdk(false)">Close</button>`)
	b.WriteString(`</div></div>`)
	return b.String()
}
