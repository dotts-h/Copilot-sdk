package web

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/dotts-h/copilot-sdk/internal/config"
)

// This file makes the Settings page editable. It renders config.json's
// user-facing knobs as a form and writes them back through editConfig, which
// snapshots-then-validates-then-rolls-back so an invalid edit never leaves the
// live config (or disk) in a bad state — the same discipline ctxforge uses for
// the forge. Advanced config keys (price overrides) are not exposed here; they
// remain editable directly in config.json. MCP servers are a forge entity with
// their own management page (see mcp.go), not a config key.

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
	return frag("settingsForm", map[string]any{
		"Fields": trusted(strings.Join(fields, "")), "Note": note, "Err": errMsg, "ForgeDir": c.ForgeDir,
	})
}

// handleSettingsSave applies the submitted settings, persisting on success and
// re-rendering the form in place with a saved confirmation or a validation error.
func (s *Server) handleSettingsSave(w http.ResponseWriter, r *http.Request) {
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
	})
	if err != nil {
		s.writePartial(w, s.renderSettings("", err.Error()))
		return
	}
	// Apply the saved budget knobs to this live session immediately so the gate
	// and soft-warn take effect now, not only on the next session.
	s.refreshBudget()
	s.writePartial(w, s.renderSettings("saved", ""))
}
