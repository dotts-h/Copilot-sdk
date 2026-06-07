package web

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/dotts-h/copilot-sdk/internal/config"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// This file makes the Settings page editable. It renders config.json's
// user-facing knobs as a form and writes them back through editConfig, which
// snapshots-then-validates-then-rolls-back so an invalid edit never leaves the
// live config (or disk) in a bad state — the same discipline ctxforge uses for
// the forge. The per-model price-override table (G1) is the last cost knob to
// gain a UI; a save persists it through editConfig AND reprices the live meter in
// place (see handleSettingsSave). MCP servers are a forge entity with their own
// management page (see mcp.go), not a config key.

// editConfig applies fn to the live config and persists it, restoring the prior
// config if either the mutation or the validating Save fails. Holds forgeMu, the
// shared forge/config mutation lock.
func (s *Server) editConfig(fn func(*config.Config)) error {
	s.hub.forgeMu.Lock()
	defer s.hub.forgeMu.Unlock()
	snapshot := *s.config
	fn(s.config)
	if err := s.config.Save(); err != nil {
		*s.config = snapshot // roll back the live config; Save() never persisted
		return err
	}
	return nil
}

// refreshBudget re-reads the budget knobs (allowance, warn fraction, hard cap)
// from the shared config into this session's cached copies, so a settings save
// takes effect on the live session immediately rather than only on the next one.
func (s *Server) refreshBudget() {
	s.hub.forgeMu.Lock()
	allowance := s.config.Telemetry.MonthlyCreditAllowance
	warn := s.config.Telemetry.WarnFraction
	hardCap := s.config.Telemetry.HardCapCredits
	s.hub.forgeMu.Unlock()

	s.mu.Lock()
	s.allowance, s.warnFraction, s.hardCap = allowance, warn, hardCap
	s.mu.Unlock()
}

// renderSettings locks and renders the settings form with an optional saved/error
// banner.
func (s *Server) renderSettings(note, errMsg string) string {
	s.hub.forgeMu.Lock()
	defer s.hub.forgeMu.Unlock()
	return renderSettingsForm(s.config, note, errMsg)
}

// renderSettingsForm builds the editable settings form. Caller holds forgeMu.
func renderSettingsForm(c *config.Config, note, errMsg string) string {
	effort := c.ReasoningEffort
	if effort == "" {
		effort = "medium"
	}
	fields := []string{
		textField("Default model", "defaultModel", c.DefaultModel, true),
		textField("Default agent (ID; blank = none)", "defaultAgent", c.DefaultAgent, false),
		selectField("Reasoning effort", "reasoningEffort", effort, reasoningOpts),
		checkboxField("Streaming", "streaming", c.Streaming),
		checkboxField("Auto-approve tools", "autoApproveTools", c.AutoApproveTools),
		numberField("Monthly credit budget", "allowance", int(c.Telemetry.MonthlyCreditAllowance)),
		numberField("Warn at (%)", "warnPercent", int(c.Telemetry.WarnFraction*100+0.5)),
		numberField("Hard cap (credits; 0 = off)", "hardCap", int(c.Telemetry.HardCapCredits)),
		textField("OTLP endpoint", "otlpEndpoint", c.Telemetry.OTLPEndpoint, false),
		textField("GitHub token env var (blank = copilot CLI session)", "githubTokenEnv", c.GitHubTokenEnv, false),
	}
	// Per-model price overrides (G1): one row per model, three numeric (float)
	// fields each (input/cached/output USD per million tokens). Closes the last
	// cost knob that required hand-editing config.json.
	fields = append(fields, priceOverrideFields(c)...)
	// Keyboard shortcuts: one single-key field per rebindable action. A blank
	// reverts to the default; Esc (close overlay) is fixed and not rebindable.
	fields = append(fields,
		`<h3 class="settings-subhead">Keyboard shortcuts</h3>`,
		`<p class="dim">One key each (case-sensitive); blank reverts to the default. Esc always closes the overlay.</p>`,
	)
	for _, k := range c.Keymap() {
		fields = append(fields, textField(k.Label, "key_"+k.ID, k.Key, false))
	}
	return frag("settingsForm", map[string]any{
		"Fields": trusted(strings.Join(fields, "")), "Note": note, "Err": errMsg, "ForgeDir": c.ForgeDir,
	})
}

// priceOverrideFields renders the per-model price-override section: a subhead, a
// hint, and one row per model. Rows are seeded from the default price book's
// models ∪ any models that already carry an override (so an override on a model
// not in the defaults still shows), sorted for a stable, deterministic order.
// Each row's three fields are pre-filled from the saved override (blank when
// none), with the built-in default rate shown as the placeholder. The rebuild on
// save reads back from the defaults, so the form never needs the live (already-
// overridden) book — it builds from a fresh DefaultPriceBook for the placeholders.
func priceOverrideFields(c *config.Config) []string {
	def := telemetry.DefaultPriceBook()
	seen := map[string]bool{}
	var models []string
	for _, m := range def.Models() {
		if !seen[m] {
			seen[m] = true
			models = append(models, m)
		}
	}
	for m := range c.Telemetry.PriceOverrides {
		if !seen[m] {
			seen[m] = true
			models = append(models, m)
		}
	}
	sort.Strings(models)

	fields := []string{
		`<h3 class="settings-subhead">Per-model price overrides</h3>`,
		`<p class="dim">USD per million tokens (input · cached · output). Blank uses the built-in default shown as the placeholder; clear all three to remove an override. Saving reprices the live meter immediately.</p>`,
	}
	for i, m := range models {
		rate, _ := def.Rate(m)
		ov, has := c.Telemetry.PriceOverrides[m]
		fields = append(fields, priceRowField(i, m, ov, has, rate))
	}
	return fields
}

// priceRowField renders one model's override row. Field names are index-keyed
// (price.<i>.model/in/cached/out) with the model id carried in a hidden field, so
// model ids containing dots (e.g. "claude-opus-4.7") can't be confused with the
// field-name delimiter on parse.
func priceRowField(i int, model string, ov [3]float64, has bool, def telemetry.ModelRate) string {
	fmtRate := func(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }
	in, cached, out := "", "", ""
	if has {
		in, cached, out = fmtRate(ov[0]), fmtRate(ov[1]), fmtRate(ov[2])
	}
	return frag("priceRow", map[string]any{
		"Key":   fmt.Sprintf("price.%d", i),
		"Model": model,
		"In":    in, "Cached": cached, "Out": out,
		"InDef": fmtRate(def.InputPerMTok), "CachedDef": fmtRate(def.CachedInputPerMTok), "OutDef": fmtRate(def.OutputPerMTok),
	})
}

// parsePriceOverrides reads the index-keyed price-override rows from a submitted
// settings form into the config map. A row whose three fields are ALL blank
// contributes NO override (the model keeps its built-in default — a blank row must
// never persist as a $0-rate override that would price turns at zero). In a row
// that IS edited, a blank cell falls back to that model's default rate (honoring
// the form hint "blank uses the built-in default"), so editing only one rate can't
// silently zero the other two. An explicitly typed number — including 0 — is kept
// verbatim, so a negative value reaches config.Validate (which rejects it, rolling
// the whole save back via editConfig). Returns nil when no row carries an override.
func parsePriceOverrides(r *http.Request) map[string][3]float64 {
	def := telemetry.DefaultPriceBook()
	overrides := map[string][3]float64{}
	for i := 0; ; i++ {
		key := fmt.Sprintf("price.%d", i)
		model := strings.TrimSpace(r.FormValue(key + ".model"))
		if model == "" {
			break // rows are rendered contiguously from 0; first gap ends the list
		}
		inRaw := strings.TrimSpace(r.FormValue(key + ".in"))
		cachedRaw := strings.TrimSpace(r.FormValue(key + ".cached"))
		outRaw := strings.TrimSpace(r.FormValue(key + ".out"))
		if inRaw == "" && cachedRaw == "" && outRaw == "" {
			continue // wholly blank row → no override (model keeps its default)
		}
		rate, _ := def.Rate(model)
		overrides[model] = [3]float64{
			rateOrDefault(inRaw, rate.InputPerMTok),
			rateOrDefault(cachedRaw, rate.CachedInputPerMTok),
			rateOrDefault(outRaw, rate.OutputPerMTok),
		}
	}
	if len(overrides) == 0 {
		return nil
	}
	return overrides
}

// rateOrDefault parses one price cell. A blank cell uses the model's default rate
// (the form hint promises blanks keep the default). An explicit number — including
// 0 (a deliberate free rate) and a negative (rejected later by Validate) — is kept
// verbatim. Unparseable garbage falls back to the default rather than $0, so a
// stray value never silently underprices a model.
func rateOrDefault(s string, def float64) float64 {
	if s == "" {
		return def
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return def
	}
	return v
}

// formHasPriceOverrides reports whether the submitted form carried the price-
// override section (any price.* field), so a partial POST that omits it leaves the
// stored overrides untouched rather than wiping them — the same preserve-on-absent
// discipline formHasKeyBindings uses for the keyboard-shortcut section.
func formHasPriceOverrides(r *http.Request) bool {
	for k := range r.Form {
		if strings.HasPrefix(k, "price.") {
			return true
		}
	}
	return false
}

// formHasKeyBindings reports whether the submitted form carried the keyboard-
// shortcut section (any key_* field), so a save can distinguish "the user
// cleared this shortcut" (present-but-blank → revert to default) from "this POST
// didn't include the section at all" (absent → leave bindings untouched).
func formHasKeyBindings(r *http.Request) bool {
	for _, a := range config.KeyActions() {
		if _, ok := r.Form["key_"+a.ID]; ok {
			return true
		}
	}
	return false
}

// handleSettingsSave applies the submitted settings, persisting on success and
// re-rendering the form in place with a saved confirmation or a validation error.
func (s *Server) handleSettingsSave(w http.ResponseWriter, r *http.Request) {
	// Parse the form up front so formHasPriceOverrides (which scans r.Form) does not
	// depend on a sibling FormValue call having lazily parsed it first.
	_ = r.ParseForm()
	// Parse the price-override rows before editConfig. Keep the parsed map so the
	// live reprice below rebuilds from exactly what was persisted.
	priceOverrides := parsePriceOverrides(r)
	hasPriceOverrides := formHasPriceOverrides(r)
	err := s.editConfig(func(c *config.Config) {
		c.DefaultModel = strings.TrimSpace(r.FormValue("defaultModel"))
		c.DefaultAgent = strings.TrimSpace(r.FormValue("defaultAgent"))
		c.ReasoningEffort = strings.TrimSpace(r.FormValue("reasoningEffort"))
		c.Streaming = r.FormValue("streaming") != ""
		c.AutoApproveTools = r.FormValue("autoApproveTools") != ""
		c.GitHubTokenEnv = strings.TrimSpace(r.FormValue("githubTokenEnv"))
		c.Telemetry.OTLPEndpoint = strings.TrimSpace(r.FormValue("otlpEndpoint"))
		// Numeric fields keep their current value when blank/unparseable; Validate
		// catches out-of-range values (e.g. warn% > 100 → fraction > 1).
		if v, e := strconv.ParseFloat(strings.TrimSpace(r.FormValue("allowance")), 64); e == nil {
			c.Telemetry.MonthlyCreditAllowance = v
		}
		if v, e := strconv.Atoi(strings.TrimSpace(r.FormValue("warnPercent"))); e == nil {
			c.Telemetry.WarnFraction = float64(v) / 100
		}
		if v, e := strconv.ParseFloat(strings.TrimSpace(r.FormValue("hardCap")), 64); e == nil {
			c.Telemetry.HardCapCredits = v
		}
		// Price overrides: replace the whole table only when the form carried the
		// section (preserve-on-absent, like key bindings). Validate (in Save) rejects
		// a negative rate, rolling the edit back through editConfig.
		if hasPriceOverrides {
			c.Telemetry.PriceOverrides = priceOverrides
		}
		// Key bindings: store only the overrides that differ from the default; a
		// blank or default-valued field clears the override. Validate (in Save)
		// rejects an invalid key (multi-char) or a duplicate, rolling the edit back.
		// Only rebuild when the form actually carried the shortcut section, so a
		// partial POST (one that omits the key_ fields) preserves the current
		// bindings rather than silently wiping them — same preserve-on-absent
		// discipline as the numeric fields above.
		if formHasKeyBindings(r) {
			bindings := map[string]string{}
			for _, a := range config.KeyActions() {
				v := strings.TrimSpace(r.FormValue("key_" + a.ID))
				if v != "" && v != a.Default {
					bindings[a.ID] = v
				}
			}
			if len(bindings) == 0 {
				c.KeyBindings = nil
			} else {
				c.KeyBindings = bindings
			}
		}
	})
	if err != nil {
		s.writePartial(w, s.renderSettings("", err.Error()))
		return
	}
	// Apply the saved budget knobs to this live session immediately so the gate
	// and soft-warn take effect now, not only on the next session.
	s.refreshBudget()
	// Apply the saved price overrides LIVE: REBUILD the price book from
	// DefaultPriceBook()+overrides and Replace the shared book's contents in place,
	// so the account meter AND every session meter (which share this one *PriceBook
	// by reference) reprice the next turn's estimate/meter without a restart. The
	// rebuild is deliberate (not incremental Set): a removed/lowered override reverts
	// toward the default. Replace touches only the price book's own (leaf) lock — it
	// runs after editConfig has released forgeMu, so it adds no lock-order risk.
	if hasPriceOverrides {
		s.meter.PriceBook().Replace(telemetry.BuildPriceBook(priceOverrides))
	}
	out := s.renderSettings("saved", "")
	// Live-apply a keybinding rebind WITHOUT a full page reload (V10, TECH_DEBT
	// #13): append an OOB re-render of the help overlay + a script that updates
	// <body data-keymap> and the JS dispatcher's map. Only when the form carried
	// the shortcut section, so a partial POST stays inert. The emitted keymap is
	// read back from the now-persisted config (post-rollback path is the error
	// branch above, which emits nothing), so the live attribute can never desync
	// from disk — a no-op save re-emits the identical, in-sync keymap.
	if formHasKeyBindings(r) {
		s.hub.forgeMu.Lock()
		keymap := s.config.Keymap()
		s.hub.forgeMu.Unlock()
		out += keymapLiveApply(keymap)
	}
	s.writePartial(w, out)
}
