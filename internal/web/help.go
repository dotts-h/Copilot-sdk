// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package web

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dotts-h/copilot-sdk/internal/config"
)

// renderShortcuts renders the keyboard-shortcuts table (key → action) shared by
// the Help page and the help overlay. Keys and labels are HTML-escaped; the
// non-rebindable Esc-closes-overlay convention is appended as a fixed row.
func renderShortcuts(keymap []config.ResolvedKey) string {
	var b strings.Builder
	b.WriteString(`<table class="kv shortcuts">`)
	for _, k := range keymap {
		b.WriteString(`<tr><th><kbd>` + esc(k.Key) + `</kbd></th><td>` + esc(k.Label) + `</td></tr>`)
	}
	b.WriteString(`<tr><th><kbd>Esc</kbd></th><td>Close the shortcuts overlay</td></tr>`)
	b.WriteString(`</table>`)
	return b.String()
}

// helpOverlay renders the body-level keyboard-shortcuts overlay (hidden until
// the bound key opens it). It lives in the page shell so it works across htmx
// navigation; the keymap is the live, config-resolved set.
func helpOverlay(keymap []config.ResolvedKey) string { return helpOverlayAttr(keymap, "") }

// helpOverlayAttr renders the overlay with extra attributes spliced onto its
// root element — used to add hx-swap-oob="true" for the Settings live-apply swap
// (the OOB re-render is matched by #help-overlay), while the index render passes
// none. Keys/labels are HTML-escaped via renderShortcuts (ADR-0001).
func helpOverlayAttr(keymap []config.ResolvedKey, extraAttr string) string {
	return `<div id="help-overlay" class="overlay" hidden role="dialog" aria-modal="true" aria-label="Keyboard shortcuts"` + extraAttr + `>` +
		`<div class="overlay-card"><h2>Keyboard shortcuts</h2>` +
		renderShortcuts(keymap) +
		`<p class="dim">Shortcuts are ignored while you're typing in a field. Customise them on the Settings page.</p>` +
		`<button type="button" class="overlay-close" onclick="toggleHelpOverlay(false)">Close</button></div></div>`
}

// keymapJSON serializes the resolved keymap to the action→key JSON the frontend
// dispatcher reads from <body data-keymap>. Shared by the initial index render
// and the Settings live-apply OOB swap so both surfaces carry one source (the map
// marshals with sorted keys → deterministic).
func keymapJSON(keymap []config.ResolvedKey) string {
	dispatch := make(map[string]string, len(keymap))
	for _, k := range keymap {
		dispatch[k.ID] = k.Key
	}
	j, _ := json.Marshal(dispatch)
	return string(j)
}

// keymapLiveApply builds the Settings POST's live-apply payload: an hx-swap-oob
// re-render of the help overlay (matched by #help-overlay) plus a script that
// calls applyKeymap to update <body data-keymap> and the JS dispatcher's reverse
// map, so a rebind takes effect WITHOUT a full page reload (TECH_DEBT #13). The
// keymap reflects the PERSISTED config, so a no-op or rolled-back save re-emits
// the in-sync keymap and can never desync the live attribute from disk. The JSON
// is HTML-safe in the <script> context: encoding/json escapes <, >, & (so no
// </script> can form) and every key is a validated single character.
func keymapLiveApply(keymap []config.ResolvedKey) string {
	return helpOverlayAttr(keymap, ` hx-swap-oob="true"`) +
		`<script>applyKeymap(` + keymapJSON(keymap) + `)</script>`
}

// helpPartial renders the static Help/reference page: how the panels work, the
// composer slash commands, and the keyboard shortcuts. It is the discoverability
// surface behind /help in the composer.
func (s *Server) helpPartial() string {
	s.hub.forgeMu.Lock()
	keymap := s.config.Keymap()
	s.hub.forgeMu.Unlock()

	cmd := func(name, desc string) string {
		return `<tr><th><code>` + esc(name) + `</code></th><td>` + esc(desc) + `</td></tr>`
	}
	var b strings.Builder
	b.WriteString(`<section class="page help" tabindex="0"><h2>Help</h2>`)
	b.WriteString(`<p class="dim">my-orchestra is a cost-aware coding companion. Chat streams live; ` +
		`every tool call, the reasoning, and the credit spend are shown as they happen.</p>`)

	b.WriteString(`<h3>Composer commands</h3>`)
	b.WriteString(`<p class="dim">Type these in the chat composer instead of a prompt.</p>`)
	b.WriteString(`<table class="kv">`)
	b.WriteString(cmd("/model [name]", "Switch the model in place (restarts the session); no name shows the current one."))
	b.WriteString(cmd("/effort [low|medium|high]", "Set the reasoning effort (restarts the session); no value shows the current one."))
	b.WriteString(cmd("/agent [id|none]", "Activate a forge agent (applies its model + reasoning) or clear it."))
	b.WriteString(cmd("/plan [on|off]", "Toggle plan mode — the agent drafts a plan you approve or revise inline before it acts."))
	b.WriteString(cmd("/auto [on|off]", "Toggle autopilot — the agent runs tools without pausing to ask."))
	b.WriteString(cmd("/ask [on|off]", "Toggle ask mode — the agent checks in before each action."))
	b.WriteString(cmd("/clear", "Reset the conversation and start a fresh session."))
	b.WriteString(cmd("/cost", "Show credit usage and refresh the cost meter."))
	b.WriteString(cmd("/attach <path>", "Queue a file to send with your next message."))
	b.WriteString(cmd("/chat … /settings", "Jump to a page (chat, telemetry, skills, instructions, agents, settings)."))
	b.WriteString(cmd("/help", "List the commands in the timeline."))
	b.WriteString(`</table>`)

	b.WriteString(`<h3>Panels</h3><table class="kv">`)
	rows := [][2]string{
		{"Chat", "Stream prompts and replies; approve tool permissions, answer the agent's questions (ask_user), fill schema-driven forms from MCP servers (elicitation), and review its plans (approve or request changes) — all inline; abort an in-flight turn with ⏹ stop. " +
			"Type ahead while a turn runs — extra prompts queue and send automatically when the turn ends."},
		{"Sessions", "List, resume, or delete past conversations. Resuming restores the full context (the first turn after a gap won't hit the prompt cache); start fresh with + New chat."},
		{"Telemetry", "Live credit/token spend, per-model breakdown, and your monthly budget."},
		{"Skills", "Reusable prompt fragments; toggle which are active for the session."},
		{"Instructions", "Always-on guidance, ordered by priority. Import project files pulls in .github/copilot-instructions.md, AGENTS.md, and CLAUDE.md."},
		{"Agents", "Named model + reasoning + skill + tool-allowlist presets; select one as the default. A built-in Chat agent is always available."},
		{"Models", "Pick the model the session uses and its reasoning effort; selecting either restarts the session on your next prompt."},
		{"Settings", "Edit config.json's main knobs (model, effort, streaming, budget); applied on your next session. Advanced keys edit the file directly."},
	}
	for _, r := range rows {
		fmt.Fprintf(&b, `<tr><th>%s</th><td>%s</td></tr>`, esc(r[0]), esc(r[1]))
	}
	b.WriteString(`</table>`)

	b.WriteString(`<h3>Keyboard shortcuts</h3>`)
	b.WriteString(`<p class="dim">Press these anywhere outside a text field; the overlay also opens with its key. Customise them on the Settings page.</p>`)
	b.WriteString(renderShortcuts(keymap))

	b.WriteString(`</section>`)
	return b.String()
}
