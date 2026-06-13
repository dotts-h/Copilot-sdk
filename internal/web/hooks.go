// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package web

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
)

// This file is the Hooks management page — the in-app CRUD over the forge's
// governance hooks (ADR-0031), mirroring the MCP/skills/workflow pages (list +
// form + preflight, validated builders, rollback-on-invalid). It distinguishes
// the shipped BUILT-IN hooks (the safe-read defaults + the mandatory dangerous
// ruleset, read-only — listed so the policy is legible but never edited) from
// USER hooks (full add/edit/enable-disable/remove). The form's preflight calls
// the pure matcher so a user can see a rule fire against a sample command before
// saving. Mode binding is exposed as a per-hook set of agent modes.

// hookKindOpts are the tool-kind options in the form; "any" is the sentinel for
// an empty ToolKind (match any kind), mapped back to "" on save.
var hookKindOpts = []string{"any", "read", "write", "shell", "mcp"}

const hookKindAny = "any"

// hookModeOpts are the agent modes a hook can be bound to (mode binding,
// ADR-0031). An empty selection means the hook applies in every mode.
var hookModeOpts = []string{ctxforge.ModeAutopilot, ctxforge.ModeInteractive, ctxforge.ModePlan}

// builtinHookSet is the shipped policy shown read-only on the page: the safe-read
// defaults followed by the mandatory dangerous ruleset, exactly as Forge.Compile
// folds them into every session. They evaluate through the same path as user
// hooks, so listing them makes the active policy legible. The set is immutable, so
// it is built once (DangerousHooks loops over dozens of patterns) and reused
// read-only across the frequent HTMX list re-renders.
var builtinHookSet = sync.OnceValue(func() []ctxforge.Hook {
	return append(ctxforge.DefaultHooks(), ctxforge.DangerousHooks()...)
})

// hooksPartial renders the Hooks page: the read-only built-in rows first, then
// the user hooks with full CRUD controls. It snapshots the user hooks under
// forgeMu and releases the lock before rendering.
func (s *Server) hooksPartial() string {
	s.hub.forgeMu.Lock()
	user := make([]ctxforge.Hook, len(s.forge.Hooks))
	copy(user, s.forge.Hooks)
	s.hub.forgeMu.Unlock()

	builtin := make([]map[string]any, 0)
	for _, h := range builtinHookSet() {
		builtin = append(builtin, hookRow(h, true))
	}
	rows := make([]map[string]any, 0, len(user))
	for _, h := range user {
		rows = append(rows, hookRow(h, false))
	}
	return frag("hooksPage", map[string]any{
		"Add": addData("hooks", "hook"), "Builtin": builtin, "Rows": rows,
	})
}

// hookRow builds the template data for one hook list row. A built-in row is
// read-only (no toggle/edit/delete) and badged; a mandatory built-in is badged
// "unbypassable".
func hookRow(h ctxforge.Hook, builtin bool) map[string]any {
	return map[string]any{
		"ID": h.ID, "On": h.Enabled, "Name": hookSummary(h), "Desc": h.Reason,
		"Action": h.Action, "Builtin": builtin, "Mandatory": h.Mandatory,
		"Modes": strings.Join(h.Modes, ", "), "HasModes": len(h.Modes) > 0,
	}
}

// hookSummary is the one-line label for a hook: its event and the match
// dimensions that are set (tool kind, command pattern, the outside-workspace
// fence). It never includes the reason (that is the row's description).
func hookSummary(h ctxforge.Hook) string {
	parts := []string{shortEvent(h.Event)}
	if k := h.Match.ToolKind; k != "" {
		parts = append(parts, k)
	} else {
		parts = append(parts, "any")
	}
	if h.Match.Pattern != "" {
		parts = append(parts, "“"+truncate(h.Match.Pattern, 40)+"”")
	}
	if h.Match.OutsideWorkspace {
		parts = append(parts, "outside-workspace")
	}
	return strings.Join(parts, " · ")
}

// shortEvent renders the hook event compactly for the list label.
func shortEvent(ev string) string {
	switch ev {
	case ctxforge.HookPreToolUse:
		return "pre"
	case ctxforge.HookPostToolUse:
		return "post"
	}
	return ev
}

// --- form ---

var hookEventOpts = []string{ctxforge.HookPreToolUse, ctxforge.HookPostToolUse}
var hookActionOpts = []string{ctxforge.HookAsk, ctxforge.HookAllow, ctxforge.HookDeny}

// renderHookForm renders the add/edit form, mirroring the MCP/skill forms plus a
// mode-binding checkbox set and a live preflight tester.
func renderHookForm(h ctxforge.Hook, isNew bool, errMsg string) string {
	title, action := "Edit hook", "/hooks/"+h.ID
	if isNew {
		title, action = "New hook", "/hooks"
	}
	kind := h.Match.ToolKind
	if kind == "" {
		kind = hookKindAny
	}
	return formShell(title, action, "hooks", errMsg,
		idField(h.ID, isNew),
		selectField("Event", "event", def(h.Event, ctxforge.HookPreToolUse), hookEventOpts),
		selectField("Tool kind", "toolKind", kind, hookKindOpts),
		textField("Command pattern (substring, or * / ? glob)", "pattern", h.Match.Pattern, false),
		checkboxField("Match only writes whose target is OUTSIDE the workspace", "outsideWorkspace", h.Match.OutsideWorkspace),
		selectField("Action", "action", def(h.Action, ctxforge.HookAsk), hookActionOpts),
		textField("Reason (shown in the timeline / fed back on deny)", "reason", h.Reason, false),
		modesField(h.Modes),
		checkboxField("Enabled", "enabled", h.Enabled),
		frag("hookPreflight", map[string]any{"Sample": ""}),
		textField("PostToolUse command (program to run after a matching tool; post-tool-use only)", "command", h.Command, false),
		textField("Command args (comma-separated; ${VAR} resolved at run)", "commandArgs", strings.Join(h.CommandArgs, ", "), false),
		frag("hookCommandPreflight", nil),
	)
}

// modesField renders the mode-binding checkboxes (an empty selection = every
// mode). Posted as repeated name="modes" values, read back by parseModes.
func modesField(selected []string) string {
	set := map[string]bool{}
	for _, m := range selected {
		set[m] = true
	}
	type opt struct {
		Value string
		On    bool
	}
	opts := make([]opt, len(hookModeOpts))
	for i, m := range hookModeOpts {
		opts[i] = opt{Value: m, On: set[m]}
	}
	return frag("modesField", map[string]any{"Options": opts})
}

// parseModes reads the posted mode checkboxes (repeated name="modes") into a
// deterministic slice. Validation is left to Hook.Validate (the single gate), so a
// crafted unknown mode surfaces as the form's "invalid mode" error rather than
// being silently dropped here.
func parseModes(r *http.Request) []string {
	_ = r.ParseForm()
	seen := map[string]bool{}
	var out []string
	for _, m := range r.Form["modes"] {
		if m = strings.TrimSpace(m); m != "" && !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	sort.Strings(out)
	return out
}

// hookFromForm builds a Hook from the posted form. The "any" tool-kind sentinel
// maps back to an empty ToolKind (match any kind).
func hookFromForm(r *http.Request, id string) ctxforge.Hook {
	kind := strings.TrimSpace(r.FormValue("toolKind"))
	if kind == hookKindAny {
		kind = ""
	}
	return ctxforge.Hook{
		ID:    id,
		Event: strings.TrimSpace(r.FormValue("event")),
		Match: ctxforge.HookMatch{
			ToolKind:         kind,
			Pattern:          strings.TrimSpace(r.FormValue("pattern")),
			OutsideWorkspace: r.FormValue("outsideWorkspace") != "",
		},
		Action:      strings.TrimSpace(r.FormValue("action")),
		Reason:      strings.TrimSpace(r.FormValue("reason")),
		Enabled:     r.FormValue("enabled") != "",
		Modes:       parseModes(r),
		Command:     strings.TrimSpace(r.FormValue("command")),
		CommandArgs: parseCSV(r.FormValue("commandArgs")),
	}
}

// hookCRUD drives the add/edit/create/update lifecycle through the validated
// forge builders with rollback-on-invalid (forgecrud.go), exactly like the other
// forge entities. Toggle and delete are custom (built-ins are not in the forge,
// so they have no controls to route here).
var hookCRUD = forgeCRUD[ctxforge.Hook]{
	kind:   "hook",
	lookup: (*ctxforge.Forge).Hook, render: renderHookForm,
	blank: func() ctxforge.Hook {
		return ctxforge.Hook{Event: ctxforge.HookPreToolUse, Action: ctxforge.HookAsk, Enabled: true}
	},
	fromForm: hookFromForm,
	add:      (*ctxforge.Forge).AddHook, update: (*ctxforge.Forge).UpdateHook,
	remove: (*ctxforge.Forge).RemoveHook, list: (*Server).hooksPartial,
}

func (s *Server) handleHookNew(w http.ResponseWriter, r *http.Request)  { hookCRUD.New(s, w, r) }
func (s *Server) handleHookEdit(w http.ResponseWriter, r *http.Request) { hookCRUD.Edit(s, w, r) }

// reservedHookRouteIDs are id values that collide with a literal segment in the
// /hooks route group: an id "preflight" would make `POST /hooks/preflight` route
// to the preflight tester instead of the update handler (Go's mux prefers the
// literal over `{id}`), so the hook could never be saved. Rejecting them at
// create — the only entry point for a new id — keeps such an id from ever
// existing. — ADR-0031.
var reservedHookRouteIDs = map[string]bool{"new": true, "preflight": true, "command-preflight": true}

func (s *Server) handleHookCreate(w http.ResponseWriter, r *http.Request) {
	if id := strings.TrimSpace(r.FormValue("id")); reservedHookRouteIDs[id] {
		s.writePartial(w, renderHookForm(hookFromForm(r, id), true,
			fmt.Sprintf("hook id %q is reserved", id)))
		return
	}
	hookCRUD.Create(s, w, r)
}

func (s *Server) handleHookUpdate(w http.ResponseWriter, r *http.Request) { hookCRUD.Update(s, w, r) }

func (s *Server) handleHookToggle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_ = s.editForge(func() error { _, err := s.forge.ToggleHook(id); return err })
	s.writePartial(w, s.hooksPartial())
}

func (s *Server) handleHookDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.editForge(func() error { return s.forge.RemoveHook(r.PathValue("id")) }); err != nil {
		s.logger.Printf("remove hook: %v", err)
	}
	s.writePartial(w, s.hooksPartial())
}

// handleHookPreflight runs the pure matcher against a sample command and returns
// just the verdict fragment (swapped into the form's result slot), so a user sees
// whether the rule would fire before saving. It mutates nothing.
func (s *Server) handleHookPreflight(w http.ResponseWriter, r *http.Request) {
	pattern := strings.TrimSpace(r.FormValue("pattern"))
	sample := r.FormValue("sample")
	s.writePartial(w, hookPreflightResult(pattern, sample))
}

// handleHookCommandPreflight reports, WITHOUT executing, the resolved PostToolUse
// command line and which ${VAR} references resolve empty in the environment —
// mirroring the MCP env preflight (ADR-0020) behind the s.lookupEnv seam. It
// mutates nothing and never runs the command. — ADR-0032.
func (s *Server) handleHookCommandPreflight(w http.ResponseWriter, r *http.Request) {
	command := strings.TrimSpace(r.FormValue("command"))
	args := parseCSV(r.FormValue("commandArgs"))
	lookup := s.lookupEnv
	if lookup == nil {
		lookup = os.Getenv
	}
	s.writePartial(w, hookCommandPreflightResult(command, args, lookup))
}

// hookCommandPreflightResult is the pure command-preflight verdict: the command
// line (with ${VAR} placeholders left visible) and the set of referenced vars that
// resolve empty (a missing var would run UNSET at execution, never the literal).
// A command is only meaningful on a post-tool-use hook, noted in the fragment.
func hookCommandPreflightResult(command string, args []string, lookup func(string) string) string {
	if command == "" {
		return frag("hookCommandPreflightResult", map[string]any{"Empty": true})
	}
	line := command
	if len(args) > 0 {
		line += " " + strings.Join(args, " ")
	}
	var missing []string
	seen := map[string]bool{}
	for _, name := range commandVarRefs(line) {
		if !seen[name] {
			seen[name] = true
			if lookup(name) == "" {
				missing = append(missing, name)
			}
		}
	}
	return frag("hookCommandPreflightResult", map[string]any{
		"Command": line, "Missing": strings.Join(missing, ", "), "HasMissing": len(missing) > 0,
	})
}

// commandVarRefs returns the VAR_NAMEs referenced as ${VAR} inside s, in order
// (the inline companion to envRef, which matches a whole-string reference). The
// ${VAR} shape is decoded only in this web seam, not in ctxforge (ADR-0020).
func commandVarRefs(s string) []string {
	matches := commandVarRefPattern.FindAllStringSubmatch(s, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m[1])
	}
	return out
}

// commandVarRefPattern matches an inline ${VAR_NAME} reference (ADR-0020).
var commandVarRefPattern = regexp.MustCompile(`\$\{([A-Z_][A-Z0-9_]*)\}`)

// hookPreflightResult is the pure preflight verdict: which form the pattern takes
// (glob vs substring) and, given a non-empty sample, whether it matches.
func hookPreflightResult(pattern, sample string) string {
	if strings.TrimSpace(pattern) == "" {
		return frag("hookPreflightResult", map[string]any{"Empty": true})
	}
	kind := "substring"
	if ctxforge.PatternIsGlob(pattern) {
		kind = "glob"
	}
	data := map[string]any{"Kind": kind, "Pattern": pattern, "HasSample": sample != ""}
	if sample != "" {
		data["Matched"] = ctxforge.MatchPattern(pattern, sample)
		data["Sample"] = sample
	}
	return frag("hookPreflightResult", data)
}
