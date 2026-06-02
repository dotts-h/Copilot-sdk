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

// Theme selects the TUI color palette.
type Theme string

const (
	ThemeAuto  Theme = "auto"
	ThemeDark  Theme = "dark"
	ThemeLight Theme = "light"
)

// Config is the root persisted configuration.
type Config struct {
	// DefaultModel is used when no agent overrides it.
	DefaultModel string `json:"defaultModel"`
	// DefaultAgent is the agent ID activated on launch (empty = none).
	DefaultAgent string `json:"defaultAgent"`
	// ReasoningEffort is the default effort when not set by an agent.
	ReasoningEffort string `json:"reasoningEffort"`
	// Theme controls the palette.
	Theme Theme `json:"theme"`
	// Streaming toggles incremental token rendering.
	Streaming bool `json:"streaming"`
	// AutoApproveTools, when true, approves all tool calls without prompting.
	AutoApproveTools bool `json:"autoApproveTools"`

	// ForgeDir is where ctxforge persists (defaults under the config dir).
	ForgeDir string `json:"forgeDir"`
	// SidecarCommand launches the Node Copilot SDK sidecar.
	SidecarCommand string   `json:"sidecarCommand"`
	SidecarArgs    []string `json:"sidecarArgs,omitempty"`
	// GitHubTokenEnv names the env var holding the GitHub token.
	GitHubTokenEnv string `json:"githubTokenEnv"`

	// Telemetry configures the credits dashboard.
	Telemetry TelemetryConfig `json:"telemetry"`
	// Keybindings maps action names to key strings.
	Keybindings map[string]string `json:"keybindings,omitempty"`

	// dir is the directory this config loads/saves from (not serialized).
	dir string
}

// TelemetryConfig configures budget tracking and price overrides.
type TelemetryConfig struct {
	// MonthlyCreditAllowance is the plan's included AI Credits.
	MonthlyCreditAllowance float64 `json:"monthlyCreditAllowance"`
	// WarnFraction triggers a UI warning once usage crosses this fraction.
	WarnFraction float64 `json:"warnFraction"`
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
		Theme:            ThemeAuto,
		Streaming:        true,
		AutoApproveTools: false,
		ForgeDir:         filepath.Join(dir, "forge"),
		SidecarCommand:   "node",
		SidecarArgs:      []string{filepath.Join(dir, "sidecar", "index.mjs")},
		GitHubTokenEnv:   "GITHUB_TOKEN",
		Telemetry: TelemetryConfig{
			MonthlyCreditAllowance: 1500, // Pro: $15 in credits.
			WarnFraction:           0.8,
		},
		Keybindings: DefaultKeybindings(),
		dir:         dir,
	}
}

// DefaultKeybindings returns the baseline action->key map.
func DefaultKeybindings() map[string]string {
	return map[string]string{
		"quit":         "ctrl+c",
		"help":         "?",
		"chat":         "1",
		"telemetry":    "2",
		"skills":       "3",
		"instructions": "4",
		"agents":       "5",
		"settings":     "6",
		"config":       "7",
		"submit":       "enter",
		"cancel":       "esc",
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
	if c.Theme == "" {
		c.Theme = ThemeAuto
	}
	if c.GitHubTokenEnv == "" {
		c.GitHubTokenEnv = "GITHUB_TOKEN"
	}
	if c.ForgeDir == "" {
		c.ForgeDir = filepath.Join(c.dir, "forge")
	}
	if c.Keybindings == nil {
		c.Keybindings = DefaultKeybindings()
	}
}

// Validate enforces invariants the UI relies on.
func (c *Config) Validate() error {
	if c.DefaultModel == "" {
		return fmt.Errorf("defaultModel is required")
	}
	switch c.Theme {
	case ThemeAuto, ThemeDark, ThemeLight:
	default:
		return fmt.Errorf("invalid theme %q", c.Theme)
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
	return nil
}

// GitHubToken resolves the configured token from the environment.
func (c *Config) GitHubToken() string { return os.Getenv(c.GitHubTokenEnv) }

// Key returns the key binding for an action, or "" if unbound.
func (c *Config) Key(action string) string { return c.Keybindings[action] }
