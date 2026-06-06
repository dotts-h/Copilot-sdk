package web

import (
	"net/http"
	"strings"

	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
)

// This file implements the composer's slash-command autocomplete menu
// (WEB_UI_PLAN.md workstream 3). As the user types a leading "/", the input
// fires GET /commands?prompt=… and swaps the returned menu into #cmd-menu;
// clicking an item fills the composer via the fillCmd() helper in index.html.

// commandSpec describes one composer slash command for help and autocomplete.
type commandSpec struct {
	Name string // command name without the leading slash, e.g. "model"
	Args string // argument hint, e.g. "[name]" or "" when none
	Desc string // one-line description
}

// fixedCommandSpecs are the non-navigation composer commands, in menu order.
var fixedCommandSpecs = []commandSpec{
	{"model", "[name]", "Switch the model in place (restarts the session)"},
	{"effort", "[low|medium|high]", "Set the reasoning effort (restarts the session)"},
	{"agent", "[id|none]", "Activate a forge agent or clear it"},
	{"plan", "[on|off]", "Toggle plan mode — the agent drafts a plan for your review"},
	{"auto", "[on|off]", "Toggle autopilot — run tools without pausing to ask"},
	{"ask", "[on|off]", "Toggle ask mode — check in before each action"},
	{"clear", "", "Reset the conversation and start a fresh session"},
	{"cost", "", "Show credit usage and refresh the meter"},
	{"attach", "<path>", "Queue a file to send with the next message"},
	{"help", "", "List the commands in the timeline"},
}

// commandSpecs returns the full command registry: the fixed commands plus the
// navigation slugs (each page is reachable as /slug from the composer).
func commandSpecs() []commandSpec {
	specs := make([]commandSpec, 0, len(fixedCommandSpecs)+len(pageNames))
	specs = append(specs, fixedCommandSpecs...)
	for _, p := range pageNames {
		specs = append(specs, commandSpec{Name: p.slug, Desc: "Go to the " + p.label + " page"})
	}
	return specs
}

// slashPrefix reports whether input is a bare or partial slash command still
// being named ("/" or "/mo"), returning the lowercased query after the slash. It
// is false once a space appears (the command is named and args are being typed).
func slashPrefix(input string) (q string, ok bool) {
	if !strings.HasPrefix(input, "/") {
		return "", false
	}
	q = strings.ToLower(strings.TrimSpace(input[1:]))
	if strings.ContainsAny(q, " \t") {
		return "", false
	}
	return q, true
}

// matchCommands returns the commands whose name has the typed prefix. It returns
// nil unless the input is a bare or partial slash command still being named:
// "/" lists everything, "/mo" filters by prefix, but anything containing a space
// (the command is fully named and args are being typed) yields no menu.
func matchCommands(input string) []commandSpec {
	q, ok := slashPrefix(input)
	if !ok {
		return nil
	}
	var out []commandSpec
	for _, c := range commandSpecs() {
		if strings.HasPrefix(c.Name, q) {
			out = append(out, c)
		}
	}
	return out
}

// matchSnippets returns the library snippets whose id (the /trigger) has the
// typed prefix, skipping any whose id collides with a reserved command/page slug
// (the command always wins, so a colliding snippet is never offered here). Same
// prefix rules as matchCommands.
func matchSnippets(input string, snippets []ctxforge.Snippet) []ctxforge.Snippet {
	q, ok := slashPrefix(input)
	if !ok {
		return nil
	}
	var out []ctxforge.Snippet
	for _, sn := range snippets {
		if isReservedCommand(sn.ID) {
			continue
		}
		if strings.HasPrefix(sn.ID, q) {
			out = append(out, sn)
		}
	}
	return out
}

// isReservedCommand reports whether name is a built-in composer command (or one
// of its aliases) or a nav-page slug. Snippets with a reserved id are shadowed by
// the command at both menu time and submit time, so the meaning of "/clear" can
// never be hijacked by a user-defined snippet.
func isReservedCommand(name string) bool {
	switch name {
	case "?", "autopilot":
		return true
	}
	for _, c := range fixedCommandSpecs {
		if c.Name == name {
			return true
		}
	}
	return isNavSlug(name)
}

// menuEntry is one row of the composer autocomplete menu: a command (filled into
// the composer as "/name ") or a snippet (whose Body is inserted client-side for
// editing). The two share the cmdMenu template via the Snippet flag.
type menuEntry struct {
	Name    string
	Args    string
	Desc    string
	Snippet bool
	Body    string
}

// handleCommands serves the autocomplete menu for the current composer value:
// matching commands first, then library snippets.
func (s *Server) handleCommands(w http.ResponseWriter, r *http.Request) {
	prompt := r.FormValue("prompt")
	cmds := matchCommands(prompt)
	var snippets []ctxforge.Snippet
	if s.forge != nil {
		s.hub.forgeMu.Lock()
		snippets = matchSnippets(prompt, s.forge.Snippets)
		s.hub.forgeMu.Unlock()
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(renderMenu(cmds, snippets)))
}

// renderMenu renders the autocomplete dropdown for the matched commands and
// snippets. An empty match set returns an empty string, which clears #cmd-menu.
// A command name is dropped into the onclick handler in a JS-string context
// (html/template applies JavaScript escaping); a snippet's body rides in a
// data attribute (html/template applies attribute escaping) and is read back as
// a plain string by fillSnippet — it never reaches the DOM as HTML.
func renderMenu(cmds []commandSpec, snippets []ctxforge.Snippet) string {
	if len(cmds)+len(snippets) == 0 {
		return ""
	}
	entries := make([]menuEntry, 0, len(cmds)+len(snippets))
	for _, c := range cmds {
		entries = append(entries, menuEntry{Name: c.Name, Args: c.Args, Desc: c.Desc})
	}
	for _, sn := range snippets {
		entries = append(entries, menuEntry{Name: sn.ID, Desc: sn.Name, Snippet: true, Body: sn.Body})
	}
	return frag("cmdMenu", entries)
}
