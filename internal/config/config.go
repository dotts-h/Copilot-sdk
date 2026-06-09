// Package config owns my-orchestra's persistent application configuration: the
// data behind both the Settings page (user-editable knobs) and the Config page
// (the full, advanced view). It is JSON-backed and dependency-free.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config is the root persisted configuration.
type Config struct {
	// DefaultModel is used when no agent overrides it.
	DefaultModel string `json:"defaultModel"`
	// DefaultAgent is the agent ID activated on launch (empty = none).
	DefaultAgent string `json:"defaultAgent"`
	// ReasoningEffort is the default effort when not set by an agent.
	ReasoningEffort string `json:"reasoningEffort"`
	// Streaming toggles incremental token rendering.
	Streaming bool `json:"streaming"`
	// AutoApproveTools, when true, approves all tool calls without prompting.
	AutoApproveTools bool `json:"autoApproveTools"`

	// ForgeDir is where ctxforge persists (defaults under the config dir).
	ForgeDir string `json:"forgeDir"`
	// GitHubTokenEnv optionally names an env var holding a GitHub token to use
	// as an explicit auth override. When empty (the default), the app uses the
	// already-logged-in `copilot` CLI session instead of requesting a token.
	GitHubTokenEnv string `json:"githubTokenEnv"`

	// Telemetry configures the credits dashboard.
	Telemetry TelemetryConfig `json:"telemetry"`

	// KeyBindings overrides the default keyboard shortcut for a UI action, keyed
	// by the action id (see KeyActions). Only overrides are stored; an action
	// absent here uses its built-in default. Edited on the Settings page and
	// listed in the help overlay. omitempty so older files read clean.
	KeyBindings map[string]string `json:"keyBindings,omitempty"`

	// dir is the directory this config loads/saves from (not serialized).
	dir string
}

// TelemetryConfig configures budget tracking and price overrides.
type TelemetryConfig struct {
	// MonthlyCreditAllowance is the plan's included AI Credits.
	MonthlyCreditAllowance float64 `json:"monthlyCreditAllowance"`
	// WarnFraction triggers a UI warning once usage crosses this fraction.
	WarnFraction float64 `json:"warnFraction"`
	// HardCapCredits is an absolute credit ceiling; a turn whose projected spend
	// would exceed it is paused for confirmation. Zero (the default) disables it.
	HardCapCredits float64 `json:"hardCapCredits,omitempty"`
	// OTLPEndpoint, if set, is forwarded to the SDK's OpenTelemetry exporter.
	OTLPEndpoint string `json:"otlpEndpoint,omitempty"`
	// PriceOverrides maps model -> [inputPerMTok, cachedPerMTok, outputPerMTok] and
	// an OPTIONAL 4th element, cacheWritePerMTok (ADR-0034). A 3-element override is
	// the pre-0059 shape and still loads (cache-write derives at the 1.25× default);
	// a 4-element override pins the cache-write rate. A variable-length slice (not a
	// fixed array) so both the legacy 3- and the new 4-element JSON forms read back
	// natively — the backward-readable migration.
	PriceOverrides map[string][]float64 `json:"priceOverrides,omitempty"`
}

const configFile = "config.json"

// Default returns a fully-populated config with sensible defaults, bound to dir.
func Default(dir string) *Config {
	return &Config{
		DefaultModel:     "gpt-5",
		DefaultAgent:     "",
		ReasoningEffort:  "medium",
		Streaming:        true,
		AutoApproveTools: false,
		ForgeDir:         filepath.Join(dir, "forge"),
		GitHubTokenEnv:   "", // empty: use the logged-in `copilot` CLI session
		Telemetry: TelemetryConfig{
			MonthlyCreditAllowance: 1500, // Pro: $15 in credits.
			WarnFraction:           0.8,
		},
		dir: dir,
	}
}

// Dir returns the configuration directory.
func (c *Config) Dir() string { return c.dir }

// Load reads config from dir/config.json, returning Default(dir) when the file
// is absent. Present-but-invalid files are an error.
func Load(dir string) (*Config, error) {
	path := filepath.Join(dir, configFile)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Default(dir), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	// Start from defaults so new fields are populated on upgrade.
	c := Default(dir)
	if err := json.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	c.dir = dir
	c.normalize()
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}

// Save validates and atomically writes the config.
func (c *Config) Save() error {
	c.normalize()
	if err := c.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	path := filepath.Join(c.dir, configFile)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return os.Rename(tmp, path)
}

// normalize fills empty derived fields and trims whitespace.
func (c *Config) normalize() {
	c.DefaultModel = strings.TrimSpace(c.DefaultModel)
	// GitHubTokenEnv is intentionally left empty by default so the app uses the
	// logged-in `copilot` CLI session; only honor an explicitly configured name.
	if c.ForgeDir == "" {
		c.ForgeDir = filepath.Join(c.dir, "forge")
	}
	c.normalizeKeyBindings()
}

// Validate enforces invariants the UI relies on.
func (c *Config) Validate() error {
	if c.DefaultModel == "" {
		return fmt.Errorf("defaultModel is required")
	}
	switch c.ReasoningEffort {
	case "", "low", "medium", "high", "xhigh":
	default:
		return fmt.Errorf("invalid reasoningEffort %q", c.ReasoningEffort)
	}
	if c.Telemetry.MonthlyCreditAllowance < 0 {
		return fmt.Errorf("monthlyCreditAllowance must be >= 0")
	}
	if c.Telemetry.WarnFraction < 0 || c.Telemetry.WarnFraction > 1 {
		return fmt.Errorf("warnFraction must be within [0,1]")
	}
	if c.Telemetry.HardCapCredits < 0 {
		return fmt.Errorf("hardCapCredits must be >= 0")
	}
	// Price overrides are USD-per-million-token rates: input/cached/output, plus an
	// optional 4th cache-write rate (ADR-0034). A row must carry 3 or 4 elements (a
	// shorter row can't price a turn; a longer one is malformed), and a negative
	// rate would price a turn as a credit, which the meter must never do. Absent
	// overrides are valid (older configs without priceOverrides stay backward-
	// compatible); a 3-element legacy row stays valid and derives cache-write.
	for model, r := range c.Telemetry.PriceOverrides {
		if len(r) != 3 && len(r) != 4 {
			return fmt.Errorf("priceOverrides[%q] must have 3 or 4 rates (input, cached, output[, cacheWrite]), got %d", model, len(r))
		}
		for _, rate := range r {
			if rate < 0 {
				return fmt.Errorf("priceOverrides[%q] rates must be >= 0", model)
			}
		}
	}
	if err := c.validateKeyBindings(); err != nil {
		return err
	}
	return nil
}

// GitHubToken resolves the configured token from the environment.
func (c *Config) GitHubToken() string { return os.Getenv(c.GitHubTokenEnv) }
