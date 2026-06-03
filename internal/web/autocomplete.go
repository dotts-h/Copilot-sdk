package web

import (
	"net/http"
	"strings"
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
	{"agent", "[id|none]", "Activate a forge agent or clear it"},
	{"plan", "[on|off]", "Toggle plan mode — the agent drafts a plan for your review"},
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

// matchCommands returns the commands whose name has the typed prefix. It returns
// nil unless the input is a bare or partial slash command still being named:
// "/" lists everything, "/mo" filters by prefix, but anything containing a space
// (the command is fully named and args are being typed) yields no menu.
func matchCommands(input string) []commandSpec {
	if !strings.HasPrefix(input, "/") {
		return nil
	}
	q := strings.ToLower(strings.TrimSpace(input[1:]))
	if strings.ContainsAny(q, " \t") {
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

// handleCommands serves the autocomplete menu for the current composer value.
func (s *Server) handleCommands(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(renderCommandMenu(matchCommands(r.FormValue("prompt")))))
}

// renderCommandMenu renders the autocomplete dropdown. An empty match list
// returns an empty string, which clears #cmd-menu. The command name is dropped
// into the onclick handler in a JS-string context, where html/template applies
// JavaScript escaping.
func renderCommandMenu(matches []commandSpec) string {
	if len(matches) == 0 {
		return ""
	}
	return frag("cmdMenu", matches)
}
