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
	// PriceOverrides maps model -> [inputPerMTok, cachedPerMTok, outputPerMTok].
	PriceOverrides map[string][3]float64 `json:"priceOverrides,omitempty"`
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
	return nil
}

// GitHubToken resolves the configured token from the environment.
func (c *Config) GitHubToken() string { return os.Getenv(c.GitHubTokenEnv) }
